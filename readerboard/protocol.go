//
// Protocol support for busylight/readerboard device communications.
// The protocol and command set are more thoroughly documented in the accompanying
// doc/readerboard.pdf document.
//
//
package readerboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//
// parseBaudRateCode reads a one-byte baud rate code and returns the baud rate it
// represents.
//
func parseBaudRateCode(code byte) (int, error) {
	switch code {
	case '0':
		return 300, nil
	case '1':
		return 600, nil
	case '2':
		return 1200, nil
	case '3':
		return 2400, nil
	case '4':
		return 4800, nil
	case '5':
		return 9600, nil
	case '6':
		return 14400, nil
	case '7':
		return 19200, nil
	case '8':
		return 28800, nil
	case '9':
		return 31250, nil
	case 'A':
		return 38400, nil
	case 'B':
		return 57600, nil
	case 'C':
		return 115200, nil
	default:
		return 0, fmt.Errorf("invalid baud rate code")
	}
}

//
// Escape485 translates arbitrary 8-bit byte sequences to be sent over RS-485 so
// they conform with the 7-bit data constraint imposed by the protocol.
// Since the protocol uses the MSB to indicate the start of a new
// command, that byte can't be escaped using this function since it MUST have its MSB set.
//
func Escape485(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for _, b := range in {
		if b == 0x7e || b == 0x7f {
			// byte is one of our escape codes; escape it.
			out = append(out, 0x7f)
			out = append(out, b)
		} else if (b & 0x80) != 0 {
			// MSB set: send 7E then the byte without the MSB
			out = append(out, 0x7e)
			out = append(out, b&0x7f)
		} else {
			out = append(out, b)
		}
	}
	return out
}

//
// Unescape485 is the inverse of Escape485; it resolves the escape bytes in the byte sequence
// passed to it, returning the original full-8-bit data stream as it was before escaping.
//
func Unescape485(in []byte) []byte {
	out := make([]byte, 0, len(in))
	literalNext := false
	setNextMSB := false

	for _, b := range in {
		if literalNext {
			out = append(out, b)
			literalNext = false
			continue
		}
		if setNextMSB {
			out = append(out, b|0x80)
			setNextMSB = false
			continue
		}

		switch b {
		case 0x7f:
			literalNext = true
			continue

		case 0x7e:
			setNextMSB = true
			continue

		default:
			out = append(out, b)
		}
	}
	return out
}

//
// reqInit initializes, and reads the target list from, the client's posted data.
// It returns a slice of integer device target numbers or an error if that couldn't happen.
//
func reqInit(r *http.Request, globalAddress int) ([]int, error) {
	var targets []int

	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	if !r.Form.Has("a") {
		return nil, fmt.Errorf("request missing device target address list")
	}

	for i, target := range strings.Split(r.Form.Get("a"), ",") {
		t, err := strconv.Atoi(target)
		if err != nil {
			return nil, fmt.Errorf("request device target #%d invalid: %v", i, err)
		}
		if t < 0 || t > 63 {
			return nil, fmt.Errorf("request device target #%d value %d out of range [0,63]", i, t)
		}

		// If the global addr is anywhere in the list, return a list with ONLY that address
		if t == globalAddress {
			return []int{globalAddress}, nil
		}
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("request device target address list is empty")
	}

	return targets, nil
}

//
// ledList extracts the LED list from the parameter "l" in the HTTP request.
// This is expected to be a string of individual 7-bit ASCII characters whose meanings
// are device dependent.
//
// Returns the list of LED codes followed by a terminating '$' character. The list may
// be the empty string.
//
// In order for this to work, the form data must already be in the http.Request field Form
// which can be arranged by calling reqInit first.
//
func ledList(r url.Values) ([]byte, error) {
	leds := r.Get("l")
	llist := []byte(leds)

	for i, ch := range leds {
		if ch < 32 || ch > 127 {
			return nil, fmt.Errorf("LED #%d ID %d out of range [32,127]", i, ch)
		}
		if ch == '$' {
			return nil, fmt.Errorf("LED #%d ID not allowed to be '$'", i)
		}
	}

	return append(llist, '$'), nil
}

type JSONErrorResponse struct {
	Status  string
	Errors  int
	Message string
}

