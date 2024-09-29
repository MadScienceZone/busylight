package readerboard

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"go.bug.st/serial"
)

type LightList []byte

type LEDSequence struct {
	IsRunning bool
	Position  int
	Sequence  LightList
}

func (v LightList) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(v))
}

func (v *LightList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = []byte(s)
	return nil
}

type EEPROMType byte

const (
	NoEEPROM EEPROMType = iota
	ExternalEEPROM
	InternalEEPROM
)

func (n EEPROMType) MarshalJSON() ([]byte, error) {
	switch n {
	case NoEEPROM:
		return json.Marshal("none")
	case ExternalEEPROM:
		return json.Marshal("external")
	case InternalEEPROM:
		return json.Marshal("internal")
	}
	return nil, fmt.Errorf("Unsupported EEPROMType value %v", n)
}

func (n *EEPROMType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "none":
		*n = NoEEPROM
	case "external", "ext":
		*n = ExternalEEPROM
	case "internal", "int":
		*n = InternalEEPROM
	default:
		return fmt.Errorf("Unsupported EEPROMType value %v", s)
	}
	return nil
}

func parseEEPROMType(e byte) (EEPROMType, error) {
	switch e {
	case 'I':
		return InternalEEPROM, nil
	case 'X':
		return ExternalEEPROM, nil
	case '_':
		return NoEEPROM, nil
	}
	return NoEEPROM, fmt.Errorf("invalid EEPROM code %v", e)
}

func EEPROMTypeName(e EEPROMType) string {
	switch e {
	case InternalEEPROM:
		return "internal"
	case ExternalEEPROM:
		return "external"
	case NoEEPROM:
		return "no"
	default:
		return "UNKNOWN"
	}
}

type DeviceStatus struct {
	DeviceModelClass byte
	DeviceAddress    byte
	GlobalAddress    byte
	SpeedUSB         int
	Speed485         int
	EEPROM           EEPROMType
	HardwareRevision string
	FirmwareRevision string
	Serial           string
	StatusLEDs       DiscreteLEDStatus
	ImageBitmap      [][64]byte
}

type DiscreteLEDStatus struct {
	StatusLights  string
	FlasherStatus LEDSequence
	StroberStatus LEDSequence
}

type NetworkDriver interface {
	Attach(netID, device string, baudRate, globalAddress int) error
	AllLightsOffBytes([]int, []byte) ([]byte, error)
	Bytes([]int, []byte) ([]byte, error)
	Detach()
	Send(string) error
	SendBytes([]byte) error
	Receive() ([]byte, error)
	IsPortOpen() bool
}

type BaseNetworkDriver struct {
	Port          serial.Port
	isPortOpen    bool
	GlobalAddress int
}

type DirectDriver struct {
	BaseNetworkDriver
}

type RS485Driver struct {
	BaseNetworkDriver
}

func (d *DirectDriver) IsPortOpen() bool {
	return d.isPortOpen
}

func (d *DirectDriver) Attach(netID, device string, baudRate, globalAddress int) error {
	var err error
	if d == nil {
		return fmt.Errorf("attach attempted to nil device %s", netID)
	}
	d.Detach()
	d.GlobalAddress = globalAddress
	if d.Port, err = serial.Open(device, &serial.Mode{BaudRate: baudRate}); err == nil {
		d.isPortOpen = true
		err = d.Port.SetReadTimeout(60 * time.Second)
	}
	return err
}

func (d *DirectDriver) Detach() {
	if d.isPortOpen {
		d.Port.Close()
		d.isPortOpen = false
	}
}

func (d *DirectDriver) AllLightsOffBytes(_ []int, command []byte) ([]byte, error) {
	return append(command, 0x04), nil
}

// Bytes produces a raw byte stream appropriate to send the given command to the target addresses on
// a direct USB network.
//
// Since this is a direct connection, addrs is ignored. We simply send the command string as-is,
// followed by the ^D terminator at the end.
// We do not allow ^D to appear in the command string itself.
// As a special case, if command is the empty string, we send "X^DC^D" to clear the whole sign.
func (d *DirectDriver) Bytes(addrs []int, command []byte) ([]byte, error) {
	if bytes.Contains(command, []byte{'\004'}) {
		return nil, fmt.Errorf("cannot send command with a ^D character in it")
	}
	return append(append([]byte{'\004'}, command...), '\004'), nil
}

