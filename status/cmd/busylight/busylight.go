//
// CLI tool to control long-running daemon busylightd
// and send direct light commands to the device.
//
// Steve Willoughby <steve@madscience.zone>
// License: BSD 3-Clause open-source license
//

package main

import (
	"flag"
	"fmt"
	"internal/busylight"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func fatal(format string, a ...interface{}) {
	fmt.Printf(format, a...)
	os.Exit(1)
}

func getDaemonProcess(config *busylight.ConfigData) *os.Process {
	pidbytes, err := ioutil.ReadFile(config.PidFile)
	if err != nil {
		return nil
	}

	pid, err := strconv.Atoi(strings.TrimSuffix(string(pidbytes), "\n"))
	if err != nil {
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	return process
}

func main() {
	var config busylight.ConfigData
	var devState busylight.DevState

	devState.Logger = log.New(os.Stdout, "busylight: ", log.LstdFlags)
	var Fmute = flag.Bool("mute", false, "muted mic in meeting")
	var Fopen = flag.Bool("open", false, "open mic in meeting")
	var Fcal = flag.Bool("cal", false, "leave meeting; back to calendar status")
	var Fzzz = flag.Bool("zzz", false, "put daemon to sleep")
	var Fwake = flag.Bool("wake", false, "wake daemon from sleep")
	var Fkill = flag.Bool("kill", false, "terminate busylight service")
	var Freload = flag.Bool("reload", false, "reload calendar data")
	var Fstatus = flag.String("status", "", "set custom status by name")
	var Fraw = flag.String("raw", "", "send raw command to device")
	var Flist = flag.Bool("list", false, "list defined status codes")
	var Fquery = flag.Bool("query", false, "report current status of lights")
	var daemon *os.Process
	flag.Parse()

	//
	// Find the user and from there the configuration file
	//
	thisUser, err := user.Current()
	if err != nil {
		fatal("Who are you? (%v)\n", err)
	}

	if err = busylight.GetConfigFromFile(
		filepath.Join(thisUser.HomeDir, ".busylight/config.json"),
		&config); err != nil {
		fatal("Can't initialize: %v\n", err)
	}

	if *Flist {
		fmt.Println("Defined status codes usable with the --status option:")
		fmt.Println("CODE------  LIGHT-EFFECT")
		for code, def := range config.StatusLights {
			fmt.Printf("%-10s  %s\n", code, def)
		}
		return
	}

	if *Fmute || *Fopen || *Fcal || *Fzzz || *Fwake || *Fkill || *Freload || *Fquery {
		daemon = getDaemonProcess(&config)
	}

	if *Fwake {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon, so I can't signal it.\n")
		} else {
			daemon.Signal(syscall.SIGVTALRM)
		}
	}

	if *Fmute {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon. Sending direct \"mute\" status\n")
			if err := busylight.LightSignal(&config, &devState, "mute", 0); err != nil {
				fmt.Printf("Warning: %v\n", err)
			}
		} else {
			daemon.Signal(syscall.SIGUSR1)
		}
	}

	if *Fopen {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon. Sending direct \"open\" status\n")
			if err := busylight.LightSignal(&config, &devState, "open", 0); err != nil {
				fmt.Printf("Warning: %v\n", err)
			}
		} else {
			daemon.Signal(syscall.SIGUSR2)
		}
	}

	if *Fcal {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon. I don't know what status to send.\n")
		} else {
			daemon.Signal(syscall.SIGHUP)
		}
	}

	if *Fkill {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon, so I can't signal it.\n")
		} else {
			daemon.Signal(syscall.SIGINT)
		}
	}

	if *Freload {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon, so I can't signal it.\n")
		} else {
			daemon.Signal(syscall.SIGPWR)
		}
	}

	if *Fstatus != "" {
		if err := busylight.LightSignal(&config, &devState, *Fstatus, 0); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	if *Fraw != "" {
		if err := busylight.RawLightSignal(&config, &devState, *Fraw, 0); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	if *Fzzz {
		if daemon == nil {
			fmt.Printf("Warning: unable to find daemon, so I can't signal it.\n")
		} else {
			daemon.Signal(syscall.SIGWINCH)
		}
	}

	if *Fquery {
		if state, err := busylight.QueryStatus(&config, &devState, 0); err == nil {
			if daemon == nil {
				fmt.Println("Daemon NOT running.")
			} else {
				fmt.Printf("Daemon running, pid=%v.\n", daemon.Pid)
			}
			fmt.Println("Current hardware status:")
			fmt.Printf("  Raw response data: %v\n", state.RawResponse[:state.ResponseLength])
			fmt.Print("  Individual LEDs:   ")
			for i, on := range state.IsLightOn {
				if on {
					if i < len(config.Colors) {
						fmt.Printf("%c", config.Colors[i])
					} else {
						fmt.Print("X")
					}
				} else {
					fmt.Print("-")
				}
			}
			fmt.Print("\n")
			showSequence("Flasher", config, state.Flasher)
			showSequence("Strober", config, state.Strober)
		} else {
			fmt.Printf("Warning: %v\n", err)
		}
	}
}

func showSequence(name string, config busylight.ConfigData, seq busylight.LightSequence) {
	if len(seq.Sequence) > 0 {
		fmt.Printf("  %s: ", name)
		for _, led := range seq.Sequence {
			if int(led) < len(config.Colors) {
				fmt.Printf("%c", config.Colors[int(led)])
			} else {
				fmt.Printf("%d", led)
			}
		}
		if seq.IsOn {
			fmt.Println(" (on)")
		} else {
			fmt.Println(" (off)")
		}
		fmt.Printf("  %*s  ", len(name), "")
		for i, _ := range seq.Sequence {
			if i == seq.SequenceIndex {
				fmt.Println("^")
				return
			}
			fmt.Print(" ")
		}
		fmt.Println("")
	} else {
		fmt.Printf("  %s disabled\n", name)
	}
}