func WrapReplyHandler(f func() (func(url.Values, HardwareModel) ([]byte, error), func(HardwareModel, []byte) (any, error)), config *ConfigData) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		sender, _ := f()
		errors := sendCommandToHardware(sender, r, config)
		if errors > 0 {
			resp, _ := json.Marshal(JSONErrorResponse{
				Errors:  errors,
				Message: "Failed to send command to target hardware device.",
			})
			io.WriteString(w, string(resp)+"\n")
			return
		}

		io.WriteString(w, "not implemented\n")
	}
}

type netTargetKey struct {
	NetworkID  string
	DeviceType HardwareModel
}

func WrapHandler(f func(url.Values, HardwareModel) ([]byte, error), config *ConfigData) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if errors := sendCommandToHardware(f, r, config); errors > 0 {
			io.WriteString(w, fmt.Sprintf("%d error%s occurred while trying to carry out this operation.\n",
				errors, func(n int) string {
					if n == 1 {
						return ""
					} else {
						return "s"
					}
				}(errors)))
		}
	}
}

func sendCommandToHardware(f func(url.Values, HardwareModel) ([]byte, error), r *http.Request, config *ConfigData) int {
	var rawBytes []byte
	var err error

	errors := 0
	targets, err := reqInit(r, config.GlobalAddress)
	if err != nil {
		log.Printf("invalid request: %v", err)
		return 1
	}

	// organize our target list by the networks they're attached to, grouped together by device model
	// so that we can optimize by sending a single multi-target command where that sort of thing is
	// possible but send separate commands to targets when we need to.
	targetNetworks := make(map[netTargetKey][]int)
	if targets[0] == config.GlobalAddress {
		// add everything
		for target, dev := range config.Devices {
			targetNetworks[netTargetKey{dev.NetworkID, dev.DeviceType}] = append(targetNetworks[netTargetKey{dev.NetworkID, dev.DeviceType}], target)
		}
	} else {
		for _, target := range targets {
			dev, isInConfig := config.Devices[target]
			if !isInConfig {
				errors++
				log.Printf("command targets device with ID %d, but that device does not exist in the server's configuration (ignored)", target)
				continue
			}
			targetNetworks[netTargetKey{dev.NetworkID, dev.DeviceType}] = append(targetNetworks[netTargetKey{dev.NetworkID, dev.DeviceType}], target)
		}
	}

	// Try sending the commands to the devices
	for targetNetwork, targetList := range targetNetworks {
		commands, err := f(r.Form, targetNetwork.DeviceType)
		if err != nil {
			errors++
			log.Printf("error preparing request for %s: %v", targetNetwork.NetworkID, err)
			continue
		}
		if len(commands) < 1 {
			log.Printf("internal error preparing bytestream for %s: nil output", targetNetwork.NetworkID)
			errors++
			continue
		}

		if commands[0] == 0xff {
			// special case for "all lights off" command
			rawBytes, err = config.Networks[targetNetwork.NetworkID].driver.AllLightsOffBytes(targetList, commands[1:])
			if err != nil {
				if b, err := Off(nil, targetNetwork.DeviceType); err != nil {
					if rawBytes, err = config.Networks[targetNetwork.NetworkID].driver.Bytes(targetList, b); err != nil {
						if b, err = Clear(nil, targetNetwork.DeviceType); err != nil {
							if rb, err := config.Networks[targetNetwork.NetworkID].driver.Bytes(targetList, b); err != nil {
								rawBytes = append(rawBytes, rb...)
							} else {
								errors++
								log.Printf("error preparing bytestream for %s (plan B CLEAR can't be set up): %v", targetNetwork.NetworkID, err)
								continue
							}
						}
					} else {
						errors++
						log.Printf("error preparing bytestream for %s (plan B OFF can't be set up): %v", targetNetwork.NetworkID, err)
						continue
					}
				} else {
					errors++
					log.Printf("error preparing bytestream for %s (plans A and B both failed): %v", targetNetwork.NetworkID, err)
					continue
				}
			}
		} else {
			rawBytes, err = config.Networks[targetNetwork.NetworkID].driver.Bytes(targetList, commands)
			if err != nil {
				errors++
				log.Printf("error preparing bytestream for %s: %v", targetNetwork.NetworkID, err)
				continue
			}
		}

		if err = config.Networks[targetNetwork.NetworkID].driver.SendBytes(rawBytes); err != nil {
			errors++
			log.Printf("error transmitting bytestream to %s: %v", targetNetwork.NetworkID, err)
			continue
		}
	}
	return errors
}

