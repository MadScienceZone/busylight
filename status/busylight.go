package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"path/filepath"
	"strings"
)

func main() {
	var Fmute = flag.Bool("mute", false, "muted mic in meeting")
	var Fopen = flag.Bool("open", false, "open mic in meeting")
	var Fcal = flag.Bool("cal", false, "leave meeting; back to calendar status")
	var Fzzz = flag.Bool("zzz", false, "toggle active/inactive status")
	var Fkill = flag.Bool("kill", false, "terminate busylight service")
	var Freload = flag.Bool("reload", false, "reload calendar data")
	flag.Parse()

	thisUser, err := user.Current()
	if err != nil { panic(err) }

	pidbytes, err := ioutil.ReadFile(filepath.Join(thisUser.HomeDir, ".busylight/busylightd.pid"))
	if err != nil { panic(err) }

	pid, err := strconv.Atoi(strings.TrimSuffix(string(pidbytes), "\n"))
	if err != nil { panic(err) }

	process, err := os.FindProcess(pid)
	if err != nil { panic(err) }

	switch {
		case *Fmute:
			process.Signal(syscall.SIGUSR1)

		case *Fopen:
			process.Signal(syscall.SIGUSR2)

		case *Fcal:
			process.Signal(syscall.SIGHUP)

		case *Fzzz:
			process.Signal(syscall.SIGWINCH)

		case *Fkill:
			process.Signal(syscall.SIGINT)

		case *Freload:
			process.Signal(syscall.SIGINFO)
	}
}
