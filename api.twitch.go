package main

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
	"github.com/strimertul/stulbe/auth"
)

func apiTwitchAuthRedirect(w http.ResponseWriter, req *http.Request) {
	// Get user context
	claims, ok := req.Context().Value(authKey).(*auth.UserClaims)
	if !ok {
		jsonErr(w, "authorization required", http.StatusUnauthorized)
	}

	uri := appClient.GetAuthorizationURL(&helix.AuthorizationURLParams{
		ResponseType: "code",
		State:        claims.User,
		Scopes:       []string{"bits:read channel:read:subscriptions channel:read:redemptions channel:read:polls channel:read:predictions channel:read:hype_train"},
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

func authorizeCallback(w http.ResponseWriter, req *http.Request) {
	// Get code from params
	code := req.URL.Query().Get("code")
	if code == "" {
		jsonErr(w, "missing code", http.StatusBadRequest)
		return
	}
	state := req.URL.Query().Get("state")
	// Exchange code for access/refresh tokens
	query := url.Values{
		"client_id":     {options.ClientID},
		"client_secret": {options.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {options.RedirectURI},
	}
	authreq, err := http.NewRequest("POST", "https://id.twitch.tv/oauth2/token?"+query.Encode(), nil)
	if err != nil {
		jsonErr(w, "failed creating auth request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(authreq)
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
	err = db.PutJSON(authKeysPrefix+state, authResp)
	if err != nil {
		jsonErr(w, "error saving auth data for user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, struct {
		Ok bool `json:"ok"`
	}{
		Ok: true,
	})
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

func refreshAccessToken(refreshToken string) (r RefreshResponse, err error) {
	// Exchange code for access/refresh tokens
	uri, err := url.Parse("https://id.twitch.tv/oauth2/token")
	if err != nil {
		return RefreshResponse{}, err
	}
	query := uri.Query()
	query.Add("client_id", options.ClientID)
	query.Add("client_secret", options.ClientSecret)
	query.Add("grant_type", "refresh_token")
	query.Add("refresh_token", refreshToken)
	authreq, err := http.NewRequest("POST", uri.String(), nil)
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

func getUserClient(req *http.Request) (*helix.Client, error) {
	// Get user context
	claims, ok := req.Context().Value(authKey).(*auth.UserClaims)
	if !ok {
		return nil, errors.New("authorization required")
	}

	// Get user's access token
	var tokens AuthResponse
	err := db.GetJSON(authKeysPrefix+claims.User, &tokens)
	if err != nil {
		return nil, err
	}

	// Handle token expiration
	if time.Now().After(tokens.Time.Add(time.Duration(tokens.ExpiresIn) * time.Second)) {
		// Refresh tokens
		refreshed, err := refreshAccessToken(tokens.RefreshToken)
		if err != nil {
			return nil, err
		}
		tokens.AccessToken = refreshed.AccessToken
		tokens.RefreshToken = refreshed.RefreshToken

		// Save new token pair
		err = db.PutJSON(authKeysPrefix+claims.User, tokens)
		if err != nil {
			return nil, err
		}
	}

	// Create user-specific client
	return helix.NewClient(&helix.Options{
		ClientID:        options.ClientID,
		ClientSecret:    options.ClientSecret,
		UserAccessToken: tokens.AccessToken,
	})
}

func apiTwitchUserData(w http.ResponseWriter, req *http.Request) {
	client, err := getUserClient(req)
	if err != nil {
		jsonErr(w, "failed getting user client: "+err.Error(), http.StatusInternalServerError)
		return
	}
	users, err := client.GetUsers(&helix.UsersParams{})
	if err != nil {
		jsonErr(w, "failed looking up user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, users.Data.Users[0])
}