func boolParam(r url.Values, key string, ifFalse, ifTrue byte) byte {
	if !r.Has(key) {
		return ifFalse
	}

	v := r.Get(key)
	if v == "" || v == "true" || v == "yes" || v == "on" {
		return ifTrue
	}
	return ifFalse
}

func intParam(r url.Values, key string) (int, error) {
	v := r.Get(key)
	if v == "" {
		return 0, nil
	}
	return strconv.Atoi(v)
}

func textParam(r url.Values) ([]byte, error) {
	t := r.Get("t")
	text := make([]byte, 0, len(t)+1)
	// This can't just be UTF-8 encoded; it is just a series of 8-bit character values
	// and we disallow codepoints > 255.
	for _, ch := range t {
		if ch == 0o33 || ch == 4 {
			return nil, fmt.Errorf("text parameter contains illegal character(s)")
		}
		if ch <= 255 {
			text = append(text, byte(ch&0xff))
		}
	}
	return append(text, 0o33), nil
}

func posParam(r url.Values) (byte, error) {
	pos := r.Get("pos")
	if len(pos) != 1 {
		return 0, fmt.Errorf("position must be a single character")
	}
	if (pos[0] < '0' || pos[0] > 'o') && pos[0] != '~' {
		return 0, fmt.Errorf("position %q out of range ['0','o'] or '~'", pos)
	}
	return pos[0], nil
}

//
// AllLightsOff turns all lights off on the specified device(s). This extinguishes the status LEDs
// and the matrix.
//    /readerboard/v1/alloff?a=<targets>
//
// This is a bit of a hack: the initial 0xff byte signals that this is an AllLightsOff
// signal which can be very different on RS-485 networks; for direct connections this
// can be sent as multiple commands. So we assume RS-485 drivers completely ignore our
// output and USB ones ignore the first byte and allow embedded ^D terminators.
func AllLightsOff(_ url.Values, hw HardwareModel) ([]byte, error) {
	if IsBusylightModel(hw) {
		return []byte{0xff, 'X'}, nil
	}
	return []byte{0xff, 'C', 0x04, 'X'}, nil
}

//
// Clear turns off all the LEDs in the display matrix.
//    /readerboard/v1/clear?a=<targets>
//    -> C
//
func Clear(_ url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("clear command not supported for hardware type %v", hw)
	}
	return []byte{'C'}, nil
}

//
// Test runs a test pattern on the target device.
//    /readerboard/v1/test?a=<targets>
//
func Test(_ url.Values, hw HardwareModel) ([]byte, error) {
	if IsReaderboardModel(hw) || BusylightModelVersion(hw) > 1 {
		return []byte{'%'}, nil
	}
	return nil, fmt.Errorf("test command not supported for hardware type %v", hw)
}

//
// Flash sets a flash pattern on the busylight status LEDs.
//    /readerboard/v1/flash?a=<targets>&l=<leds>
//    -> F l0 l1 ... lN $
//
func Flash(r url.Values, _ HardwareModel) ([]byte, error) {
	l, err := ledList(r)
	if err != nil {
		return nil, err
	}

	return append([]byte{'F'}, l...), nil
}

//
// Font selection to indexed font table.
//    /readerboard/v1/font?a=<targets>&idx=<digit>
//    -> A digit
//
func Font(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("font command not supported for hardware type %v", hw)
	}
	idx := r.Get("idx")
	if len(idx) != 1 || idx[0] < '0' || idx[0] > '9' {
		return nil, fmt.Errorf("font index %q out of range [0,9]", idx)
	}
	return []byte{'A', idx[0]}, nil
}

