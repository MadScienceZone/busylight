//
// vi:set ai sm nu ts=4 sw=4:
//
// Long-running daemon to control the busylight.
// Automatically polls Google calendar busy/free times
// and can be controlled via signals from a Zoom meeting
// monitoring script:
//
//    USR1   - in zoom, muted
//    USR2   - in zoom, unmuted
//    HUP    - out of zoom
//    INFO   - force refresh from calendar now
//    VTALRM - toggle urgent indicator
//    WINCH  - toggle idle/working state
//
// Steve Willoughby <steve@alchemy.com>
// License: BSD 3-Clause open-source license
//

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
	"os/user"
	"path/filepath"
	"sort"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"go.bug.st/serial"
	"os/signal"
	"syscall"
)

type ConfigData struct {
	Calendars      []string
	TokenFile      string
	CredentialFile string
	LogFile        string
	PidFile        string
	Device         string
	BaudRate       int
	googleConfig   []byte
	logger        *log.Logger
}

func getConfigFromFile(filename string, data *ConfigData) error {
	cdata, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Unable to read from %s: %v", filename, err)
	}

	err = json.Unmarshal(cdata, &data)
	if err != nil {
		return fmt.Errorf("Unable to understand %s configuration: %v", filename, err)
	}
	return nil
}

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

type BusyPeriod struct{
	Start, End time.Time
}

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

type CalendarAvailability struct{
	LastPollTime      time.Time
	UpcomingPeriods []BusyPeriod	// will be in chronological order
}

func (cal *CalendarAvailability) RemoveExpiredPeriods(config *ConfigData) {
	for len(cal.UpcomingPeriods) > 0 {
		if time.Now().Add(5 * time.Second).After(cal.UpcomingPeriods[0].End) {
			cal.UpcomingPeriods = cal.UpcomingPeriods[1:]
		} else {
			break
		}
	}
	if len(cal.UpcomingPeriods) == 0 && time.Now().After(cal.LastPollTime.Add(30 * time.Minute)) {
		err := cal.Refresh(config)
		if err != nil {
			config.logger.Printf("Unable to refresh calendar data while removing expired periods: %v", err)
		}
	}
	// yes, we're trusting the Google service not to give us past events.
}

func (cal *CalendarAvailability) NextTransitionTime(config *ConfigData) time.Time {
	cal.RemoveExpiredPeriods(config)

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

func (cal *CalendarAvailability) ScheduledBusyNow(config *ConfigData) bool {
	cal.RemoveExpiredPeriods(config)

	if len(cal.UpcomingPeriods) == 0 {
		return false
	}
	if time.Now().Add(5 * time.Second).After(cal.UpcomingPeriods[0].Start) {
		return true
	}
	return false
}

func (cal *CalendarAvailability) Refresh(config *ConfigData) error {
	config.logger.Printf("Polling Google Calendars")
	googleConfig, err := google.ConfigFromJSON(config.googleConfig, calendar.CalendarReadonlyScope)
	if err != nil { return err }

	client, err := getClient(googleConfig, config.TokenFile)
	if err != nil { return fmt.Errorf("Unable to query calendar: %v", err) }

	srv, err := calendar.New(client)
	if err != nil { return err }

	var query calendar.FreeBusyRequest
	query.TimeMax = time.Now().Add(time.Hour * 8).Format(time.RFC3339)
	query.TimeMin = time.Now().Format(time.RFC3339)
	for _, calId := range config.Calendars {
		query.Items = append(query.Items, &calendar.FreeBusyRequestItem{Id: calId})
	}
	freelist, err := srv.Freebusy.Query(&query).Do()
	if err != nil { return err }

	var rawbusylist []BusyPeriod
	for calId, calData := range freelist.Calendars {
		for _, e := range calData.Errors {
			config.logger.Printf("ERROR: Calendar \"%s\": %v", calId, e)
		}
		for _, busy := range calData.Busy {
			config.logger.Printf("Calendar \"%s\": busy %v - %v", calId, busy.Start, busy.End)
			startTime, err := time.Parse(time.RFC3339, busy.Start)
			if err != nil {
				config.logger.Printf("ERROR: Unable to parse start time \"%v\": %v", busy.Start, err)
				continue
			}
			endTime, err := time.Parse(time.RFC3339, busy.End)
			if err != nil {
				config.logger.Printf("ERROR: Unable to parse end time \"%v\": %v", busy.End, err)
				continue
			}
			rawbusylist = append(rawbusylist, BusyPeriod{Start: startTime, End: endTime})
		}
	}
	// smush list and sort it
	config.logger.Printf("DEBUG: Initial list: %v", rawbusylist)
	sort.Sort(ByStartTime(rawbusylist))
	config.logger.Printf("DEBUG: Sorted list: %v", rawbusylist)
	var currentStart time.Time
	var currentEnd   time.Time

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
	config.logger.Printf("DEBUG: final list: %v", cal.UpcomingPeriods)
	cal.LastPollTime = time.Now()
	return nil
}

//
// we can maintain a list of busy/free times since the last time we polled the calendar.
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

func setup(config *ConfigData) error {
	var thisUser *user.User

	thisUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("Unable to determine current user: %v", err)
	}

	err = getConfigFromFile(filepath.Join(thisUser.HomeDir, ".busylight/config.json"), config)
	if err != nil {
		return fmt.Errorf("Unable to initialize: %v", err)
	}

	f, err := os.OpenFile(config.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("Unable to open logfile: %v", err)
	}
	config.logger = log.New(f, "busylightd: ", log.LstdFlags)

	myPID := os.Getpid()
	config.logger.Printf("busylightd started, PID=%v", myPID)

	pidf, err := os.OpenFile(config.PidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		config.logger.Printf("ERROR creating PID file (is another busylightd running?): %v", err)
		return err
	}
	pidf.WriteString(fmt.Sprintf("%d\n", myPID))
	pidf.Close()

	config.googleConfig, err = ioutil.ReadFile(config.CredentialFile)
	if err != nil {
		config.logger.Printf("Unable to read client secret file %v: %v", config.CredentialFile, err)
		return fmt.Errorf("Unable to read client secret file %v: %v", config.CredentialFile, err)
	}

	return nil
}

