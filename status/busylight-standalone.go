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
		case *red2:
			_, err = port.Write([]byte("2"))
		case *reds:
			_, err = port.Write([]byte("!"))
		case *green:
			_, err = port.Write([]byte("G"))
		case *blue:
			_, err = port.Write([]byte("B"))
		case *yellow:
			_, err = port.Write([]byte("Y"))
		case *redred:
			_, err = port.Write([]byte("#"))
		case *redblue:
			_, err = port.Write([]byte("%"))
		case *off:
			_, err = port.Write([]byte("X"))
	}
	if err != nil { panic(err) }
}