func (d *RS485Driver) AllLightsOffBytes(a []int, _ []byte) ([]byte, error) {
	if len(a) == 1 && a[0] < 16 {
		return []byte{byte(0x80 | (a[0] & 0x0f))}, nil
	}
	return nil, fmt.Errorf("alloff cannot target more than one device or devices with ID > 15")
}

// Bytes produces a raw byte stream appropriate to send the given command to the target addresses on
// an RS-485 serial network. Bytes after the initial start-of-command byte are escaped.
//
// For RS-485 network-connected devices, a binary header is sent first:
//    1001aaaa                               Single/global target with address in [0,15]
//    1011gggg 00nnnnnn 00aaaa ... 00aaaa    Multiple targets or addresses >15
//                      \_______n_______/
//
func (d *RS485Driver) Bytes(a []int, cmd []byte) ([]byte, error) {
	output485 := Escape485(cmd)

	if len(a) == 0 {
		return nil, fmt.Errorf("command with no target device addresses cannot be sent via RS-485")
	}

	if len(a) == 1 && a[0] < 16 {
		return append([]byte{byte(0x90 | (a[0] & 0x0f))}, output485...), nil
	}

	if len(a) > 63 {
		return nil, fmt.Errorf("number of target device addresses exceeds max of 63")
	}

	addrs := make([]byte, 2+len(a))
	addrs[0] = 0xb0 | (byte(d.GlobalAddress) & 0x0f)
	addrs[1] = byte(len(a) & 0x3f)
	for i := 0; i < len(a); i++ {
		if a[i] < 0 || a[i] > 63 {
			return nil, fmt.Errorf("address %d out of range [0,63]", a[i])
		}
		addrs[2+i] = byte(a[i] & 0x3f)
	}
	return append(addrs, output485...), nil
}

func (d *RS485Driver) Attach(netID, device string, baudRate, globalAddress int) error {
	var err error
	if d == nil {
		return fmt.Errorf("attach attempted to nil device %s", netID)
	}
	d.Detach()
	d.GlobalAddress = globalAddress
	if d.Port, err = serial.Open(device, &serial.Mode{BaudRate: baudRate}); err == nil {
		d.isPortOpen = true
		err = d.Port.SetReadTimeout(1 * time.Second)
	}
	return err
}

func (d *RS485Driver) Detach() {
	if d != nil && d.isPortOpen {
		d.Port.Close()
		d.isPortOpen = false
	}
}

func (d *RS485Driver) IsPortOpen() bool {
	return d.isPortOpen
}

//
// Attach creates an appropriate driver if one doesn't already exist. If there was already an open driver,
// that is closed. Then the port is opened based on the description held in the NetworkDescription fields.
//
func (net *NetworkDescription) Attach(netID string, globalAddress int) error {
	var err error

	if net.driver == nil {
		switch net.ConnectionType {
		case RS485Network:
			net.driver = &RS485Driver{}
			log.Printf("network %s: RS-485 network", netID)
		case USBDirect:
			net.driver = &DirectDriver{}
			log.Printf("network %s: USB direct connection", netID)
		default:
			return fmt.Errorf("network %s: invalid network type %v", netID, net.ConnectionType)
		}
	}

tryOpeningPort:
	for {
		if net.Device != "" {
			// hard-coded device name to use
			if err = net.driver.Attach(netID, net.Device, net.BaudRate, globalAddress); err != nil {
				pe, isPortError := err.(*serial.PortError)
				if isPortError && pe.Code() == serial.PortBusy {
					log.Printf("network %s device %s is busy; retrying...", netID, net.Device)
					time.Sleep(250 * time.Millisecond)
					continue
				}
				return fmt.Errorf("can't open network %s serial device %s: %v", netID, net.Device, err)
			}
			return nil
		}

		// not hard-coded; search for a device matching a regular expression
		log.Printf("network %s: searching for available device port in %s...", netID, net.DeviceDir)
		fileList, err := os.ReadDir(net.DeviceDir)
		if err != nil {
			return fmt.Errorf("can't scan directory %s: %v", net.DeviceDir, err)
		}
		for _, f := range fileList {
			if f.IsDir() {
				continue
			}

			ok, err := regexp.MatchString(net.DeviceRegexp, f.Name())
			if err != nil {
				return fmt.Errorf("matching %s vs %s: %v", f.Name(), net.DeviceRegexp, err)
			}
			if ok {
				if err = net.driver.Attach(netID, fmt.Sprintf("%s%c%s", net.DeviceDir, os.PathSeparator, f.Name()), net.BaudRate, globalAddress); err != nil {
					pe, isPortError := err.(*serial.PortError)
					if isPortError && pe.Code() == serial.PortBusy {
						log.Printf("network %s: found busy device %s; waiting for it to be free...", netID, f.Name())
						time.Sleep(250 * time.Millisecond)
						continue tryOpeningPort
					}
					return fmt.Errorf("can't open network %s serial device %s: %v", netID, f.Name(), err)
				}
				log.Printf("network %s: opened %s%c%s", netID, net.DeviceDir, os.PathSeparator, f.Name())
				return nil
			}
		}
		return fmt.Errorf("network %s: unable to open any device matching /%s/ in %s.", netID, net.DeviceRegexp, net.DeviceDir)
	}
	return fmt.Errorf("network %s: reached unreachable condition (bug?)", netID)
}