//
// Graph plots a histogram graph data point on the display.
//    /readerboard/v1/graph?a=<targets>&v=<n>[&colors=<rgb>...]
//    -> H n
//    -> H K rgb0 ... rgb7
//
func Graph(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("graph command not supported for hardware type %v", hw)
	}
	if r.Has("colors") {
		rgb := r.Get("colors")
		if len(rgb) != 8 {
			return nil, fmt.Errorf("colors parameter requires eight values")
		}
		for i := 0; i < len(rgb); i++ {
			if rgb[i] < '0' || rgb[i] > '?' {
				return nil, fmt.Errorf("colors parameter value $%d %q out of range", i, rgb[i])
			}
		}
		return append([]byte{'H', 'K'}, rgb...), nil
	}

	value, err := intParam(r, "v")
	if err != nil {
		return nil, err
	}
	if value < 0 {
		value = 0
	} else if value > 8 {
		value = 8
	}
	return []byte{'H', byte(value + '0')}, nil
}

//
// Bitmap displays a bitmap image on the display
//    /readerboard/v1/bitmap?a=<targets>[&merge=<bool]&pos=<pos>[&trans=<trans>]&image=<redcols>$<greencols>$<bluecols>$<flashcols>
//    -> I M/. pos trans R0 ... RN $ G0 ... GN $ B0 ... BN $ F0 ... FN $
//
func Bitmap(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("bitmap command not supported for hardware type %v", hw)
	}
	merge := boolParam(r, "merge", '.', 'M')
	trans := r.Get("trans")
	if trans == "" {
		trans = "."
	}
	if len(trans) != 1 {
		return nil, fmt.Errorf("transition code must be a single character")
	}
	image := r.Get("image")
	pos, err := posParam(r)
	if err != nil {
		return nil, err
	}

	currentColor := "red"
	savedLength := 1
	for i := 0; i < len(image); i++ {
		if image[i] == '$' {
			if ((i - savedLength - 1) % 2) != 0 {
				return nil, fmt.Errorf(fmt.Sprintf("the %s color plane is not an even number of hex digits", currentColor))
			}
			savedLength = i

			switch currentColor {
			case "red":
				if IsReaderboardMonochrome(hw) {
					currentColor = "flashing"
				} else {
					currentColor = "green"
				}
			case "green":
				currentColor = "blue"
			case "blue":
				currentColor = "flashing"
			case "flashing":
				return nil, fmt.Errorf("too many color planes or separators")
			}
		} else {
			if !((image[i] >= '0' && image[i] <= '9') || (image[i] >= 'a' && image[i] <= 'f') || (image[i] >= 'A' && image[i] <= 'F')) {
				return nil, fmt.Errorf("invalid hex character %q in %s image bitplane", image[i], currentColor)
			}
		}
	}
	if currentColor != "flashing" {
		return nil, fmt.Errorf("not enough color bitplanes provided (ended at %s)", currentColor)
	}
	return append(append([]byte{'I', merge, pos, trans[0]}, []byte(image)...), '$'), nil
}

//
// Color sets the current drawing color.
//    /readerboard/v1/color?a=<targets>&color=<rgb>
//    -> K rgb
//
func Color(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("color command not supported for hardware type %v", hw)
	}
	color := r.Get("color")
	if color == "" {
		color = "1"
	}
	if len(color) != 1 {
		return nil, fmt.Errorf("color codes must be a single character")
	}
	if color[0] < '0' || color[0] > '?' {
		return nil, fmt.Errorf("invalid color code")
	}
	return []byte{'K', color[0]}, nil
}

//
// Move repositions the text cursor.
//    /readerboard/v1/move?a=<targets>&pos=<pos>
//    -> @ pos
//
func Move(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("move command not supported for hardware type %v", hw)
	}
	pos, err := posParam(r)
	if err != nil {
		return nil, err
	}
	return []byte{'@', pos}, nil
}

//
// Off turns off the status LEDs.
//    /readerboard/v1/off?a=<targets>
//    -> X
//
func Off(r url.Values, _ HardwareModel) ([]byte, error) {
	return []byte{'X'}, nil
}

//
// Scroll scrolls a text message across the display.
//    /readerboard/v1/scroll?a=<targets>&t=<text>[&loop=<bool>]
//    -> < L/. text
//
func Scroll(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("scroll command not supported for hardware type %v", hw)
	}
	loop := boolParam(r, "loop", '.', 'L')
	t, err := textParam(r)
	if err != nil {
		return nil, err
	}
	return append([]byte{'<', loop}, t...), nil
}

