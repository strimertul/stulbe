package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"runtime"

	"github.com/dgraph-io/badger/v3"
	"github.com/gorilla/mux"
	"github.com/nicklaw5/helix"

	"github.com/mattn/go-colorable"
	"github.com/sirupsen/logrus"
)

//go:embed frontend/dist/*
var frontend embed.FS

var db *badger.DB

var client *helix.Client

var log = logrus.New()

func wrapLogger(module string) logrus.FieldLogger {
	return log.WithField("module", module)
}

func parseLogLevel(level string) logrus.Level {
	switch level {
	case "error":
		return logrus.ErrorLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "info", "notice":
		return logrus.InfoLevel
	case "debug":
		return logrus.DebugLevel
	case "trace":
		return logrus.TraceLevel
	default:
		return logrus.InfoLevel
	}
}

func main() {
	bind := flag.String("bind", ":9999", "Bind addr in format address:port")
	dbdir := flag.String("dbfile", "data", "Filename for database")
	loglevel := flag.String("loglevel", "info", "Logging level (debug, info, warn, error)")
	flag.Parse()

	log.SetLevel(parseLogLevel(*loglevel))

	// Ok this is dumb but listen, I like colors.
	if runtime.GOOS == "windows" {
		log.SetFormatter(&logrus.TextFormatter{ForceColors: true})
		log.SetOutput(colorable.NewColorableStdout())
	}

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
	options := badger.DefaultOptions(*dbdir)
	options.Logger = wrapLogger("db")
	db, err = badger.Open(options)
	failOnError(err, "Could not open DB")
	defer db.Close()

	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api").Subrouter()
	bindApiRoutes(apiRouter)
	fedir, _ := fs.Sub(frontend, "frontend/dist")
	router.Handle("/", http.FileServer(http.FS(fedir)))
	http.Handle("/", router)
	httpLogger := wrapLogger("http")
	httpLogger.WithField("bind", *bind).Info("starting web server")
	fatalError(http.ListenAndServe(*bind, nil), "HTTP server died unexepectedly")
}

func failOnError(err error, text string) {
	if err != nil {
		fatalError(err, text)
	}
}

func fatalError(err error, text string) {
	if err != nil {
		log.Fatalf("FATAL ERROR OCCURRED: %s\n\n%s", text, err.Error())
	}
}
