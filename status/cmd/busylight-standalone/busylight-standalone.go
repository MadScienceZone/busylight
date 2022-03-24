//
// This is just a standalone tool I wrote while
// experimenting with controlling the hardware.
// It may be of use while building your own light
// hardware but isn't intended to be part of the
// "production" code.
//
// Steve Willoughby <steve@madscience.zone>
// License: BSD 3-Clause open-source license
//
package main

import (
	"flag"
	"fmt"
	"log"

	"go.bug.st/serial"
)

func main() {
	var steady = flag.String("on", "", "turn on light (0-6)")
	var flash = flag.String("flash", "", "flash one or more lights (0-6)")
	var strobe = flag.String("strobe", "", "strobe one or more lights (0-6 or \"off\")")

	var red1 = flag.Bool("red", false, "(deprecated, legacy) display red #1 light")
	var red2 = flag.Bool("red2", false, "(deprecated, legacy) display red #2 light")
	var reds = flag.Bool("reds", false, "(deprecated, legacy) display red #1 light")
	var green = flag.Bool("green", false, "(deprecated, legacy) display green light")
	var blue = flag.Bool("blue", false, "(deprecated, legacy) display blue light")
	var yellow = flag.Bool("yellow", false, "(deprecated, legacy) display yellow light")
	var redred = flag.Bool("redred", false, "(deprecated, legacy) flash both reds alternately")
	var redblue = flag.Bool("redblue", false, "(deprecated, legacy) flash red and blue alternately")
	var low = flag.Bool("lowpri", false, "(deprecated, legacy) low-priority signal")

	var off = flag.Bool("off", false, "turn off lights")
	var list = flag.Bool("list", false, "list port names")
	var device = flag.String("device", "", "serial device of the light")
	flag.Parse()

	if *list {
		names, err := serial.GetPortsList()
		if err != nil {
			panic(err)
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}

	if device == nil || *device == "" {
		log.Fatalf("--device option is required; use --list to see a list of possible devices to use.")
	}

	port, err := serial.Open(*device, &serial.Mode{
		BaudRate: 9600,
	})
	if err != nil {
		log.Fatalf("Can't open serial device: %v", err)
	}
	defer port.Close()

	switch {
	/* new, more general, commands */
	case *steady != "":
		if len(*steady) != 1 {
			log.Fatal("--on requres a single light ID (e.g. --on=2)")
		}
		send("S"+*steady, port)

	case *flash != "":
		send("F"+*flash+"$", port)

	case *strobe != "":
		if *strobe == "off" {
			send("*$", port)
		} else {
			send("*"+*strobe+"$", port)
		}

	case *off:
		send("X", port)

	/* support for old legacy commands */
	case *red1:
		send("R", port)
	case *red2:
		send("2", port)
	case *reds:
		send("!", port)
	case *green:
		send("G", port)
	case *blue:
		send("B", port)
	case *yellow:
		send("Y", port)
	case *redred:
		send("#", port)
	case *redblue:
		send("%", port)
	}
	if *low {
		send("@", port)
	}
}

func send(code string, port serial.Port) {
	fmt.Printf("Sending %s\n", code)
	_, err := port.Write([]byte(code))
	if err != nil {
		log.Fatalf("Error writing to serial port: %v", err)
	}
}
