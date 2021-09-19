package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"

	"github.com/strimertul/stulbe/api"
	"github.com/strimertul/stulbe/auth"
)

type APIError struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

type contextKey int

const (
	authKey contextKey = iota
)

func bindApiRoutes(r *mux.Router) {
	get := r.Methods("GET", "OPTIONS").Subrouter()
	post := r.Methods("POST", "OPTIONS").Subrouter()

	// Auth endpoint (for privileged apps)
	post.HandleFunc("/auth", apiAuth)

	// Loyalty endpoints (public)
	get.HandleFunc("/stream/{channelid}/loyalty/config", apiLoyaltyConfig)
	get.HandleFunc("/stream/{channelid}/loyalty/rewards", apiLoyaltyRewards)
	get.HandleFunc("/stream/{channelid}/loyalty/goals", apiLoyaltyGoals)
	get.HandleFunc("/stream/{channelid}/loyalty/info/{uid}", apiLoyaltyUserData)

	post.HandleFunc("/twitch/authorize", wrapAuth(apiTwitchAuthRedirect))
	get.HandleFunc("/twitch/user", wrapAuth(apiTwitchUserData))
	get.HandleFunc("/twitch/list", wrapAuth(apiTwitchListSubscriptions))
	post.HandleFunc("/twitch/clear", wrapAuth(apiTwitchClearSubscriptions))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	})
}

// wrapAuth implements Basic Auth authorization for provided endpoints
// This is not as secure as it should be but it will probably work ok for now
func wrapAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user credentials
		header := r.Header.Get("Authorization")
		parts := strings.Fields(header)

		// Auth required, fail if not provided
		if len(parts) < 2 || strings.ToLower(parts[0]) != "bearer" {
			unauthorized(w)
			return
		}

		token := parts[1]

		claims, err := authStore.Verify(token)
		if err != nil {
			switch err {
			case auth.ErrTokenExpired:
				jsonErr(w, "authentication required", http.StatusUnauthorized)
			case auth.ErrTokenParseFailed:
				jsonErr(w, "invalid token", http.StatusBadRequest)
			default:
				jsonErr(w, err.Error(), http.StatusBadRequest)
			}
			return
		}

		ctx := context.WithValue(r.Context(), authKey, claims)
		handler(w, r.WithContext(ctx))
	}
}

func apiAuth(w http.ResponseWriter, r *http.Request) {
	var authPayload api.AuthRequest
	err := json.NewDecoder(r.Body).Decode(&authPayload)
	if err != nil {
		jsonErr(w, fmt.Sprintf("invalid json body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	user, token, err := authStore.Authenticate(authPayload.User, authPayload.AuthKey, jwt.StandardClaims{
		ExpiresAt: time.Now().Add(time.Hour * 24 * 7).Unix(),
	})

	if err != nil {
		if err == auth.ErrInvalidKey || err == auth.ErrUserNotFound {
			jsonErr(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		httpLogger.WithError(err).Error("internal error while authenticating")
		jsonErr(w, fmt.Sprintf("server error: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, api.AuthResponse{
		Ok:    true,
		User:  user.User,
		Level: string(user.Level),
		Token: token,
	})
}
func getChannelByID(channelid string) (string, error) {
	channelName, ok := channelcache.Get(channelid)
	if !ok {
		resp, err := appClient.GetChannelInformation(&helix.GetChannelInformationParams{
			BroadcasterID: channelid,
		})
		if err != nil {
			return "", fmt.Errorf("error fetching data: " + err.Error())
		}
		if resp.Error != "" {
			return "", errors.New(resp.Error)
		}
		if len(resp.Data.Channels) < 1 {
			return "", errors.New("user id not found")
		}
		channelName = strings.ToLower(resp.Data.Channels[0].BroadcasterName)
		channelcache.Add(channelid, channelName)
	}
	return channelName.(string), nil
}

func getUserByID(userid string) (helix.User, error) {
	username, ok := usercache.Get(userid)
	if !ok {
		resp, err := appClient.GetUsers(&helix.UsersParams{
			IDs: []string{userid},
		})
		if err != nil {
			return helix.User{}, fmt.Errorf("error fetching data: " + err.Error())
		}
		if resp.Error != "" {
			return helix.User{}, errors.New(resp.Error)
		}
		if len(resp.Data.Users) < 1 {
			return helix.User{}, errors.New("user id not found")
		}
		username = resp.Data.Users[0]
		usercache.Add(userid, username)
	}
	return username.(helix.User), nil
}

func unauthorized(w http.ResponseWriter) {
	jsonErr(w, "authentication required", http.StatusUnauthorized)
}

func jsonResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	return jsoniter.ConfigFastest.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, message string, code int) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	return jsoniter.ConfigFastest.NewEncoder(w).Encode(APIError{
		Ok:    false,
		Error: message,
	})
}