func main() {
	var config ConfigData

	if err := setup(&config); err != nil {
		log.Fatalf("Unable to start daemon: %v", err)
	}
	defer func(){
		err := os.Remove(config.PidFile)
		if err != nil {
			config.logger.Printf("Error removing PID file: %v", err)
		}
		config.logger.Printf("busylightd shutting down")
	}()

	//
	// Open the hardware port
	//
	port, err := serial.Open(config.Device, &serial.Mode{
		BaudRate: config.BaudRate,
	})
	if err != nil {
		config.logger.Fatalf("Can't open serial device: %v", err)
	}
	defer port.Close()

	//
	// Listen for incoming signals from outside
	//
	req := make(chan os.Signal, 5)
	signal.Notify(req, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGWINCH, syscall.SIGINFO, syscall.SIGINT, syscall.SIGVTALRM)

	//
	// Signal that we're online and ready
	//
	port.Write([]byte("B"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
	time.Sleep(50 * time.Millisecond)
	port.Write([]byte("B"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))

	//
	// Get initial calendar download
	//
	var busyTimes CalendarAvailability
	err = busyTimes.Refresh(&config)
	if err != nil {
		config.logger.Printf("Error updating busy/free times from calendar: %v", err)
	}

	isZoomNow   := false
	isZoomMuted := false
	isActiveNow := true
	isUrgent    := false

	//
	// Set the current state and schedule for next transition
	//
	isBusyTimeNow := busyTimes.ScheduledBusyNow(&config)
	nextTransitionTime := busyTimes.NextTransitionTime(&config)
	transitionTimer := time.NewTimer(time.Until(nextTransitionTime))

	if isBusyTimeNow {
		port.Write([]byte("Y"))
	} else {
		port.Write([]byte("G"))
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
				config.logger.Printf("Periodic calendar refresh starts")
				err = busyTimes.Refresh(&config)
				if err != nil {
					config.logger.Printf("Reload failed: %v", err)
				}
				isBusyTimeNow = busyTimes.ScheduledBusyNow(&config)
				transitionTimer.Stop()
				transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config)))
			} else {
				config.logger.Printf("Ignoring scheduled request to refresh calendar since service isn't active now.")
				refreshTimer.Stop()
			}

		case _ = <-transitionTimer.C:
			config.logger.Printf("Scheduled status change")
			isBusyTimeNow = busyTimes.ScheduledBusyNow(&config)
			transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config)))

		case externalSignal := <-req:
			switch externalSignal {
			case syscall.SIGVTALRM:
				isUrgent = !isUrgent
				config.logger.Printf("Toggle URGENT indicator to %v", isUrgent)

			case syscall.SIGHUP:
				config.logger.Printf("ZOOM: Call ended")
				isZoomNow = false

			case syscall.SIGUSR1:
				config.logger.Printf("ZOOM: Muted")
				isZoomNow = true
				isZoomMuted = true

			case syscall.SIGUSR2:
				config.logger.Printf("ZOOM: Unmuted")
				isZoomNow = true
				isZoomMuted = false

			case syscall.SIGWINCH:
				config.logger.Printf("Toggle active state")
				isActiveNow = !isActiveNow
				if isActiveNow {
					config.logger.Printf("Activating service; getting fresh calendar data")
					err = busyTimes.Refresh(&config)
					if err != nil {
						config.logger.Printf("Error updating busy/free times from calendar: %v", err)
					}
					config.logger.Printf("Resetting timers")
					refreshTimer.Reset(1 * time.Hour)
					isBusyTimeNow = busyTimes.ScheduledBusyNow(&config)
					transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config)))
				} else {
					config.logger.Printf("Stopping timers")
					refreshTimer.Stop()
					transitionTimer.Stop()
				}

			case syscall.SIGINFO:
				if isActiveNow {
					config.logger.Printf("Reloading calendar status by request")
					err = busyTimes.Refresh(&config)
					if err != nil {
						config.logger.Printf("Reload failed: %v", err)
					}
					isBusyTimeNow = busyTimes.ScheduledBusyNow(&config)
					transitionTimer.Stop()
					transitionTimer.Reset(time.Until(busyTimes.NextTransitionTime(&config)))
				} else {
					config.logger.Printf("Ignoring reload request since service isn't active now.")
				}

			case syscall.SIGINT:
				config.logger.Printf("Received interrupt signal")
				break eventLoop

			default:
				config.logger.Printf("Received unexpeced signal %v (ignored)", externalSignal)
			}
		}

		// Set signal to current state
		if isActiveNow {
			if isUrgent {
				port.Write([]byte("%"))
			} else if isZoomNow {
				if isZoomMuted {
					port.Write([]byte("R"))
					config.logger.Printf("Signal ZOOM MUTED")
				} else {
					port.Write([]byte("#"))
					config.logger.Printf("Signal ZOOM OPEN")
				}
			} else if isBusyTimeNow {
				port.Write([]byte("Y"))
				config.logger.Printf("Signal BUSY")
			} else {
				port.Write([]byte("G"))
				config.logger.Printf("Signal FREE")
			}
		} else {
			port.Write([]byte("X"))
			config.logger.Printf("Signal off")
		}
	}

	//
	// Signal shutdown
	//
	port.Write([]byte("2"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
	time.Sleep(50 * time.Millisecond)
	port.Write([]byte("2"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
}
