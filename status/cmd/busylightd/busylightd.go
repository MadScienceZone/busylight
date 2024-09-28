//
// Long-running daemon to update the busylight status
// based on availability as shown on Google calendars.
//
//    USR1   - in online meeting, muted
//    USR2   - in online meeting, unmuted
//    HUP    - out of online meeting
//    INFO   - force refresh from calendar now
//    VTALRM - wake from idle state (was: toggle urgent indicator)
//    WINCH  - enter idle state (was: toggle idle/working state)
//    CHLD   - not used (was: toggle low-priority)
//    INT    - turn off lights and exit
//
// Steve Willoughby <steve@madscience.zone>
// License: BSD 3-Clause open-source license
//

package main

import (
	"encoding/json"
	"fmt"
	"internal/busylight"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

func getClient(config *oauth2.Config, tokFile string) (*http.Client, error) {
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		return nil, err
	}
	return config.Client(context.Background(), tok), nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// BusyPeriod specifies a range of times during which a calendar indicates one or more events occur.
type BusyPeriod struct {
	Start, End time.Time
}

// ByStartTime provides a custom sort order for `BusyPeriod` elements.
type ByStartTime []BusyPeriod

func (a ByStartTime) Len() int {
	return len(a)
}

func (a ByStartTime) Less(i, j int) bool {
	return a[i].Start.Before(a[j].Start)
}

func (a ByStartTime) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// CalendarAvailability tracks the overall availability as shown on the monitored calendars.
type CalendarAvailability struct {
	// When did we most recently check with the API to get calendar busy/free times?
	LastPollTime time.Time

	// The list of "busy" time spans found on the calendars from the last poll.
	UpcomingPeriods []BusyPeriod // will be in chronological order
}

// RemoveExpiredPeriods trims busy spans from a `CalendarAvailability` value which occur in the past.
func (cal *CalendarAvailability) RemoveExpiredPeriods(config *busylight.ConfigData, devState *busylight.DevState) {
	for len(cal.UpcomingPeriods) > 0 {
		if time.Now().Add(5 * time.Second).After(cal.UpcomingPeriods[0].End) {
			cal.UpcomingPeriods = cal.UpcomingPeriods[1:]
		} else {
			break
		}
	}
	if len(cal.UpcomingPeriods) == 0 && time.Now().After(cal.LastPollTime.Add(30*time.Minute)) {
		err := cal.Refresh(config, devState)
		if err != nil {
			devState.Logger.Printf("Unable to refresh calendar data while removing expired periods: %v", err)
		}
	}
	// yes, we're trusting the Google service not to give us past events.
}

// NextTransitionTime returns the absolute time at which we need to check again to change the lights.
func (cal *CalendarAvailability) NextTransitionTime(config *busylight.ConfigData, devState *busylight.DevState) time.Time {
	cal.RemoveExpiredPeriods(config, devState)

	if len(cal.UpcomingPeriods) == 0 {
		// nothing scheduled for the time we queried about.
		// Tell the caller to check back in 8 hours.
		return time.Now().Add(8 * time.Hour)
	}
	if time.Now().Add(5 * time.Second).After(cal.UpcomingPeriods[0].Start) {
		// we're already into the period, so the next transition will be at its end
		return cal.UpcomingPeriods[0].End
	}
	// the period hasn't started yet so the transition will be at its beginning.
	return cal.UpcomingPeriods[0].Start
}

// ScheduledBusyNow checks to see if, according to the monitored calendars, we are scheduled to be busy right now.
func (cal *CalendarAvailability) ScheduledBusyNow(config *busylight.ConfigData, devState *busylight.DevState) bool {
	cal.RemoveExpiredPeriods(config, devState)

	if len(cal.UpcomingPeriods) == 0 {
		return false
	}
	if time.Now().Add(5 * time.Second).After(cal.UpcomingPeriods[0].Start) {
		return true
	}
	return false
}

// Refresh polls the Google API and updates the `CalendarAvailability` structure accordingly.
func (cal *CalendarAvailability) Refresh(config *busylight.ConfigData, devState *busylight.DevState) error {
	devState.Logger.Printf("Polling Google Calendars")
	googleConfig, err := google.ConfigFromJSON(devState.GoogleConfig, calendar.CalendarReadonlyScope)
	if err != nil {
		return err
	}

	client, err := getClient(googleConfig, config.TokenFile)
	if err != nil {
		return fmt.Errorf("Unable to query calendar: %v", err)
	}

	srv, err := calendar.New(client)
	if err != nil {
		return err
	}

	var query calendar.FreeBusyRequest
	queryStartTime := time.Now()
	queryEndTime := queryStartTime.Add(time.Hour * 8)
	query.TimeMin = queryStartTime.Format(time.RFC3339)
	query.TimeMax = queryEndTime.Format(time.RFC3339)
	for cID := range config.Calendars {
		query.Items = append(query.Items, &calendar.FreeBusyRequestItem{Id: cID})
	}
	freelist, err := srv.Freebusy.Query(&query).Do()
	if err != nil {
		return err
	}

	var rawbusylist []BusyPeriod
	for calID, calData := range freelist.Calendars {
		calInfo, isKnown := config.Calendars[calID]
		if !isKnown {
			devState.Logger.Printf("WARNING: Calendar <%s> in API results does not match any in our configuration!", calID)
			calInfo = busylight.CalendarConfigData{
				Title: fmt.Sprintf("UNKNOWN<%v>", calID),
			}
		}

		for _, e := range calData.Errors {
			devState.Logger.Printf("ERROR: Calendar \"%s\": %v", calInfo.Title, e)
		}
		for _, busy := range calData.Busy {
			startTime, err := time.Parse(time.RFC3339, busy.Start)
			if err != nil {
				devState.Logger.Printf("ERROR: %s: Unable to parse start time \"%v\": %v", calInfo.Title, busy.Start, err)
				continue
			}
			endTime, err := time.Parse(time.RFC3339, busy.End)
			if err != nil {
				devState.Logger.Printf("ERROR: %s: Unable to parse end time \"%v\": %v", calInfo.Title, busy.End, err)
				continue
			}
			devState.Logger.Printf("Calendar \"%s\": busy %v - %v", calInfo.Title, startTime.Local(), endTime.Local())
			if calInfo.IgnoreAllDayEvents {
				// This calendar is on our ignore list for all-day bookings.
				// There isn't any really great way to identify all-day events
				// since all we see is the aggregate busy time ranges.
				// So we'll compromise by assuming if the calendar is marked busy for the
				// entire query period, it's something we should ignore for the given
				// calendar.
				// It's far from perfect but it gets us closer to something useful.
				if startTime.Before(queryStartTime.Add(5*time.Second)) &&
					endTime.After(queryEndTime.Add(-5*time.Second)) {
					devState.Logger.Printf("Ignoring long-running event from %s", calInfo.Title)
					continue
				}
			}
			rawbusylist = append(rawbusylist, BusyPeriod{Start: startTime, End: endTime})
		}
	}
	// smush list and sort it
	devState.Logger.Printf("DEBUG: Initial list: %v", rawbusylist)
	sort.Sort(ByStartTime(rawbusylist))
	devState.Logger.Printf("DEBUG: Sorted list: %v", rawbusylist)
	var currentStart time.Time
	var currentEnd time.Time

	cal.UpcomingPeriods = nil
	for _, eachPeriod := range rawbusylist {
		if currentEnd.IsZero() {
			currentEnd = eachPeriod.End
		}

		if currentStart.IsZero() {
			currentStart = eachPeriod.Start
		} else if eachPeriod.Start.After(currentEnd) {
			// disjoint; we've reached the end of our busy time, so commit what we have
			cal.UpcomingPeriods = append(cal.UpcomingPeriods, BusyPeriod{Start: currentStart, End: currentEnd})
			currentStart = eachPeriod.Start
			currentEnd = eachPeriod.End
		} else if eachPeriod.End.After(currentEnd) {
			// overlapping; this ends after what we have so far, so extend our busy time
			currentEnd = eachPeriod.End
		} else {
			// overlapping; this is completely inside the time we already have, so we don't need to do anything.
		}
	}
	if !currentStart.IsZero() {
		// we need to commit the last one, too
		cal.UpcomingPeriods = append(cal.UpcomingPeriods, BusyPeriod{Start: currentStart, End: currentEnd})
	}
	devState.Logger.Printf("DEBUG: final list: %v", cal.UpcomingPeriods)
	cal.LastPollTime = time.Now()
	return nil
}

//
// We maintain a list of busy/free times since the last time we polled the calendar.
// from that we can also know when the next transition time will be
// global state:
//  busy until next transition
//  free until next transition
// Also globally know if in zoom meeting, which overrides the busy/free indicator
//  until the meeting ends.
//
// At transition time:
//  change global state
//  signal status if not in zoom meeting
//  schedule next transition
//
// Hourly:
//  reload state from google
//  update status as it should be now
//  re-schedule next transition

func setup(config *busylight.ConfigData, devState *busylight.DevState) error {
	var thisUser *user.User
	previousLogFile := config.LogFile
	previousPidFile := config.PidFile

	thisUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("Unable to determine current user: %v", err)
	}

	err = busylight.GetConfigFromFile(filepath.Join(thisUser.HomeDir, ".busylight/config.json"), config)
	if err != nil {
		return fmt.Errorf("Unable to initialize: %v", err)
	}

	//
	// If we're just re-reading the configuration, we will leave the
	// existing logfile and pid file alone.
	//
	if devState.Logger == nil {
		f, err := os.OpenFile(config.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("Unable to open logfile: %v", err)
		}
		devState.Logger = log.New(f, "busylightd: ", log.LstdFlags)

		myPID := os.Getpid()
		devState.Logger.Printf("busylightd started, PID=%v", myPID)

		pidf, err := os.OpenFile(config.PidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			devState.Logger.Printf("ERROR creating PID file (is another busylightd running?): %v", err)
			return err
		}
		pidf.WriteString(fmt.Sprintf("%d\n", myPID))
		pidf.Close()

		devState.GoogleConfig, err = ioutil.ReadFile(config.CredentialFile)
		if err != nil {
			devState.Logger.Printf("Unable to read client secret file %v: %v", config.CredentialFile, err)
			return fmt.Errorf("Unable to read client secret file %v: %v", config.CredentialFile, err)
		}
	} else {
		if previousPidFile != config.PidFile {
			devState.Logger.Printf("WARNING: PID file changed from %v to %v on reload. This requires a full restart of the daemon. Ignoring the change for now.", previousPidFile, config.PidFile)
		}
		if previousLogFile != config.LogFile {
			devState.Logger.Printf("WARNING: Log file changed from %v to %v on reload. This requires a full restart of the daemon. Ignoring the change for now.", previousLogFile, config.LogFile)
		}
	}

	//
	// Signal that we're online and ready
	//
	_ = busylight.AttachToLight(config, devState)
	_ = busylight.LightSignal(config, devState, "start", 100*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "off", 50*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "start", 100*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "off", 0)
	busylight.DetachFromLight(devState)

	return nil
}

