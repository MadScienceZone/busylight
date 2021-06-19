//
// vi:set ai sm nu ts=4 sw=4:
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
	var red1 = flag.Bool("red", false, "display red #1 light")
	var red2 = flag.Bool("red2", false, "display red #2 light")
	var reds = flag.Bool("reds", false, "display both red lights")
	var green = flag.Bool("green", false, "display green light")
	var blue = flag.Bool("blue", false, "display blue light")
	var yellow = flag.Bool("yellow", false, "display yellow light")
	var redred = flag.Bool("redred", false, "flash both reds alternately")
	var redblue = flag.Bool("redblue", false, "flash red and blue alternately")
	var off = flag.Bool("off", false, "turn off lights")
	var list = flag.Bool("list", false, "list port names")
	var low = flag.Bool("lowpri", false, "low-priority signal")
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
	case *red1:  send("R", port)
	case *red2:  send("2", port)
	case *reds:  send("!", port)
	case *green: send("G", port)
	case *blue:  send("B", port)
	case *yellow:send("Y", port)
	case *redred:send("#", port)
	case *redblue:send("%", port)
	case *off:    send("X", port)
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