func showAddress(a byte) string {
	if a == 0xff {
		return "--"
	}
	return fmt.Sprintf("%d", a)
}

func logStatusLEDs(s DiscreteLEDStatus) {
	log.Printf("| status lights on=%s", s.StatusLights)
	if s.FlasherStatus.IsRunning {
		log.Printf("| flasher running, pos=%d, sequence=%s", s.FlasherStatus.Position, s.FlasherStatus.Sequence)
	} else {
		log.Printf("| flasher stopped")
	}
	if s.StroberStatus.IsRunning {
		log.Printf("| strober running, pos=%d, sequence=%s", s.StroberStatus.Position, s.StroberStatus.Sequence)
	} else {
		log.Printf("| strober stopped")
	}
}

func ProbeDevices(configData *ConfigData) error {
	for nid, net := range configData.Networks {
		if net.driver == nil {
			log.Printf("Network id %s: type=%v; dev=%s; dir=%s; regexp=%s; speed=%d; open=NIL", nid, net.ConnectionType, net.Device, net.DeviceDir, net.DeviceRegexp, net.BaudRate)
		} else {
			log.Printf("Network id %s: type=%v; dev=%s; dir=%s; regexp=%s; speed=%d; open=%v", nid, net.ConnectionType, net.Device, net.DeviceDir, net.DeviceRegexp, net.BaudRate, net.driver.IsPortOpen())
		}
	}
	for id, dev := range configData.Devices {
		var commands, rawBytes []byte
		var received []byte
		var err error
		log.Printf("Device address %d: type=%v; net=%s (%s; s/n=%s)", id, dev.DeviceType, dev.NetworkID, dev.Description, dev.Serial)
		sender, parser := Query()
		if net, ok := configData.Networks[dev.NetworkID]; ok {
			if commands, err = sender(nil, dev.DeviceType); err != nil {
				return fmt.Errorf("error getting bytestream for unit %d: %v", id, err)
			}

			if rawBytes, err = net.driver.Bytes([]int{id}, commands); err != nil {
				return fmt.Errorf("error preparing bytestream for unit %d: %v", id, err)
			}

			if err := func() error {
				lockNetwork(dev.NetworkID)
				defer unlockNetwork(dev.NetworkID)

				if err := net.driver.SendBytes(rawBytes); err != nil {
					return fmt.Errorf("error transmitting bytestream to unit %d: %v", id, err)
				}

				if received, err = net.driver.Receive(); err != nil {
					return fmt.Errorf("device address %d: error reading reply: %v", id, err)
				}
				return nil
			}(); err != nil {
				return err
			}

			if statData, err := parser(dev.DeviceType, received); err == nil {
				if s, ok := statData.(DeviceStatus); ok {
					log.Printf("probed device ID %d on network %s:", id, dev.NetworkID)
					switch s.DeviceModelClass {
					case 'B':
						switch dev.DeviceType {
						case Busylight1:
							log.Printf("| busylight model 1.x; USB speed %d; %s EEPROM; hw %s; fw %s; S/N %s",
								s.SpeedUSB, EEPROMTypeName(s.EEPROM), s.HardwareRevision, s.FirmwareRevision, s.Serial)
							logStatusLEDs(s.StatusLEDs)
						case Busylight2:
							log.Printf("| busylight model 2.x; address %v; global %v; USB speed %d; RS-485 speed %d; %s EEPROM; hw %s; fw %s; S/N %s",
								showAddress(s.DeviceAddress), showAddress(s.GlobalAddress), s.SpeedUSB, s.Speed485, EEPROMTypeName(s.EEPROM), s.HardwareRevision, s.FirmwareRevision, s.Serial)
							logStatusLEDs(s.StatusLEDs)
						default:
							log.Printf("| IDENTIFIES AS A BUSYLIGHT DEVICE REV %s BUT CONFIGURED AS %s!", s.HardwareRevision, HardwareModelName(dev.DeviceType))
						}
					case 'M':
						switch dev.DeviceType {
						case Readerboard3Mono:
							log.Printf("| monochrome readerboard model 3.x; address %v; global %v; USB speed %d; RS-485 speed %d; %s EEPROM; hw %s; fw %s; S/N %s",
								showAddress(s.DeviceAddress), showAddress(s.GlobalAddress), s.SpeedUSB, s.Speed485, EEPROMTypeName(s.EEPROM), s.HardwareRevision, s.FirmwareRevision, s.Serial)
							logStatusLEDs(s.StatusLEDs)
							log.Printf("| bitmap %s", hex.EncodeToString(s.ImageBitmap[0][:]))
							log.Printf("| flash  %s", hex.EncodeToString(s.ImageBitmap[1][:]))
							logMonochromeBitmap(s.ImageBitmap)
						default:
							log.Printf("| IDENTIFIES AS A MONOCHROME READERBOARD DEVICE REV %s BUT CONFIGURED AS %s!", s.HardwareRevision, HardwareModelName(dev.DeviceType))
						}
					case 'C':
						switch dev.DeviceType {
						case Readerboard3RGB:
							log.Printf("| color readerboard model 3.x; address %v; global %v; USB speed %d; RS-485 speed %d; %s EEPROM; hw %s; fw %s; S/N %s",
								showAddress(s.DeviceAddress), showAddress(s.GlobalAddress), s.SpeedUSB, s.Speed485, EEPROMTypeName(s.EEPROM), s.HardwareRevision, s.FirmwareRevision, s.Serial)
							logStatusLEDs(s.StatusLEDs)
							log.Printf("| red plane %s", hex.EncodeToString(s.ImageBitmap[0][:]))
							log.Printf("| green \"   %s", hex.EncodeToString(s.ImageBitmap[1][:]))
							log.Printf("| blue  \"   %s", hex.EncodeToString(s.ImageBitmap[2][:]))
							log.Printf("| flash \"   %s", hex.EncodeToString(s.ImageBitmap[3][:]))
							logColorBitmap(s.ImageBitmap)
						default:
							log.Printf("| IDENTIFIES AS A COLOR READERBOARD DEVICE REV %s BUT CONFIGURED AS %s!", s.HardwareRevision, HardwareModelName(dev.DeviceType))
						}

					default:
						log.Printf("| legacy or unknown device; raw data %s", received)
					}

					if s.DeviceAddress != 0xff && s.DeviceAddress != byte(id) {
						log.Printf("| WARNING! device thinks its address is %d but configured as %d!", s.DeviceAddress, id)
					}
					if s.GlobalAddress != byte(configData.GlobalAddress) {
						log.Printf("| WARNING! device thinks the global address is %d but configured as %d!", s.GlobalAddress, configData.GlobalAddress)
					}
					if s.Serial != dev.Serial {
						log.Printf("| WARNING! device serial number is %s but configured as %s!", s.Serial, dev.Serial)
					}
				} else {
					log.Printf("device address %d: unable to parse raw response %s", id, received)
				}
			} else {
				return fmt.Errorf("device address %d: error parsing output \"%s\": %v", id, received, err)
			}
		} else {
			return fmt.Errorf("device address %d: belongs to network %s but I can't find that network.", id, dev.NetworkID)
		}
	}
	return nil
}