// reverse whatever setup() did
func closeDevice(config *busylight.ConfigData, devState *busylight.DevState) {
	_ = busylight.AttachToLight(config, devState)
	_ = busylight.LightSignal(config, devState, "stop", 100*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "off", 50*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "stop", 100*time.Millisecond)
	_ = busylight.LightSignal(config, devState, "off", 0)
	busylight.DetachFromLight(devState)
}

func shutdown(config *busylight.ConfigData, devState *busylight.DevState) {
	closeDevice(config, devState)
	err := os.Remove(config.PidFile)
	if err != nil {
		devState.Logger.Printf("Error removing PID file: %v", err)
	}
	devState.Logger.Printf("busylightd shutting down")
}

func main() {
	var config busylight.ConfigData
	var devState busylight.DevState

	if err := setup(&config, &devState); err != nil {
		log.Fatalf("Unable to start daemon: %v", err)
	}
	defer shutdown(&config, &devState)

	//
	// Listen for incoming signals from outside
	//
	req := make(chan os.Signal, 5)
	signal.Notify(req, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGWINCH, syscall.SIGPWR, syscall.SIGINT, syscall.SIGVTALRM)

	//
	// Get initial calendar download
	//
	var busyTimes CalendarAvailability
	err := busyTimes.Refresh(&config, &devState)
	if err != nil {
		devState.Logger.Printf("Error updating busy/free times from calendar: %v", err)
	}

	isZoomNow := false
	isZoomMuted := false
	isActiveNow := true

	//
	// Set the current state and schedule for next transition
	//
	isBusyTimeNow := busyTimes.ScheduledBusyNow(&config, &devState)
	nextTransitionTime := busyTimes.NextTransitionTime(&config, &devState)
	transitionTimer := time.NewTimer(time.Until(nextTransitionTime))

	if isBusyTimeNow {
		if err := busylight.LightSignal(&config, &devState, "busy", 0); err != nil {
			shutdown(&config, &devState)
			os.Exit(1)
		}
	} else {
		if err := busylight.LightSignal(&config, &devState, "free", 0); err != nil {
			shutdown(&config, &devState)
			os.Exit(1)
		}
	}

	// We will keep a timer for refreshing the calendar and one for transitioning
	// to the next free/busy state
	refreshTimer := time.NewTicker(time.Hour * 1)

	//
	// Main event loop:
	// 	On incoming signals, indicate light status as requested by signaller
	//  Otherwise, update Google calendar status hourly while active
	//	Update lights based on busy/free status when transition times arrive unless in Zoom
	//
eventLoop:
	for {
		select {
		case _ = <-refreshTimer.C:
			if isActiveNow {
				devState.Logger.Printf("Periodic calendar refresh starts")
				err = busyTimes.Refresh(&config, &devState)
				if err != nil {
					devState.Logger.Printf("Reload failed: %v", err)
				}
				isBusyTimeNow = busyTimes.ScheduledBusyNow(&config, &devState)
				transitionTimer.Stop()
				transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config, &devState)))
			} else {
				devState.Logger.Printf("Ignoring scheduled request to refresh calendar since service isn't active now.")
				refreshTimer.Stop()
			}

		case _ = <-transitionTimer.C:
			devState.Logger.Printf("Scheduled status change")
			isBusyTimeNow = busyTimes.ScheduledBusyNow(&config, &devState)
			transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config, &devState)))

		case externalSignal := <-req:
			switch externalSignal {

			case syscall.SIGVTALRM:
				if !isActiveNow {
					isActiveNow = true
					devState.Logger.Printf("Activating service; re-loading configuration and opening serial port")
					err = setup(&config, &devState)
					if err != nil {
						devState.Logger.Fatalf("Error loading configuration data. Unable to restart: %v", err)
						return
					}
					devState.Logger.Printf("Activating service; getting fresh calendar data")
					err = busyTimes.Refresh(&config, &devState)
					if err != nil {
						devState.Logger.Printf("Error updating busy/free times from calendar: %v", err)
					}
					devState.Logger.Printf("Resetting timers")
					refreshTimer.Reset(1 * time.Hour)
					isBusyTimeNow = busyTimes.ScheduledBusyNow(&config, &devState)
					transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config, &devState)))
				}

			case syscall.SIGHUP:
				devState.Logger.Printf("Call ended")
				isZoomNow = false

			case syscall.SIGUSR1:
				devState.Logger.Printf("Muted")
				isZoomNow = true
				isZoomMuted = true

			case syscall.SIGUSR2:
				devState.Logger.Printf("Unmuted")
				isZoomNow = true
				isZoomMuted = false

			case syscall.SIGWINCH:
				if isActiveNow {
					isActiveNow = false
					devState.Logger.Printf("Stopping timers")
					refreshTimer.Stop()
					transitionTimer.Stop()
					closeDevice(&config, &devState)
					devState.Logger.Printf("Daemon in inactive state... zzz")
				}

			case syscall.SIGPWR:
				if isActiveNow {
					devState.Logger.Printf("Reloading calendar status by request")
					err = busyTimes.Refresh(&config, &devState)
					if err != nil {
						devState.Logger.Printf("Reload failed: %v", err)
					}
					isBusyTimeNow = busyTimes.ScheduledBusyNow(&config, &devState)
					transitionTimer.Stop()
					transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config, &devState)))
				} else {
					devState.Logger.Printf("Ignoring reload request since service isn't active now.")
				}

			case syscall.SIGINT:
				devState.Logger.Printf("Received interrupt signal")
				break eventLoop

			default:
				devState.Logger.Printf("Received unexpeced signal %v (ignored)", externalSignal)
			}
		}

		// Set signal to current state
		if isActiveNow {
			if isZoomNow {
				if isZoomMuted {
					if err := busylight.LightSignal(&config, &devState, "muted", 0); err != nil {
						devState.Logger.Printf("busylight.LightSignal: %v", err)
						shutdown(&config, &devState)
						break
					}
					devState.Logger.Printf("Signal mic MUTED")
				} else {
					if err := busylight.LightSignal(&config, &devState, "open", 0); err != nil {
						devState.Logger.Printf("busylight.LightSignal: %v", err)
						shutdown(&config, &devState)
						break
					}
					devState.Logger.Printf("Signal mic OPEN")
				}
			} else if isBusyTimeNow {
				if err := busylight.LightSignal(&config, &devState, "busy", 0); err != nil {
					devState.Logger.Printf("busylight.LightSignal: %v", err)
					shutdown(&config, &devState)
					break
				}
				devState.Logger.Printf("Signal BUSY")
			} else {
				if err := busylight.LightSignal(&config, &devState, "free", 0); err != nil {
					devState.Logger.Printf("busylight.LightSignal: %v", err)
					shutdown(&config, &devState)
					break
				}
				devState.Logger.Printf("Signal FREE")
			}
		} else {
			if err := busylight.LightSignal(&config, &devState, "off", 0); err != nil {
				devState.Logger.Printf("busylight.LightSignal: %v", err)
				shutdown(&config, &devState)
				break
			}
			devState.Logger.Printf("Signal off")
		}
	}
	_ = busylight.LightSignal(&config, &devState, "off", 0)
}
