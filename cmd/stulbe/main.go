package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger/v3"
	"github.com/mattn/go-colorable"
	"github.com/nicklaw5/helix"
	"github.com/sirupsen/logrus"

	"github.com/strimertul/stulbe"
	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"
)

var log = logrus.New()

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
	bootstrap := flag.String("bootstrap", "", "Create admin user with given credentials (user:token)")
	regenerateSecret := flag.Bool("regen-secret", false, "Force secret key generation, this will invalidate all previous session!")
	flag.Parse()

	log.SetLevel(parseLogLevel(*loglevel))

	// Ok this is dumb but listen, I like colors.
	if runtime.GOOS == "windows" {
		log.SetFormatter(&logrus.TextFormatter{ForceColors: true})
		log.SetOutput(colorable.NewColorableStdout())
	}

	// Open DB
	db, err := database.Open(badger.DefaultOptions(*dbdir), wrapLogger("db"))
	failOnError(err, "Could not open DB")
	defer db.Close()

	authStore, err := auth.Init(db, auth.Options{
		Logger:              wrapLogger("auth"),
		ForgeGenerateSecret: *regenerateSecret,
	})
	failOnError(err, "Could not initialize auth store")

	if *bootstrap != "" {
		parts := strings.SplitN(*bootstrap, ":", 2)
		if len(parts) < 2 || len(parts[0]) < 1 || len(parts[1]) < 1 {
			log.Fatalf("-bootstrap argument requires credentials in format username:token")
		}

		// Add administrator
		failOnError(authStore.AddUser(parts[0], parts[1], auth.ULAdmin), "Error adding admin user")

		log.WithField("user", parts[0]).Infof("Created admin user")
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

func wrapLogger(module string) logrus.FieldLogger {
	return log.WithField("module", module)
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
