package stulbe

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"

	lru "github.com/hashicorp/golang-lru"
	"github.com/nicklaw5/helix/v2"
	kv "github.com/strimertul/kilovolt/v8"
)

type BackendConfig struct {
	WebhookURL    string
	RedirectURL   string
	WebhookSecret string
	Twitch        *helix.Options
}

type Backend struct {
	Hub    *kv.Hub
	DB     *database.DBModule
	Auth   *auth.Storage
	Log    *zap.Logger
	Client *helix.Client

	config       BackendConfig
	usercache    *lru.Cache
	channelcache *lru.Cache
	webhookURL   *url.URL
	redirectURL  *url.URL
	httpLogger   *zap.Logger
}

func NewBackend(hub *kv.Hub, db *database.DBModule, authStore *auth.Storage, config BackendConfig, log *zap.Logger) (*Backend, error) {
	if log == nil {
		log, _ = zap.NewProduction()
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

	return &Backend{
		Auth:   authStore,
		Log:    log,
		Client: client,
		Hub:    hub,
		DB:     db,

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
	b.httpLogger.Info("starting web server", zap.String("bind", bind))
	return http.ListenAndServe(bind, router)
}

func wrapLogger(log *zap.Logger, module string) *zap.Logger {
	return log.With(zap.String("module", module))
}

func userNamespace(user string) string {
	return "@userdata/" + user + "/"
}
