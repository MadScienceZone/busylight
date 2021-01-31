//
// vi:set ai sm nu ts=4 sw=4:
//
// CLI tool to control long-running daemon busylightd
// by sending it these signals:
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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"path/filepath"
	"strings"
)

func fatal(format string, a ...interface{}) {
	fmt.Printf(format, a...)
	os.Exit(1)
}

func main() {
	var Fmute = flag.Bool("mute", false, "muted mic in meeting")
	var Fopen = flag.Bool("open", false, "open mic in meeting")
	var Fcal = flag.Bool("cal", false, "leave meeting; back to calendar status")
	var Fzzz = flag.Bool("zzz", false, "toggle active/inactive status")
	var Fkill = flag.Bool("kill", false, "terminate busylight service")
	var Freload = flag.Bool("reload", false, "reload calendar data")
	var Furgent = flag.Bool("urgent", false, "toggle urgent condition indicator")
	flag.Parse()

	thisUser, err := user.Current()
	if err != nil { fatal("Who are you? (%v)\n", err) }

	pidbytes, err := ioutil.ReadFile(filepath.Join(thisUser.HomeDir, ".busylight/busylightd.pid"))
	if err != nil { fatal("Can't read PID file: %v\n", err) }

	pid, err := strconv.Atoi(strings.TrimSuffix(string(pidbytes), "\n"))
	if err != nil { fatal("Can't understand PID value: %v\n", err) }

	process, err := os.FindProcess(pid)
	if err != nil { fatal("Can't find daemon process: %v\n", err) }

	if *Furgent { process.Signal(syscall.SIGVTALRM)   }
	if *Fmute   { process.Signal(syscall.SIGUSR1)  }
	if *Fopen   { process.Signal(syscall.SIGUSR2)  }
	if *Fcal    { process.Signal(syscall.SIGHUP)   }
	if *Fzzz    { process.Signal(syscall.SIGWINCH) }
	if *Fkill   { process.Signal(syscall.SIGINT)   }
	if *Freload { process.Signal(syscall.SIGINFO)  }
}