//
// Text displays a text message on the display.
//    /readerboard/v1/text?a=<targets>&t=<text>[&merge=<bool>][&align=<align>][&trans=<trans>]
//    -> T M/. . trans text
//
func Text(r url.Values, hw HardwareModel) ([]byte, error) {
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("text command not supported for hardware type %v", hw)
	}
	merge := boolParam(r, "merge", '.', 'M')
	align := r.Get("align")
	trans := r.Get("trans")
	text, err := textParam(r)
	if err != nil {
		return nil, err
	}
	if align == "" {
		align = "<"
	}
	if trans == "" {
		trans = "."
	}
	if len(align) != 1 {
		return nil, fmt.Errorf("alignment value must be a single character")
	}
	if len(trans) != 1 {
		return nil, fmt.Errorf("transition value must be a single character")
	}
	return append([]byte{'T', merge, align[0], trans[0]}, text...), nil
}

//
// Light sets a static pattern on the busylight status LEDs.
//    /readerboard/v1/light?a=<targets>&l=<leds>
//    -> L l0 l1 ... lN $
//
func Light(r url.Values, hw HardwareModel) ([]byte, error) {
	l, err := ledList(r)
	if err != nil {
		return nil, err
	}
	if len(l) == 2 {
		return []byte{'S', l[0]}, nil
	}
	if !IsReaderboardModel(hw) {
		return nil, fmt.Errorf("light command with more than one lit LED not supported for hardware type %v", hw)
	}
	return append([]byte{'L'}, l...), nil
}

//
// Strobe sets a strobe pattern on the busylight status LEDs.
//    /readerboard/v1/strobe?a=<targets>&l=<leds>
//    -> * l0 l1 ... ln $
//
func Strobe(r url.Values, _ HardwareModel) ([]byte, error) {
	l, err := ledList(r)
	if err != nil {
		return nil, err
	}
	return append([]byte{'*'}, l...), nil
}

func extractString(src []byte, idx int, prefix string) (string, int, error) {
	if idx >= len(src) {
		return "", idx, fmt.Errorf("expected string field not found in data")
	}
	if prefix != "" {
		if !bytes.HasPrefix(src[idx:], []byte(prefix)) {
			return "", idx, fmt.Errorf("missing expected \"%s\" for string field", prefix)
		}
		idx += len(prefix)
	}
	if end := bytes.IndexAny(src[idx:], "$\033"); end >= 0 {
		return string(src[idx : idx+end]), idx + end + 1, nil
	}
	return "", idx, fmt.Errorf("missing string field terminator")
}

func parseFlasherStatus(src string) (LEDSequence, error) {
	if len(src) < 2 {
		return LEDSequence{}, fmt.Errorf("flasher sequence data too short")
	}
	seq := LEDSequence{}

	if src[0] == 'R' {
		seq.IsRunning = true
	} else if src[0] != 'S' {
		return seq, fmt.Errorf("flasher sequence data invalid: run state value %v", src[0])
	}

	if src[1] == '_' {
		return seq, nil
	}

	if len(src) < 3 || src[2] != '@' {
		return seq, fmt.Errorf("flasher sequence data invalid: can't read position marker")
	}

	if src[1] < '0' || src[1] > 'o' {
		return seq, fmt.Errorf("flasher sequence data invalid: position %v out of range", src[1])
	}
	seq.Position = int(src[1]) - '0'
	seq.Sequence = []byte(src[3:])
	return seq, nil
}

func parseBitmapPlane(hex string) ([64]byte, error) {
	var b [64]byte

	if len(hex)%2 != 0 {
		return b, fmt.Errorf("hex string must have even number of characters")
	}
	if len(hex) > 128 {
		return b, fmt.Errorf("hex string too long")
	}

	for i := 0; i < 64 && i*2+2 <= len(hex); i++ {
		ui, err := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if err != nil {
			return b, fmt.Errorf("hex byte at index %d (%s) is invalid: %v", i*2, hex[i*2:i*2+2], err)
		}
		b[i] = byte(ui)
	}
	return b, nil
}

