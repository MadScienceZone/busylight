package main

import (
	"flag"
	"fmt"
	"go.bug.st/serial"
	"log"
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
	var calendar = flag.Bool("calendar", false, "set to calendar busy/free state")
	var list = flag.Bool("list", false, "list port names")
	flag.Parse()

	if *list {
		names, err := serial.GetPortsList()
		if err != nil { panic(err) }
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}

	port, err := serial.Open("/dev/tty.usbmodem2101", &serial.Mode{
		BaudRate: 9600,
	})
	if err != nil {
		log.Fatalf("Can't open serial device: %v", err)
	}
	defer port.Close()

	switch {
		case *red1:
			_, err = port.Write([]byte("R"))
			break;
		case *red2:
			_, err = port.Write([]byte("2"))
			break;
		case *reds:
			_, err = port.Write([]byte("!"))
			break;
		case *green:
			_, err = port.Write([]byte("G"))
			break;
		case *blue:
			_, err = port.Write([]byte("B"))
			break;
		case *yellow:
			_, err = port.Write([]byte("Y"))
			break;
		case *redred:
			_, err = port.Write([]byte("#"))
			break;
		case *redblue:
			_, err = port.Write([]byte("%"))
			break;
		case *off:
			_, err = port.Write([]byte("X"))
			break;
		case *calendar:
			log.Fatalf("--calendar not implemented")
			break;
	}
	if err != nil { panic(err) }
}