func logColorBitmap(bitmap [][64]byte) {
	for row := 0; row < 8; row++ {
		var s strings.Builder
		for col := 0; col < 64; col++ {
			var bits byte
			if bitmap[0][col]&(1<<row) != 0 {
				bits |= 0x01
			}
			if bitmap[1][col]&(1<<row) != 0 {
				bits |= 0x02
			}
			if bitmap[2][col]&(1<<row) != 0 {
				bits |= 0x04
			}
			if bitmap[3][col]&(1<<row) != 0 {
				bits |= 0x08
			}
			switch bits {
			case 0:
				fmt.Fprintf(&s, ".")
			case 1:
				fmt.Fprintf(&s, "R")
			case 2:
				fmt.Fprintf(&s, "G")
			case 3:
				fmt.Fprintf(&s, "Y")
			case 4:
				fmt.Fprintf(&s, "B")
			case 5:
				fmt.Fprintf(&s, "M")
			case 6:
				fmt.Fprintf(&s, "C")
			case 7:
				fmt.Fprintf(&s, "W")
			case 8:
				fmt.Fprintf(&s, "!")
			case 9:
				fmt.Fprintf(&s, "r")
			case 10:
				fmt.Fprintf(&s, "g")
			case 11:
				fmt.Fprintf(&s, "y")
			case 12:
				fmt.Fprintf(&s, "b")
			case 13:
				fmt.Fprintf(&s, "m")
			case 14:
				fmt.Fprintf(&s, "c")
			case 15:
				fmt.Fprintf(&s, "w")
			default:
				fmt.Fprintf(&s, "?(%02X)", bits)
			}
		}
		log.Printf("| %s |", s.String())
	}
}