//
// Query inquires about the device status and returns a JSON represenation of the status
//    /readerboard/v1/query?a=<targets>&status
//    -> ?
//    <- L l0 l1 ... lN $ F R/S _/{pos @ l0 l1 ... lN} $ S R/S _/{pos @ l0 l1 ... lN} $ \n
//    /readerboard/v1/query?a=<targets>&status
//    -> Q
//    <- Q B = ad uspd rspd glb I/X/_ $ L ... $ V vers $ R vers $ S sn $ \n
//    <- Q C = ad uspd rspd glb I/X/_ $ L ... $ V vers $ R vers $ S sn $ M red... $ green... $ blue... $ flash... $ \n
//    <- Q M = ad uspd rspd glb I/X/_ $ L ... $ V vers $ R vers $ S sn $ M bits... $ flash... $ \n
//
//    (485)  1101aaaa ...
//           1111gggg 00000001 00aaaaaa ...
//
// This is an extra level of abstraction than the non-reply commands; Query() returns two functions.
// The first is like the other (non-reply) ones that would be passed to WrapHandler; the second is a parser
// function that will be called to parse and validate the data received back from the device.

func parseStatusLEDs(in []byte, idx int) (DiscreteLEDStatus, int, error) {
	var fstat string
	var err error
	stat := DiscreteLEDStatus{}
	if stat.StatusLights, idx, err = extractString(in, idx, "L"); err != nil {
		return stat, idx, fmt.Errorf("status query response status light string could not be extracted (%v)", err)
	}
	if fstat, idx, err = extractString(in, idx, "F"); err != nil {
		return stat, idx, fmt.Errorf("status query response flasher string could not be extracted (%v)", err)
	}
	if stat.FlasherStatus, err = parseFlasherStatus(fstat); err != nil {
		return stat, idx, fmt.Errorf("status query response flasher string could not be parsed (%v)", err)
	}
	if fstat, idx, err = extractString(in, idx, "S"); err != nil {
		return stat, idx, fmt.Errorf("status query response strober string could not be extracted (%v)", err)
	}
	if stat.StroberStatus, err = parseFlasherStatus(fstat); err != nil {
		return stat, idx, fmt.Errorf("status query response strober string could not be parsed (%v)", err)
	}
	return stat, idx, nil
}

func QueryStatus() (func(url.Values, HardwareModel) ([]byte, error), func(HardwareModel, []byte) (any, error)) {
	return func(_ url.Values, _ HardwareModel) ([]byte, error) {
			return []byte{'?'}, nil
		}, func(hw HardwareModel, in []byte) (any, error) {
			// parse the response data, returning the data structure represented or the number of additional bytes
			// still needed before we can have a successful read
			var err error
			var idx int

			if len(in) < 9 {
				return DiscreteLEDStatus{}, fmt.Errorf("query response from hardware too short (%d)", len(in))
			}

			stat, idx, err := parseStatusLEDs(in, 0)
			if err != nil {
				return stat, fmt.Errorf("status query response not understood: %v", err)
			}
			if idx < len(in) {
				log.Printf("WARNING: received %d bytes from device but only %d were expected: %v", len(in), idx, in)
			}
			return stat, nil
		}
}

