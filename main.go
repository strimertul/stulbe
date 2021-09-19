package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger/v3"
	"github.com/gorilla/mux"
	lru "github.com/hashicorp/golang-lru"
	"github.com/mattn/go-colorable"
	"github.com/nicklaw5/helix"
	"github.com/sirupsen/logrus"

	kv "github.com/strimertul/kilovolt/v4"

	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"
)

var db *database.DB
var authStore *auth.Storage
var log = logrus.New()
var httpLogger logrus.FieldLogger
var options *helix.Options
var appClient *helix.Client

var usercache *lru.Cache
var channelcache *lru.Cache
var webhookURI string
var webhookSecret string

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
	var err error
	db, err = database.Open(badger.DefaultOptions(*dbdir), wrapLogger("db"))
	failOnError(err, "Could not open DB")
	defer db.Close()

	// Initialize caches to avoid spamming Twitch's APIs
	usercache, err = lru.New(128)
	failOnError(err, "Could not create LRU cache for users")
	channelcache, err = lru.New(128)
	failOnError(err, "Could not create LRU cache for channels")

	// Initialize KV (required)
	hub, err := kv.NewHub(db.Client(), wrapLogger("kv"))
	failOnError(err, "Could not initialize KV hub")
	go hub.Run()

	authStore, err = auth.Init(db, auth.Options{
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

	webhookSecret = os.Getenv("TWITCH_WEBHOOK_SECRET")
	if webhookSecret == "" {
		fatalError(fmt.Errorf("TWITCH_WEBHOOK_SECRET env var must be set to a random string between 10 and 100 characters"), "Missing configuration")
	}

	redirectURI := os.Getenv("REDIRECT_URI")
	if redirectURI == "" {
		fatalError(fmt.Errorf("REDIRECT_URI env var must be set to a valid URL on which the stulbe host is reacheable (eg. https://stulbe.your.tld/callback"), "Missing configuration")
	}
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		fatalError(fmt.Errorf("REDIRECT_URI parsing error: %s)", err), "Invalid configuration")
	}

	webhookURI = os.Getenv("WEBHOOK_URI")
	if redirectURI == "" {
		fatalError(fmt.Errorf("WEBHOOK_URI env var must be set to a valid URL on which the stulbe host is reacheable (eg. https://stulbe.your.tld/webhook"), "Missing configuration")
	}
	webhookURL, err := url.Parse(webhookURI)
	if err != nil {
		fatalError(fmt.Errorf("WEBHOOK_URI parsing error: %s)", err), "Invalid configuration")
	}

	// Create Twitch client
	options = &helix.Options{
		ClientID:     twitchClientID,
		ClientSecret: twitchClientSecret,
		RedirectURI:  redirectURI,
	}
	appClient, err = helix.NewClient(options)
	if err != nil {
		fatalError(err, "Failed to initialize Twitch helix client")
	}

	resp, err := appClient.RequestAppAccessToken([]string{})
	if err != nil {
		fatalError(err, "Failed to get Twitch helix app token")
	}
	if resp.Error != "" {
		fatalError(errors.New(resp.Error), "Failed to get Twitch helix app token")
	}

	// Set the access token on the client
	appClient.SetAppAccessToken(resp.Data.AccessToken)
	log.Info("helix api access authorized")

	router := mux.NewRouter()
	router.Use(cors)
	apiRouter := router.PathPrefix("/api").Subrouter()
	bindApiRoutes(apiRouter)
	router.HandleFunc(redirectURL.Path, authorizeCallback)
	router.HandleFunc(webhookURL.Path+"/{user}", webhookCallback)
	router.HandleFunc("/ws", wrapAuth(func(w http.ResponseWriter, r *http.Request) {
		// Get user context
		claims := r.Context().Value(authKey).(*auth.UserClaims)
		hub.CreateClient(w, r, kv.ClientOptions{
			Namespace: userNamespace(claims.User),
		})
	}))
	httpLogger = wrapLogger("http")
	httpLogger.WithField("bind", *bind).Info("starting web server")

	fatalError(http.ListenAndServe(*bind, router), "HTTP server died unexepectedly")
}

func userNamespace(user string) string {
	return "@userdata/" + user + "/"
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
