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

	"github.com/strimertul/strimertul/modules/loyalty"

	"github.com/strimertul/stulbe/api"
	"github.com/strimertul/stulbe/auth"
	"github.com/strimertul/stulbe/database"
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
	r.Use(cors)
	get := r.Methods("GET").Subrouter()
	post := r.Methods("POST").Subrouter()

	// Auth endpoint (for privileged apps)
	post.HandleFunc("/auth", apiAuth)

	// Loyalty endpoints (public)
	get.HandleFunc("/stream/{channelid}/loyalty/config", apiLoyaltyConfig)
	get.HandleFunc("/stream/{channelid}/loyalty/rewards", apiLoyaltyRewards)
	get.HandleFunc("/stream/{channelid}/loyalty/goals", apiLoyaltyGoals)
	get.HandleFunc("/stream/{channelid}/loyalty/info/{uid}", apiLoyaltyUserData)

	// Redeem endpoints (for twitch users)
	post.HandleFunc("/twitch-ext/reward/redeem", apiExLoyaltyRedeem)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS,PUT")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type")

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

func apiLoyaltyConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := getChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	configKey := userNamespace(channel) + loyalty.ConfigKey

	data := loyalty.Config{}
	err = db.GetJSON(configKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, struct {
		Currency string `json:"currency"`
		Interval int64  `json:"interval"`
	}{
		Currency: data.Currency,
		Interval: data.Points.Interval,
	})
}

func apiLoyaltyRewards(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := getChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rewardKey := userNamespace(channel) + loyalty.RewardsKey

	data := loyalty.RewardStorage{}
	err = db.GetJSON(rewardKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func apiLoyaltyGoals(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := getChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	goalKey := userNamespace(channel) + loyalty.GoalsKey

	data := loyalty.GoalStorage{}
	err = db.GetJSON(goalKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func apiLoyaltyUserData(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := getChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userid := vars["uid"]
	if !strings.HasPrefix(userid, "U") {
		jsonErr(w, "invalid user id", http.StatusBadRequest)
		return
	}
	user, err := getUserByID(userid[1:])
	if err != nil {
		jsonErr(w, "error fetching username: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pointsKey := userNamespace(channel) + loyalty.PointsPrefix + user.Login
	var data loyalty.PointsEntry
	err = db.GetJSON(pointsKey, &data)
	if err != nil {
		if err != database.ErrKeyNotFound {
			jsonErr(w, "error fetching points: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			data = loyalty.PointsEntry{Points: 0}
		}
	}

	jsonResponse(w, struct {
		DisplayName string `json:"display_name"`
		Balance     int64  `json:"balance"`
	}{
		DisplayName: user.DisplayName,
		Balance:     data.Points,
	})
}

func apiExLoyaltyRedeem(w http.ResponseWriter, r *http.Request) {
	// Get user data
	var data struct {
		Token    string `json:"token"`
		RewardID string `json:"reward_id"`
	}
	err := jsoniter.ConfigFastest.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		jsonErr(w, "error decoding JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse JWT and extract data from it
	t, err := jwt.ParseWithClaims(data.Token, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		return twitchExtensionJWT, nil
	})
	if err != nil {
		jsonErr(w, "error decoding JWT token: "+err.Error(), http.StatusBadRequest)
		return
	}

	claims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		jsonErr(w, "something's wrong I can feel it", http.StatusInternalServerError)
		return
	}

	// Get channel/user from IDs
	channel, err := getChannelByID(claims["channel_id"].(string))
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := getUserByID(claims["user_id"].(string))
	if err != nil {
		jsonErr(w, "error fetching username: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Send event to hub via database
	err = db.PutJSON(userNamespace(channel)+api.KVExLoyaltyRedeem, api.ExLoyaltyRedeem{
		Username:    user.Login,
		DisplayName: user.DisplayName,
		Channel:     channel,
		RewardID:    data.RewardID,
	})
	if err != nil {
		jsonErr(w, "error sending request to DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, api.StatusResponse{Ok: true})
}

func getChannelByID(channelid string) (string, error) {
	channelName, ok := channelcache.Get(channelid)
	if !ok {
		resp, err := client.GetChannelInformation(&helix.GetChannelInformationParams{
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
		resp, err := client.GetUsers(&helix.UsersParams{
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
