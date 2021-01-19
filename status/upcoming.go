// Check for upcoming meetings in Google calendar
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
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"

	_ "github.com/go-sql-driver/mysql"
)

type ConfigData struct {
	Calendars      []string
	TokenFile      string
	CredentialFile string
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
	var config ConfigData
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
		++errors
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


