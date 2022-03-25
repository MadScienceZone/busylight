package busylight

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"time"

	"go.bug.st/serial"
)

// CalendarConfigData provides configuration data which can be specified for each calendar
// being monitored. These are read from the config.json file.
type CalendarConfigData struct {
	Title              string // Arbitrary user-friendly name for the calendar
	IgnoreAllDayEvents bool   // If true, ignore this calendar if booked the whole time
}

// ConfigData holds the configuration specified by the user in the config.json file
// as well as some run-time values we need to refer to throughout the run of the daemon.
type ConfigData struct {
	// A map of all Google calendars being monitored by the daemon.Calendars
	// The key is the Google-provided calendar ID; the value is a CalendarConfigData
	// structure describing what we want to do with that calendar.
	Calendars map[string]CalendarConfigData

	// Definitions of named light effects
	StatusLights map[string]string

	// The path to the file where our access credentials to the calendars is cached.
	TokenFile string

	// The path to the file where our API keys are stored.
	CredentialFile string

	// The path to our logfile where daemon activity is recorded.
	LogFile string

	// The path to the file where we store our PID while we're running.
	PidFile string

	// The path to the serial device we use to communicate with the light hardware.
	Device string

	// If `Device` is empty, then `DeviceDir` specifies a directory to search for
	// the hardware port. The first file we can successfully open that matches
	// the regular expression `DeviceRegexp` will be used.
	DeviceDir    string
	DeviceRegexp string

	// The baud rate at which we communicate with the hardware.
	BaudRate int
}

type DevState struct {
	// These values are used internally by the daemon while it's running.
	GoogleConfig []byte      // unmarshalled data needed for Google API calls
	Logger       *log.Logger // logger open on the requested file
	Port         serial.Port // open serial port device
	PortOpen     bool        // is `port` valid and open now?
}

// lightSignal tells the hardware to signal a particular condition on the lights.
// If `delay` is positive, we wait that long before returning, to make some trivial
// multi-step (but very quick and short-lived) sequences easy to implement.
func LightSignal(config *ConfigData, devState *DevState, color string, delay time.Duration) error {
	// colorCode maps the color strings as passed in to this function to the
	// actual commands sent to the hardware.
	// The "color" is the name of a defined pattern from the "StatusLights"
	// entry in the config file.

	var defaultColorCode = map[string]string{
		"start": "S0",   // flashed twice as daemon comes online
		"stop":  "S1",   // flashed twice as daemon goes offline
		"off":   "X",    // turn off all lights
		"busy":  "S3",   // signal that the user is busy
		"free":  "S4",   // signal that the user is free
		"muted": "S2",   // in meeting with mic muted
		"open":  "F12$", // in meeting with mic open
	}

	command, ok := config.StatusLights[color]
	if !ok {
		command, ok = defaultColorCode[color]
		if !ok {
			return fmt.Errorf("undefined color code \"%v\"", color)
		}
	}

	return RawLightSignal(config, devState, command, delay)
}

func RawLightSignal(config *ConfigData, devState *DevState, command string, delay time.Duration) error {
	if !devState.PortOpen {
		if err := AttachToLight(config, devState); err != nil {
			return err
		}
		defer DetachFromLight(devState)
	}
	devState.Port.Write([]byte(command))
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

func GetConfigFromFile(filename string, data *ConfigData) error {
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

func AttachToLight(config *ConfigData, devState *DevState) error {
	var err error

	//
	// Open the hardware port
	//
	if devState.PortOpen {
		devState.Port.Close()
		devState.PortOpen = false
	}

tryOpeningPort:
	for !devState.PortOpen {
		// If the user had a specific port in mind, just use that.
		if config.Device != "" {
			devState.Port, err = serial.Open(config.Device, &serial.Mode{
				BaudRate: config.BaudRate,
			})
			if err != nil {
				pe, isPortError := err.(*serial.PortError)
				if isPortError && pe.Code() == serial.PortBusy {
					devState.Logger.Printf("light device is busy; retrying...")
					time.Sleep(250 * time.Millisecond)
					continue tryOpeningPort
				}
				return fmt.Errorf("can't open serial device %v: %v", config.Device, err)
			}
			devState.PortOpen = true
		} else {
			// On the other hand, maybe we should hunt around to find it.
			// This is necessary on systems where the USB port is given a
			// random device name every time.
			devState.Logger.Printf("Searching for available device port in %s...", config.DeviceDir)
			fileList, err := os.ReadDir(config.DeviceDir)
			if err != nil {
				return fmt.Errorf("can't scan directory %s: %v", config.DeviceDir, err)
			}
			for _, f := range fileList {
				if !f.IsDir() {
					ok, err := regexp.MatchString(config.DeviceRegexp, f.Name())
					if err != nil {
						return fmt.Errorf("Matching %s vs %s: %v", f.Name(), config.DeviceRegexp, err)
					}
					if ok {
						devState.Port, err = serial.Open(fmt.Sprintf("%s%c%s", config.DeviceDir, os.PathSeparator, f.Name()),
							&serial.Mode{BaudRate: config.BaudRate})
						if err != nil {
							pe, isPortError := err.(*serial.PortError)
							if isPortError && pe.Code() == serial.PortBusy {
								devState.Logger.Printf("found light device %s; waiting for it to be free...", f.Name())
								time.Sleep(250 * time.Millisecond)
								continue tryOpeningPort
							} else {
								devState.Logger.Fatalf("error opening %s: %v", f.Name(), err)
							}
						} else {
							devState.Logger.Printf("Opened %s%c%s", config.DeviceDir, os.PathSeparator, f.Name())
							devState.PortOpen = true
							break
						}
					}
				}
			}
			if !devState.PortOpen {
				return fmt.Errorf("unable to open any device matching /%s/ in %s.", config.DeviceRegexp, config.DeviceDir)
			}
		}
	}
	return nil
}

func DetachFromLight(devState *DevState) {
	if devState.PortOpen {
		devState.Port.Close()
		devState.PortOpen = false
	}
}
