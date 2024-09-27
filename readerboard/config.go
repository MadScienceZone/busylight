package readerboard

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"sync"
)

// ConfigData holds the configuration specified by the user in the config.json file
// as well as some run-time values we need to refer to throughout the run of the daemon.
type ConfigData struct {
	// The designated global address, in the range 0-15. Commands directed to this
	// address will be sent to all devices. Each address MUST have this number
	// configured into their operating parameters.  The factory default setting
	// for the global address for busylight units and readerboards is 15.
	GlobalAddress int

	// The list of known devices which the server is controlling, indexed by the device
	// address. Addresses are arbitrarily assigned values in the range 0-63. There is a
	// slight protocol efficiency gain for giving addresses 0-15 to devices connected
	// via RS-485.
	Devices DevMap

	// The list of known interfaces over which we communicate with the devices,
	// indexed by arbitrarily-assigned names
	Networks map[string]NetworkDescription

	// The path to our logfile where server activity is recorded.
	LogFile string

	// The path to the file where we store our PID while we're running.
	PidFile string

	// Server endpoint
	Endpoint string
}

type DevMap map[int]DeviceDescription

func (dm DevMap) MarshalJSON() ([]byte, error) {
	sm := make(map[string]DeviceDescription)
	for k, v := range dm {
		sm[strconv.Itoa(k)] = v
	}
	return json.Marshal(sm)
}

func (dm *DevMap) UnmarshalJSON(b []byte) error {
	sm := make(map[string]DeviceDescription)
	if err := json.Unmarshal(b, &sm); err != nil {
		return err
	}
	*dm = make(DevMap)
	for k, v := range sm {
		sk, err := strconv.Atoi(k)
		if err != nil {
			return err
		}
		(*dm)[sk] = v
	}
	return nil
}

// NetworkDescription describes each hardware interface that our devices are
// connected to.
type NetworkDescription struct {
	// Is this an RS-485 or USB connection?
	ConnectionType NetworkType

	// The path to the serial device we use to communicate with the light hardware.
	Device string

	// If `Device` is empty, then `DeviceDir` specifies a directory to search for
	// the hardware port. The first file we can successfully open that matches
	// the regular expression `DeviceRegexp` will be used.
	DeviceDir    string
	DeviceRegexp string

	// The baud rate at which we communicate with the hardware.
	BaudRate int

	driver NetworkDriver
}

var networkLocks map[string]*sync.Mutex

func lockNetwork(id string) {
	networkLocks[id].Lock()
}

func unlockNetwork(id string) {
	networkLocks[id].Unlock()
}

func createNetworkLock(id string) {
	networkLocks[id] = &sync.Mutex{}
}

func init() {
	networkLocks = make(map[string]*sync.Mutex)
}

type NetworkType byte

const (
	RS485Network NetworkType = iota
	USBDirect
)

func (n NetworkType) MarshalJSON() ([]byte, error) {
	switch n {
	case RS485Network:
		return json.Marshal("RS-485")
	case USBDirect:
		return json.Marshal("USB")
	}
	return nil, fmt.Errorf("Unsupported NetworkType value %v", n)
}

func (n *NetworkType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "RS-485", "RS485", "rs485", "rs-485", "485":
		*n = RS485Network
	case "USB", "usb":
		*n = USBDirect
	default:
		return fmt.Errorf("Unsupported NetworkTypoe value %v", s)
	}
	return nil
}

// DeviceDescription holds the configuration data for each individual busylight or readerboard
// device managed by the server.
type DeviceDescription struct {
	// What kind of device is this? This may be changed by the server if the
	// device itself reports that it is a different type or version.
	DeviceType HardwareModel

	// Network ID (the key in the ConfigData Networks map) where this is attached.
	NetworkID string

	// Free-form description of the device
	Description string

	// Device serial number. If this is non-empty, a warning will be logged if the
	// device at this address doesn't report this serial number back, since this
	// may indicate that the wrong device is configured at this target address.
	Serial string
}

type HardwareModel byte

const (
	Busylight1       HardwareModel = iota // Busylight model 1.x, USB only
	Busylight2                            // Busylight model 2.x, USB or RS-485
	Readerboard3RGB                       // Readerboard model 3.x, USB or RS-485, RGB 64x8 matrix plus 8-light busylight status LEDs
	Readerboard3Mono                      // Readerboard model 3.x, USB or RS-485, monochrome 64x8 matrix plus 8-light busylight status LEDs
)

func BusylightModelVersion(hw HardwareModel) int {
	switch hw {
	case Busylight1:
		return 1
	case Busylight2:
		return 2
	}
	return 0
}

func IsBusylightModel(hw HardwareModel) bool {
	return hw == Busylight1 || hw == Busylight2
}

func IsReaderboardModel(hw HardwareModel) bool {
	return hw == Readerboard3RGB || hw == Readerboard3Mono
}

func IsReaderboardMonochrome(hw HardwareModel) bool {
	return hw == Readerboard3Mono
}

func (m HardwareModel) MarshalJSON() ([]byte, error) {
	switch m {
	case Busylight1:
		return json.Marshal("Busylight1")
	case Busylight2:
		return json.Marshal("Busylight2")
	case Readerboard3RGB:
		return json.Marshal("Readerboard3_RGB")
	case Readerboard3Mono:
		return json.Marshal("Readerboard3_Monochrome")
	}
	return nil, fmt.Errorf("Unsupported HardwareModel value %v", m)
}

func (m *HardwareModel) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "Busylight1.x", "Busylight1":
		*m = Busylight1
	case "Busylight2", "Busylight2.x", "Busylight2.0", "Busylight2.1", "Busylight":
		*m = Busylight2
	case "Readerboard3", "Readerboard", "Readerboard3_RGB", "Readerboard3RGB", "ReaderboardRGB", "Readerboard_RGB":
		*m = Readerboard3RGB
	case "Readerboard3Mono", "ReaderboardMono", "Readerboard3_Mono", "Readerboard3Monochrome", "ReaderboardMonochrome", "Readerboard3_Monochrome":
		*m = Readerboard3Mono
	default:
		return fmt.Errorf("Unsupported HardwareModel value %v", s)
	}
	return nil
}

func HardwareModelName(m HardwareModel) string {
	if s, err := m.MarshalJSON(); err == nil {
		return string(s)
	}
	return "unknown"
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

	for id, _ := range data.Networks {
		createNetworkLock(id)
	}
	return nil
}
