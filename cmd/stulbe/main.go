package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v3"
	"github.com/nicklaw5/helix/v2"
	"go.uber.org/zap"

	"github.com/strimertul/stulbe"
	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"
)

var log *zap.Logger

func main() {
	bind := flag.String("bind", ":9999", "Bind addr in format address:port")
	dbdir := flag.String("dbfile", "data", "Filename for database")
	debug := flag.Bool("debug", false, "Enable debug logging")
	bootstrap := flag.String("bootstrap", "", "Create admin user with given credentials (user:token)")
	regenerateSecret := flag.Bool("regen-secret", false, "Force secret key generation, this will invalidate all previous session!")
	flag.Parse()

	if *debug {
		var err error
		log, err = zap.NewDevelopment()
		failOnError(err, "Failed to create logger")
	} else {
		var err error
		log, err = zap.NewProduction()
		failOnError(err, "Failed to create logger")
	}

	// Open DB
	db, err := database.Open(badger.DefaultOptions(*dbdir).WithSyncWrites(true), log.With(zap.String("module", "db")))
	failOnError(err, "Could not open DB")
	defer db.Close()

	authStore, err := auth.Init(db, auth.Options{
		Logger:              log.With(zap.String("module", "auth")),
		ForgeGenerateSecret: *regenerateSecret,
	})
	failOnError(err, "Could not initialize auth store")

	if *bootstrap != "" {
		parts := strings.SplitN(*bootstrap, ":", 2)
		if len(parts) < 2 || len(parts[0]) < 1 || len(parts[1]) < 1 {
			log.Fatal("-bootstrap argument requires credentials in format username:token")
		}

		// Add administrator
		failOnError(authStore.AddUser(parts[0], parts[1], auth.ULAdmin), "Error adding admin user")

		log.Info("Created admin user", zap.String("user", parts[0]))
	} else {
		if authStore.CountUsers() < 1 {
			log.Warn("No config found, you should start stulbe with -boostrap to set-up an administrator!")
		}
	}

	// Get Twitch credentials from env
	twitchClientID := os.Getenv("TWITCH_CLIENT_ID")
	twitchClientSecret := os.Getenv("TWITCH_CLIENT_SECRET")
	if twitchClientID == "" || twitchClientSecret == "" {
		fatalError(fmt.Errorf("TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET env vars must be set to a Twitch application credentials"), "Missing configuration")
	}

	webhookSecret := os.Getenv("TWITCH_WEBHOOK_SECRET")
	if webhookSecret == "" {
		fatalError(fmt.Errorf("TWITCH_WEBHOOK_SECRET env var must be set to a random string between 10 and 100 characters"), "Missing configuration")
	}

	redirectURL := os.Getenv("REDIRECT_URI")
	if redirectURL == "" {
		fatalError(fmt.Errorf("REDIRECT_URI env var must be set to a valid URL on which the stulbe host is reacheable (eg. https://stulbe.your.tld/callback"), "Missing configuration")
	}

	webhookURL := os.Getenv("WEBHOOK_URI")
	if webhookURL == "" {
		fatalError(fmt.Errorf("WEBHOOK_URI env var must be set to a valid URL on which the stulbe host is reacheable (eg. https://stulbe.your.tld/webhook"), "Missing configuration")
	}

	// Create Twitch client
	backend, err := stulbe.NewBackend(db, authStore, stulbe.BackendConfig{
		WebhookSecret: webhookSecret,
		WebhookURL:    webhookURL,
		RedirectURL:   redirectURL,
		Twitch: &helix.Options{
			ClientID:     twitchClientID,
			ClientSecret: twitchClientSecret,
			RedirectURI:  redirectURL,
		},
	}, log)
	failOnError(err, "Could not create backend")

	fatalError(backend.RunHTTPServer(*bind), "HTTP server died unexepectedly")
}

func failOnError(err error, text string) {
	if err != nil {
		fatalError(err, text)
	}
}

func fatalError(err error, text string) {
	if err != nil {
		fmt.Printf("FATAL ERROR OCCURRED: %s\n%s\n", text, err.Error())
		os.Exit(1)
	}
}