func Query() (func(url.Values, HardwareModel) ([]byte, error), func(HardwareModel, []byte) (any, error)) {
	return func(_ url.Values, _ HardwareModel) ([]byte, error) {
			return []byte{'Q'}, nil
		}, func(hw HardwareModel, in []byte) (any, error) {
			// parse the response data, returning the data structure represented or the number of additional bytes
			// still needed before we can have a successful read
			var err error
			var idx int

			// try parsing full device status response
			if len(in) < 15 {
				return DeviceStatus{}, fmt.Errorf("query response from hardware too short (%d)", len(in))
			}
			if in[0] != 'Q' || in[2] != '=' || in[8] != '$' {
				return DeviceStatus{}, fmt.Errorf("query response is invalid (%v...)", in[0:9])
			}

			stat := DeviceStatus{
				DeviceModelClass: in[1],
				DeviceAddress:    parseAddress(in[3]),
				GlobalAddress:    parseAddress(in[6]),
			}
			if stat.SpeedUSB, err = parseBaudRateCode(in[4]); err != nil {
				return stat, fmt.Errorf("query response usb baud rate code %c invalid (%v)", in[4], err)
			}
			if stat.Speed485, err = parseBaudRateCode(in[5]); err != nil {
				return stat, fmt.Errorf("query response rs-485 baud rate code %c invalid (%v)", in[5], err)
			}
			if stat.EEPROM, err = parseEEPROMType(in[7]); err != nil {
				return stat, fmt.Errorf("query response EEPROM type code %c invalid (%v)", in[7], err)
			}
			if stat.HardwareRevision, idx, err = extractString(in, 9, "V"); err != nil {
				return stat, fmt.Errorf("query response hardware version could not be parsed (%v)", err)
			}
			if stat.FirmwareRevision, idx, err = extractString(in, idx, "R"); err != nil {
				return stat, fmt.Errorf("query response firmware version could not be parsed (%v)", err)
			}
			if stat.Serial, idx, err = extractString(in, idx, "S"); err != nil {
				return stat, fmt.Errorf("query response serial number could not be parsed (%v)", err)
			}
			if stat.StatusLEDs, idx, err = parseStatusLEDs(in, idx); err != nil {
				return stat, fmt.Errorf("query response status LEDs could not be parsed (%v)", err)
			}

			if stat.DeviceModelClass == 'B' {
				if idx < len(in) {
					log.Printf("WARNING: read %d bytes of status from device but only %d was expected: %v", len(in), idx, in)
				}
				return stat, nil
			}
			var planeHexBytes string
			var planeBytes [64]byte
			if planeHexBytes, idx, err = extractString(in, idx, "M"); err != nil {
				return stat, fmt.Errorf("query response red bitmap plane could not be extracted (%v)", err)
			}
			if planeBytes, err = parseBitmapPlane(planeHexBytes); err != nil {
				return stat, fmt.Errorf("query response red bitmap plane could not be parsed (%v)", err)
			}
			stat.ImageBitmap = append(stat.ImageBitmap, planeBytes)
			if stat.DeviceModelClass == 'C' {
				if planeHexBytes, idx, err = extractString(in, idx, ""); err != nil {
					return stat, fmt.Errorf("query response green bitmap plane could not be extracted (%v)", err)
				}
				if planeBytes, err = parseBitmapPlane(planeHexBytes); err != nil {
					return stat, fmt.Errorf("query response green bitmap plane could not be parsed (%v)", err)
				}
				stat.ImageBitmap = append(stat.ImageBitmap, planeBytes)
				if planeHexBytes, idx, err = extractString(in, idx, ""); err != nil {
					return stat, fmt.Errorf("query response blue bitmap plane could not be extracted (%v)", err)
				}
				if planeBytes, err = parseBitmapPlane(planeHexBytes); err != nil {
					return stat, fmt.Errorf("query response blue bitmap plane could not be parsed (%v)", err)
				}
				stat.ImageBitmap = append(stat.ImageBitmap, planeBytes)
			}
			if planeHexBytes, idx, err = extractString(in, idx, ""); err != nil {
				return stat, fmt.Errorf("query response flash bitmap plane could not be extracted (%v)", err)
			}
			if planeBytes, err = parseBitmapPlane(planeHexBytes); err != nil {
				return stat, fmt.Errorf("query response flash bitmap plane could not be parsed (%v)", err)
			}
			stat.ImageBitmap = append(stat.ImageBitmap, planeBytes)
			if idx < len(in) {
				log.Printf("WARNING: read %d bytes of status from device but only %d was expected: %v", len(in), idx, in)
			}
			return stat, nil
		}
}

func parseAddress(b byte) byte {
	if b < '0' || b > 'o' {
		return 0xff
	}
	return b - '0'
}

func WrapInternalHandler(f func([]int, url.Values) error, config *ConfigData) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		targets, err := reqInit(r, config.GlobalAddress)
		if err != nil {
			log.Printf("invalid request: %v", err)
			io.WriteString(w, "invalid request\n")
		}
		if err := f(targets, r.Form); err != nil {
			log.Printf("requested internal command failed: %v", err)
			io.WriteString(w, fmt.Sprintf("error: %v\n", err))
		}
	}
}

func Post(_ []int, _ url.Values) error {
	return nil
}
func Unpost(_ []int, _ url.Values) error {
	return nil
}
func Update(_ []int, _ url.Values) error {
	return nil
}