func logMonochromeBitmap(bitmap [][64]byte) {
	for row := 0; row < 8; row++ {
		var s strings.Builder
		for col := 0; col < 64; col++ {
			var bits byte
			if bitmap[0][col]&(1<<row) != 0 {
				bits |= 0x01
			}
			if bitmap[1][col]&(1<<row) != 0 {
				bits |= 0x08
			}
			switch bits {
			case 0:
				fmt.Fprintf(&s, ".")
			case 1:
				fmt.Fprintf(&s, "@")
			case 8:
				fmt.Fprintf(&s, "!")
			case 9:
				fmt.Fprintf(&s, "#")
			default:
				fmt.Fprintf(&s, "?")
			}
		}
		log.Printf("| %s |", s.String())
	}
}

func AttachToAllNetworks(configData *ConfigData) error {
	for netID, net := range configData.Networks {
		if err := net.Attach(netID, configData.GlobalAddress); err != nil {
			return err
		}
		configData.Networks[netID] = net
	}
	return nil
}

// Detach closes the hardware connection to the network interface associated with
// this network, if there was one.
func (net *NetworkDescription) Detach() {
	if net != nil && net.driver != nil {
		net.driver.Detach()
	}
}

// Send sends a command string to a device.
func (d *DirectDriver) Send(command string) error {
	return d.SendBytes([]byte(command))
}

func (d *RS485Driver) Send(command string) error {
	return d.SendBytes([]byte(command))
}

func (d *DirectDriver) SendBytes(command []byte) error {
	log.Printf("-> %s", command)
	if _, err := d.Port.Write(command); err != nil {
		log.Printf("%v", err)
		return err
	}
	log.Printf("draining")
	return d.Port.Drain()
}

func (d *RS485Driver) SendBytes(command []byte) error {
	if _, err := d.Port.Write(command); err != nil {
		return err
	}
	return d.Port.Drain()
}

const maxResponseLength = 1024

func (d *DirectDriver) Receive() ([]byte, error) {
	inputbuf := make([]byte, maxResponseLength)
	var rawResponse []byte

	for {
		bytesRead, err := d.Port.Read(inputbuf)
		if err != nil {
			return nil, err
		}
		if bytesRead == 0 {
			return nil, fmt.Errorf("EOF reading from device")
		}

		for i := 0; i < bytesRead; i++ {
			if len(rawResponse) >= maxResponseLength {
				return nil, fmt.Errorf("read more than max %d bytes from device", maxResponseLength)
			}
			if inputbuf[i] == '\n' {
				return rawResponse, nil
			}
			rawResponse = append(rawResponse, inputbuf[i])
		}
	}
}

func (d *RS485Driver) Receive() ([]byte, error) {
	return nil, fmt.Errorf("receive not implemented")
}

func parseLightList(lights []byte) ([]byte, error) {
	return lights, nil
}

func parseLEDSequence(packet []byte) (LEDSequence, error) {
	var seq LEDSequence
	var err error

	if len(packet) < 2 {
		return seq, fmt.Errorf("malformed LED sequence (short)")
	}
	if packet[0] == 'R' {
		seq.IsRunning = true
	} else if packet[0] != 'S' {
		return seq, fmt.Errorf("malformed LED sequence (LED status run state code %v not defined)", packet[0])
	}

	if packet[1] == '_' {
		return seq, nil
	}

	if len(packet) < 3 || packet[2] != '@' {
		return seq, fmt.Errorf("malformed LED sequence (short or missing @)")
	}

	if seq.Position, err = parsePos(packet[1]); err != nil {
		return seq, err
	}

	seq.Sequence = []byte(packet[3:])
	return seq, nil
}

func parsePos(code byte) (int, error) {
	if code == '~' {
		return -1, nil
	}
	if code < 48 || code > 48+63 {
		return 0, fmt.Errorf("code %d out of range ['0','o']", code)
	}
	return int(code) - 48, nil
}
