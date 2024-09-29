//
// Server for busylight and readerboard devices
//
// This opens a simple web service API endpoint which clients can use to send updates
// to controlled busylight indicator and readerboard devices.  The busylight hardware
// and firmware are described in the same repository as this source code. The readerboard
// hardware and firmware appear in github.com/MadScienceZone/readerboard.
//
package main

import (
	"busylight/readerboard"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func configureServer() (*readerboard.ConfigData, error) {
	var configFile = flag.String("conf", "", "Load configuration information from the named file")
	var configData readerboard.ConfigData
	flag.Parse()

	if *configFile == "" {
		return nil, fmt.Errorf("-conf option is required")
	}

	if err := readerboard.GetConfigFromFile(*configFile, &configData); err != nil {
		return nil, err
	}

	if configData.LogFile != "" {
		if configData.LogFile == "-" {
			log.SetOutput(os.Stdout)
		} else {
			f, err := os.OpenFile(configData.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				return nil, err
			}
			log.SetOutput(f)
		}
	}

	log.Print("Server starting up. Device configuration follows.")
	log.Printf("global address=%v; logfile=%v; pidfile=%v", configData.GlobalAddress, configData.LogFile, configData.PidFile)

	myPID := os.Getpid()
	if configData.PidFile == "" {
		log.Printf("PID=%v (no PID file configured)", myPID)
	} else {
		pidf, err := os.OpenFile(configData.PidFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			log.Printf("Error creating PID file (is another instance already running?): %v", err)
			return nil, err
		}
		pidf.WriteString(fmt.Sprintf("%d\n", myPID))
		pidf.Close()
		log.Printf("PID=%v (written to %s)", myPID, configData.PidFile)
	}

	return &configData, nil
}

func main() {
	configData, err := configureServer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if configData.PidFile != "" {
			log.Printf("Removing %s", configData.PidFile)
			err := os.Remove(configData.PidFile)
			if err != nil {
				log.Printf("Error removing PID file %s: %v", configData.PidFile, err)
			}
		}
		log.Print("Server shutting down.")
	}()

	if err := readerboard.AttachToAllNetworks(configData); err != nil {
		log.Printf("Unable to attach to all networks: %v", err)
		return
	}
	if err := readerboard.ProbeDevices(configData); err != nil {
		log.Printf("Error probing devices: %v", err)
		return
	}

	serverDone := &sync.WaitGroup{}
	serverDone.Add(1)
	server := &http.Server{Addr: configData.Endpoint}

	http.HandleFunc("/readerboard/v1/alloff", readerboard.WrapHandler(readerboard.AllLightsOff, configData, true))
	http.HandleFunc("/readerboard/v1/bitmap", readerboard.WrapHandler(readerboard.Bitmap, configData, true))
	http.HandleFunc("/readerboard/v1/clear", readerboard.WrapHandler(readerboard.Clear, configData, true))
	http.HandleFunc("/readerboard/v1/color", readerboard.WrapHandler(readerboard.Color, configData, true))
	http.HandleFunc("/readerboard/v1/flash", readerboard.WrapHandler(readerboard.Flash, configData, true))
	http.HandleFunc("/readerboard/v1/font", readerboard.WrapHandler(readerboard.Font, configData, true))
	http.HandleFunc("/readerboard/v1/graph", readerboard.WrapHandler(readerboard.Graph, configData, true))
	http.HandleFunc("/readerboard/v1/light", readerboard.WrapHandler(readerboard.Light, configData, true))
	http.HandleFunc("/readerboard/v1/move", readerboard.WrapHandler(readerboard.Move, configData, true))
	http.HandleFunc("/readerboard/v1/off", readerboard.WrapHandler(readerboard.Off, configData, true))
	http.HandleFunc("/readerboard/v1/scroll", readerboard.WrapHandler(readerboard.Scroll, configData, true))
	http.HandleFunc("/readerboard/v1/strobe", readerboard.WrapHandler(readerboard.Strobe, configData, true))
	http.HandleFunc("/readerboard/v1/test", readerboard.WrapHandler(readerboard.Test, configData, true))
	http.HandleFunc("/readerboard/v1/text", readerboard.WrapHandler(readerboard.Text, configData, true))
	http.HandleFunc("/readerboard/v1/configure-device", readerboard.WrapHandler(readerboard.ConfigureDevice, configData, false))

	http.HandleFunc("/readerboard/v1/query", readerboard.WrapReplyHandler(readerboard.Query, configData))
	http.HandleFunc("/readerboard/v1/busy", readerboard.WrapReplyHandler(readerboard.QueryStatus, configData))

	http.HandleFunc("/readerboard/v1/post", readerboard.WrapInternalHandler(readerboard.Post, configData))
	http.HandleFunc("/readerboard/v1/postlist", readerboard.WrapInternalHandler(readerboard.PostList, configData))
	http.HandleFunc("/readerboard/v1/unpost", readerboard.WrapInternalHandler(readerboard.Unpost, configData))
	http.HandleFunc("/readerboard/v1/update", readerboard.WrapInternalHandler(readerboard.Update, configData))

	go func() {
		defer serverDone.Done()
		log.Printf("Starting to serve HTTP on %s", configData.Endpoint)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Unexpected HTTP server error: %v", err)
		}
	}()

	req := make(chan os.Signal)
	signal.Notify(req, syscall.SIGHUP, syscall.SIGINT)

eventloop:
	for {
		select {
		case externalSignal := <-req:
			switch externalSignal {
			case syscall.SIGHUP, syscall.SIGINT:
				log.Printf("%v received", externalSignal)
				if err := server.Shutdown(context.TODO()); err != nil {
					log.Printf("Error trying to shut down HTTP server: %v", err)
				}
				serverDone.Wait()
				log.Printf("HTTP Server shut down; exiting")
				break eventloop
			}
		}
	}
}
