package stulbe

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix/v2"

	kv "github.com/strimertul/kilovolt/v8"
	"github.com/strimertul/stulbe/auth"
)

func (b Backend) apiTwitchAuthRedirect(w http.ResponseWriter, req *http.Request) {
	// Get user context
	claims, ok := req.Context().Value(authKey).(*auth.UserClaims)
	if !ok {
		jsonErr(w, "authorization required", http.StatusUnauthorized)
	}

	uri := b.Client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		ResponseType: "code",
		State:        claims.User,
		Scopes:       []string{"bits:read channel:read:subscriptions channel:read:redemptions channel:read:polls channel:read:predictions channel:read:hype_train user_read"},
	})
	jsonResponse(w, struct {
		AuthorizationURL string `json:"auth_url"`
	}{
		uri,
	})
}

type AuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int      `json:"expires_in"`
	Scope        []string `json:"scope"`
	Time         time.Time
}

const authKeysPrefix = "@twitch-auth/"

func (b *Backend) authorizeCallback(w http.ResponseWriter, req *http.Request) {
	// Get code from params
	code := req.URL.Query().Get("code")
	if code == "" {
		jsonErr(w, "missing code", http.StatusBadRequest)
		return
	}
	state := req.URL.Query().Get("state")
	// Exchange code for access/refresh tokens
	query := url.Values{
		"client_id":     {b.config.Twitch.ClientID},
		"client_secret": {b.config.Twitch.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {b.config.Twitch.RedirectURI},
	}
	authRequest, err := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token?"+query.Encode(), nil)
	if err != nil {
		jsonErr(w, "failed creating auth request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(authRequest)
	if err != nil {
		jsonErr(w, "failed sending auth request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	var authResp AuthResponse
	err = jsoniter.ConfigFastest.NewDecoder(resp.Body).Decode(&authResp)
	if err != nil && err != io.EOF {
		jsonErr(w, "failed reading auth response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	authResp.Time = time.Now()
	err = b.DB.PutJSON(authKeysPrefix+state, authResp)
	if err != nil {
		jsonErr(w, "error saving auth data for user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Subscribe to alerts
	client, err := helix.NewClient(&helix.Options{
		ClientID:        b.config.Twitch.ClientID,
		ClientSecret:    b.config.Twitch.ClientSecret,
		UserAccessToken: authResp.AccessToken,
	})
	if err != nil {
		jsonErr(w, "failed creating user client: "+err.Error(), http.StatusInternalServerError)
		return
	}
	users, err := client.GetUsers(&helix.UsersParams{})
	if err != nil {
		jsonErr(w, "failed looking up user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(users.Data.Users) < 1 {
		jsonErr(w, "no users found", http.StatusInternalServerError)
		return
	}
	user := users.Data.Users[0]
	_, err = b.ensureAlertSubscription(user.ID, state)
	if err != nil {
		jsonErr(w, "failed subscribing to alerts: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><h2>All done, you can close me now!</h2><script>window.close();</script></body></html>`)
}

type RefreshResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	Scope        []string `json:"scope"`
}

func (b *Backend) refreshAccessToken(refreshToken string) (r RefreshResponse, err error) {
	// Exchange code for access/refresh tokens
	query := url.Values{
		"client_id":     {b.config.Twitch.ClientID},
		"client_secret": {b.config.Twitch.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	authreq, err := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token?"+query.Encode(), nil)
	if err != nil {
		return RefreshResponse{}, err
	}
	resp, err := http.DefaultClient.Do(authreq)
	if err != nil {
		return RefreshResponse{}, err
	}
	defer resp.Body.Close()
	var refreshResp RefreshResponse
	err = jsoniter.ConfigFastest.NewDecoder(resp.Body).Decode(&refreshResp)
	return refreshResp, err
}

var subscriptionVersions = map[string]string{
	"channel.update":                                         "1",
	"channel.follow":                                         "1",
	"channel.subscribe":                                      "1",
	"channel.subscription.gift":                              "1",
	"channel.subscription.message":                           "1",
	"channel.cheer":                                          "1",
	"channel.raid":                                           "1",
	"channel.poll.begin":                                     "1",
	"channel.poll.progress":                                  "1",
	"channel.poll.end":                                       "1",
	"channel.prediction.begin":                               "1",
	"channel.prediction.progress":                            "1",
	"channel.prediction.lock":                                "1",
	"channel.prediction.end":                                 "1",
	"channel.hype_train.begin":                               "1",
	"channel.hype_train.progress":                            "1",
	"channel.hype_train.end":                                 "1",
	"channel.channel_points_custom_reward.add":               "1",
	"channel.channel_points_custom_reward.update":            "1",
	"channel.channel_points_custom_reward.remove":            "1",
	"channel.channel_points_custom_reward_redemption.add":    "1",
	"channel.channel_points_custom_reward_redemption.update": "1",
	"stream.online":                                          "1",
	"stream.offline":                                         "1",
}

func (b *Backend) ensureAlertSubscription(id string, state string) (int, error) {
	//TODO Proper cursor stuff but seriously who has more than 100??
	subs, err := b.Client.GetEventSubSubscriptions(&helix.EventSubSubscriptionsParams{})
	if err != nil {
		return -1, err
	}
	webhook := fmt.Sprintf("%s/%s", b.config.WebhookURL, state)
	subscriptions := map[string]bool{
		"channel.update":                                         false,
		"channel.follow":                                         false,
		"channel.subscribe":                                      false,
		"channel.subscription.gift":                              false,
		"channel.subscription.message":                           false,
		"channel.cheer":                                          false,
		"channel.raid":                                           false,
		"channel.poll.begin":                                     false,
		"channel.poll.progress":                                  false,
		"channel.poll.end":                                       false,
		"channel.prediction.begin":                               false,
		"channel.prediction.progress":                            false,
		"channel.prediction.lock":                                false,
		"channel.prediction.end":                                 false,
		"channel.hype_train.begin":                               false,
		"channel.hype_train.progress":                            false,
		"channel.hype_train.end":                                 false,
		"channel.channel_points_custom_reward.add":               false,
		"channel.channel_points_custom_reward.update":            false,
		"channel.channel_points_custom_reward.remove":            false,
		"channel.channel_points_custom_reward_redemption.add":    false,
		"channel.channel_points_custom_reward_redemption.update": false,
		"stream.online":                                          false,
		"stream.offline":                                         false,
	}
	transport := helix.EventSubTransport{
		Method:   "webhook",
		Callback: webhook,
		Secret:   b.config.WebhookSecret,
	}
	condition := func(topic string, id string) helix.EventSubCondition {
		switch topic {
		case "channel.raid":
			return helix.EventSubCondition{
				ToBroadcasterUserID: id,
			}
		default:
			return helix.EventSubCondition{
				BroadcasterUserID: id,
			}
		}
	}
	for _, sub := range subs.Data.EventSubSubscriptions {
		// Ignore subscriptions that aren't for this service
		if sub.Transport.Callback != webhook {
			continue
		}
		if sub.Status != "enabled" {
			// Either revoked or inactive for some reason, remove it so we can make it again
			_, err := b.Client.RemoveEventSubSubscription(sub.ID)
			if err != nil {
				b.Log.Error("Failed to remove event subscription", zap.Error(err))
			}
		} else {
			subscriptions[sub.Type] = true
		}
	}
	cost := 0
	for topic, subscribed := range subscriptions {
		if !subscribed {
			sub, err := b.Client.CreateEventSubSubscription(&helix.EventSubSubscription{
				Type:      topic,
				Version:   subscriptionVersions[topic],
				Status:    "enabled",
				Transport: transport,
				Condition: condition(topic, id),
			})
			if sub.Error != "" || sub.ErrorMessage != "" {
				b.Log.Error("subscription error", zap.String("err", sub.Error), zap.String("message", sub.ErrorMessage))
				return -1, errors.New(sub.Error + ": " + sub.ErrorMessage)
			}
			cost = sub.Data.TotalCost
			if err != nil {
				return -1, err
			}
		}
	}
	return cost, nil
}

func (b *Backend) getUserClient(req *http.Request) (*helix.Client, error) {
	// Get user context
	claims, ok := req.Context().Value(authKey).(*auth.UserClaims)
	if !ok {
		return nil, errors.New("authorization required")
	}

	// Get user's access token
	var tokens AuthResponse
	err := b.DB.GetJSON(authKeysPrefix+claims.User, &tokens)
	if err != nil {
		return nil, err
	}

	// Handle token expiration
	if time.Now().After(tokens.Time.Add(time.Duration(tokens.ExpiresIn) * time.Second)) {
		// Refresh tokens
		refreshed, err := b.refreshAccessToken(tokens.RefreshToken)
		if err != nil {
			return nil, err
		}
		tokens.AccessToken = refreshed.AccessToken
		tokens.RefreshToken = refreshed.RefreshToken

		// Save new token pair
		err = b.DB.PutJSON(authKeysPrefix+claims.User, tokens)
		if err != nil {
			return nil, err
		}
	}

	// Create user-specific client
	return helix.NewClient(&helix.Options{
		ClientID:        b.config.Twitch.ClientID,
		ClientSecret:    b.config.Twitch.ClientSecret,
		UserAccessToken: tokens.AccessToken,
	})
}

func (b *Backend) apiTwitchUserData(w http.ResponseWriter, req *http.Request) {
	client, err := b.getUserClient(req)
	if err != nil {
		if err == kv.ErrorKeyNotFound {
			jsonErr(w, "twitch user not authenticated", http.StatusFailedDependency)
			return
		}
		jsonErr(w, "failed getting user client: "+err.Error(), http.StatusInternalServerError)
		return
	}
	users, err := client.GetUsers(&helix.UsersParams{})
	if err != nil {
		jsonErr(w, "failed looking up user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if (users.Error != "") || (users.ErrorMessage != "") {
		jsonErr(w, "error looking up user: "+users.Error+": "+users.ErrorMessage, http.StatusInternalServerError)
		return
	}
	jsonResponse(w, users.Data.Users[0])
}

func (b *Backend) apiTwitchListSubscriptions(w http.ResponseWriter, req *http.Request) {
	claims := req.Context().Value(authKey).(*auth.UserClaims)
	if claims.Level != auth.ULAdmin {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
	}
	subs, err := b.Client.GetEventSubSubscriptions(&helix.EventSubSubscriptionsParams{})
	if err != nil {
		jsonErr(w, "failed getting subscriptions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if (subs.Error != "") || (subs.ErrorMessage != "") {
		jsonErr(w, "error getting subscriptions: "+subs.Error+": "+subs.ErrorMessage, http.StatusInternalServerError)
		return
	}
	jsonResponse(w, subs.Data.EventSubSubscriptions)
}

func (b *Backend) apiTwitchClearSubscriptions(w http.ResponseWriter, req *http.Request) {
	claims := req.Context().Value(authKey).(*auth.UserClaims)
	if claims.Level != auth.ULAdmin {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	deleted, err := b.ClearSubscriptions(claims.User)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, struct {
		Ok      bool `json:"ok"`
		Deleted int  `json:"deleted"`
	}{
		true,
		deleted,
	})
}

func (b *Backend) ClearSubscriptions(user string) (int, error) {
	subs, err := b.Client.GetEventSubSubscriptions(&helix.EventSubSubscriptionsParams{})
	if err != nil {
		return -1, fmt.Errorf("failed looking up subscriptions: %w", err)
	}
	deleted := 0
	for _, sub := range subs.Data.EventSubSubscriptions {
		// Ignore subscriptions that aren't for this service
		if !strings.HasSuffix(sub.Transport.Callback, user) {
			continue
		}
		_, err := b.Client.RemoveEventSubSubscription(sub.ID)
		if err != nil {
			return deleted, fmt.Errorf("failed removing subscription: %w", err)
		}
		deleted++
	}
	return deleted, nil
}
