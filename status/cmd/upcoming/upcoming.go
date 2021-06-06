//
// vi:set ai sm nu ts=4 sw=4:
//
// Steve Willoughby <steve@madscience.zone>
// License: BSD 3-Clause open-source license
//
// Check for upcoming meetings in Google calendar.
// Originally this was part of my early experiments to
// learn how to query the Google API but may also be
// useful as a manual debugging aid.
//
// This tool is also useful for manually obtaining a Google
// API authentication token for the other tools to then
// continue using.
//

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

type calendarConfigData struct {
	Title              string
	IgnoreAllDayEvents bool
}

type configData struct {
	Calendars      map[string]calendarConfigData
	TokenFile      string
	CredentialFile string
}

func getConfigFromFile(filename string, data *configData) error {
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
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the authorization code:\n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
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

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	var config configData
	var thisUser *user.User

	thisUser, err := user.Current()
	if err != nil {
		log.Fatalf("Unable to determine current user: %v", err)
	}

	err = getConfigFromFile(filepath.Join(thisUser.HomeDir, ".busylight/config.json"), &config)
	if err != nil {
		log.Fatalf("Unable to initialize: %v", err)
	}

	b, err := ioutil.ReadFile(config.CredentialFile)
	if err != nil {
		log.Fatalf("Unable to read client secret file %v: %v", config.CredentialFile, err)
	}

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
	for calID := range config.Calendars {
		query.Items = append(query.Items, &calendar.FreeBusyRequestItem{Id: calID})
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
	for calID, calData := range freelist.Calendars {
		log.Printf("For calendar <%v>:", calID)
		for _, e := range calData.Errors {
			log.Printf("   ERROR %v", e)
		}
		for _, busy := range calData.Busy {
			st, err := time.Parse(time.RFC3339, busy.Start)
			et, err2 := time.Parse(time.RFC3339, busy.End)
			if err != nil || err2 != nil {
				log.Printf("   ERROR: Unable to understand these time values: %v - %v", busy.Start, busy.End)
			} else {
				log.Printf("   Busy %v - %v", st.Local().Format(time.UnixDate), et.Local().Format(time.UnixDate))
			}
		}
	}
	if errors > 0 {
		log.Fatalf("Errors encountered: %d", errors)
	}
}
