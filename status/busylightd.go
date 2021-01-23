//
//
// Long-running daemon to control the busylight.
// Automatically polls Google calendar busy/free times
// and can be controlled via signals from a Zoom meeting
// monitoring script:
//
//    USR1 - in zoom, muted
//    USR2 - in zoom, unmuted
//    HUP  - out of zoom
//    INFO - force refresh from calendar now
//    WINCH- toggle idle/working state
//
// Steve Willoughby <steve@alchemy.com>
//

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
	"os/user"
	"path/filepath"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
//	"golang.org/x/oauth2/google"
//	"google.golang.org/api/calendar/v3"
	"go.bug.st/serial"
	"os/signal"
	"syscall"
)

type ConfigData struct {
	Calendars      []string
	TokenFile      string
	CredentialFile string
	LogFile        string
	PidFile        string
	Device         string
	BaudRate       int
	googleConfig   []byte
	logger        *log.Logger
}

func getConfigFromFile(filename string, data *ConfigData) error {
	cdata, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("Unable to read from %s: %v", filename, err)
	}

	err = json.Unmarshal(cdata, &data)
	if err != nil {
		return fmt.Errorf("Unable to understand %s configuration: %v", filename, err)
	}
	return nil
}

func getClient(config *oauth2.Config, tokFile string) *http.Client {
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	log.Printf("Unable to use cached credentials. Use stand-alone tool to manually obtain Google calendar authorization.")
	return nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// This should be refactored out if we're not getting new authorizations now
func saveToken(path string, token *oauth2.Token) {
	log.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Printf("Unable to cache oauth token: %v", err)
	} else {
		defer f.Close()
		json.NewEncoder(f).Encode(token)
	}
}

func setup(config *ConfigData) error {
	var thisUser *user.User

	thisUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("Unable to determine current user: %v", err)
	}

	err = getConfigFromFile(filepath.Join(thisUser.HomeDir, ".busylight/config.json"), config)
	if err != nil {
		return fmt.Errorf("Unable to initialize: %v", err)
	}

	f, err := os.OpenFile(config.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("Unable to open logfile: %v", err)
	}
	config.logger = log.New(f, "busylightd: ", log.LstdFlags)

	myPID := os.Getpid()
	config.logger.Printf("busylightd started, PID=%v", myPID)

	pidf, err := os.OpenFile(config.PidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		config.logger.Printf("ERROR creating PID file (is another busylightd running?): %v", err)
		return err
	}
	pidf.WriteString(fmt.Sprintf("%d\n", myPID))
	pidf.Close()

	config.googleConfig, err = ioutil.ReadFile(config.CredentialFile)
	if err != nil {
		config.logger.Printf("Unable to read client secret file %v: %v", config.CredentialFile, err)
		return fmt.Errorf("Unable to read client secret file %v: %v", config.CredentialFile, err)
	}

	return nil
}

func main() {
	var config ConfigData

	if err := setup(&config); err != nil {
		log.Fatalf("Unable to start daemon: %v", err)
	}
	defer func(){
		err := os.Remove(config.PidFile)
		if err != nil {
			config.logger.Printf("Error removing PID file: %v", err)
		}
		config.logger.Printf("busylightd shutting down")
	}()

	//
	// Open the hardware port
	//
	port, err := serial.Open(config.Device, &serial.Mode{
		BaudRate: config.BaudRate,
	})
	if err != nil {
		config.logger.Fatalf("Can't open serial device: %v", err)
	}
	defer port.Close()

	//
	// Listen for incoming signals from outside
	//
	req := make(chan os.Signal, 5)
	signal.Notify(req, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGWINCH, syscall.SIGINFO, syscall.SIGINT)

	//
	// Signal that we're online and ready
	//
	port.Write([]byte("B"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
	time.Sleep(50 * time.Millisecond)
	port.Write([]byte("B"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))

	//
	// Main event loop:
	// 	On incoming signals, indicate light status as requested by signaller
	//  Otherwise, update Google calendar status hourly while active
	//	Update lights based on busy/free status when transition times arrive unless in Zoom
	//
eventLoop:
	for {
		select {
			case externalSignal := <-req:
				switch externalSignal {
					case syscall.SIGHUP:
						config.logger.Printf("ZOOM: Call ended")
						// XXX go to state per calendar
						port.Write([]byte("G"))

					case syscall.SIGUSR1:
						config.logger.Printf("ZOOM: Muted")
						port.Write([]byte("R"))

					case syscall.SIGUSR2:
						config.logger.Printf("ZOOM: Unmuted")
						port.Write([]byte("#"))

					case syscall.SIGWINCH:
						config.logger.Printf("Toggle active state")
						// XXX make a proper toggle
						port.Write([]byte("X"))

					case syscall.SIGINFO:
						config.logger.Printf("Reloading calendar status by request")
						// XXX

					case syscall.SIGINT:
						config.logger.Printf("Received interrupt signal")
						break eventLoop

					default:
						config.logger.Printf("Received unexpeced signal %v (ignored)", externalSignal)
				}
		}
	}

	//
	// Signal shutdown
	//
	port.Write([]byte("2"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
	time.Sleep(50 * time.Millisecond)
	port.Write([]byte("2"))
	time.Sleep(100 * time.Millisecond)
	port.Write([]byte("X"))
}

/*
	googleConfig, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(googleConfig, config.TokenFile)

	srv, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

	// time parameters for query
	now := time.Now().Format(time.RFC3339)
	eod := time.Now().Add(time.Hour * 8).Format(time.RFC3339)

	var query calendar.FreeBusyRequest
	query.TimeMax = eod
	query.TimeMin = now
	for _, calId := range config.Calendars {
		query.Items = append(query.Items, &calendar.FreeBusyRequestItem{Id: calId})
	}

	//
	// for now, just report busy periods
	//
	var errors int
	freelist, err := srv.Freebusy.Query(&query).Do()
	if err != nil {
		log.Fatalf("Error reading calendar: %v", err)
		errors++
	}
	for calId, calData := range freelist.Calendars {
		log.Printf("<%v>", calId)
		for _, e := range calData.Errors {
			log.Printf("   ERROR %v", e)
		}
		for _, busy := range calData.Busy {
			log.Printf("   %v - %v", busy.Start, busy.End)
		}
	}
	if errors > 0 {
		log.Fatalf("Errors encountered: %d", errors)
	}
}











import (
	"flag"
	"fmt"
	"log"
)

func main() {

	if *list {
		names, err := serial.GetPortsList()
		if err != nil { panic(err) }
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}


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
*/
