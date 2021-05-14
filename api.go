package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"

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

	post.HandleFunc("/auth", apiAuth)
	get.HandleFunc("/stream/{streamer}/loyalty/rewards", apiLoyaltyRewards)
	get.HandleFunc("/stream/{streamer}/loyalty/goals", apiLoyaltyGoals)
	get.HandleFunc("/stream/{streamer}/loyalty/points/{user}", apiLoyaltyUserPoints)
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

func apiLoyaltyRewards(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rewardKey := remapForUser(vars["streamer"])(loyalty.RewardsKey)

	data := loyalty.RewardStorage{}
	err := db.GetJSON(rewardKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func apiLoyaltyGoals(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	goalKey := remapForUser(vars["streamer"])(loyalty.GoalsKey)

	data := loyalty.GoalStorage{}
	err := db.GetJSON(goalKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func apiLoyaltyUserPoints(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pointsKey := remapForUser(vars["streamer"])(loyalty.PointsKey)
	user := vars["user"]

	data := make(loyalty.PointStorage)
	err := db.GetJSON(pointsKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	balance := int64(0)
	if val, ok := data[user]; ok {
		balance = val
	}

	jsonResponse(w, balance)
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
