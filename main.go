package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/dgraph-io/badger/v3"
	"github.com/gorilla/mux"
	"github.com/nicklaw5/helix"
)

//go:embed frontend/dist/*
var frontend embed.FS

var db *badger.DB

var client *helix.Client

func main() {
	bind := flag.String("bind", ":9999", "Bind addr in format address:port")
	dbdir := flag.String("dbfile", "data", "Filename for database")
	flag.Parse()

	// Get Twitch credentials from env
	twitchClientID := os.Getenv("TWITCH_CLIENT_ID")
	twitchClientSecret := os.Getenv("TWITCH_CLIENT_SECRET")

	if twitchClientID == "" || twitchClientSecret == "" {
		fatalError(fmt.Errorf("TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET env vars must be set to a Twitch application credentials"), "Missing configuration")
	}

	// Create Twitch client
	var err error
	client, err = helix.NewClient(&helix.Options{
		ClientID:     twitchClientID,
		ClientSecret: twitchClientSecret,
	})
	if err != nil {
		fatalError(err, "Failed to initialize Twitch helix client")
	}

	resp, err := client.RequestAppAccessToken([]string{"user:read:email"})
	if err != nil {
		fatalError(err, "Failed to get Twitch helix app token")
	}

	// Set the access token on the client
	client.SetAppAccessToken(resp.Data.AccessToken)

	// Open DB
	db, err = badger.Open(badger.DefaultOptions(*dbdir))
	if err != nil {
		fatalError(err, "Could not open DB")
	}
	defer db.Close()

	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api").Subrouter()
	bindApiRoutes(apiRouter)
	fedir, _ := fs.Sub(frontend, "frontend/dist")
	router.Handle("/", http.FileServer(http.FS(fedir)))
	http.Handle("/", router)
	log.Printf("starting web server at %s", *bind)
	fatalError(http.ListenAndServe(*bind, nil), "HTTP server died unexepectedly")
}

func fatalError(err error, text string) {
	if err != nil {
		log.Fatalf("FATAL ERROR OCCURRED: %s\n\n%s", text, err.Error())
	}
}
