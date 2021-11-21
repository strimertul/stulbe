package stulbe

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	kv "github.com/strimertul/kilovolt/v6"
	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nicklaw5/helix"
	"github.com/sirupsen/logrus"
)

type BackendConfig struct {
	WebhookURL    string
	RedirectURL   string
	WebhookSecret string
	Twitch        *helix.Options
}

type Backend struct {
	Hub    *kv.Hub
	DB     *database.DB
	Auth   *auth.Storage
	Log    logrus.FieldLogger
	Client *helix.Client

	config       BackendConfig
	usercache    *lru.Cache
	channelcache *lru.Cache
	webhookURL   *url.URL
	redirectURL  *url.URL
	httpLogger   logrus.FieldLogger
}

func NewBackend(db *database.DB, authStore *auth.Storage, config BackendConfig, log logrus.FieldLogger) (*Backend, error) {
	if log == nil {
		log = logrus.New()
	}

	redirectURL, err := url.Parse(config.RedirectURL)
	if err != nil {
		return nil, fmt.Errorf("REDIRECT_URI parsing error: %w)", err)
	}

	webhookURL, err := url.Parse(config.WebhookURL)
	if err != nil {
		return nil, fmt.Errorf("WEBHOOK_URI parsing error: %w)", err)
	}

	// Initialize caches to avoid spamming Twitch's APIs
	usercache, err := lru.New(128)
	if err != nil {
		return nil, fmt.Errorf("could not create LRU cache for users: %w", err)
	}
	channelcache, err := lru.New(128)
	if err != nil {
		return nil, fmt.Errorf("could not create LRU cache for channels: %w", err)
	}

	// Create client for Twitch APIs
	client, err := helix.NewClient(config.Twitch)
	if err != nil {
		return nil, fmt.Errorf("could not create twitch client: %w", err)
	}

	resp, err := client.RequestAppAccessToken([]string{})
	if err != nil {
		return nil, fmt.Errorf("Failed to get Twitch helix app token: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("Failed to get Twitch helix app token: %w", errors.New(resp.Error))
	}

	// Set the access token on the client
	client.SetAppAccessToken(resp.Data.AccessToken)
	log.Info("helix api access authorized")

	// Initialize KV (required)
	hub, err := kv.NewHub(db.Client(), kv.HubOptions{}, wrapLogger(log, "kv"))
	if err != nil {
		return nil, fmt.Errorf("could not initialize KV hub: %w", err)
	}
	go hub.Run()

	return &Backend{
		Auth:   authStore,
		Log:    log,
		Client: client,
		Hub:    hub,

		usercache:    usercache,
		channelcache: channelcache,
		httpLogger:   wrapLogger(log, "http"),
		webhookURL:   webhookURL,
		redirectURL:  redirectURL,
		config:       config,
	}, nil
}

func (b *Backend) RunHTTPServer(bind string) error {
	router := b.BindRoutes()
	b.httpLogger.WithField("bind", bind).Info("starting web server")
	return http.ListenAndServe(bind, router)
}

func wrapLogger(log logrus.FieldLogger, module string) logrus.FieldLogger {
	return log.WithField("module", module)
}

func userNamespace(user string) string {
	return "@userdata/" + user + "/"
}
