package main

import (
	"encoding/json"
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

func bindApiRoutes(r *mux.Router) {
	get := r.Methods("GET").Subrouter()
	post := r.Methods("POST").Subrouter()

	post.HandleFunc("/auth", apiAuth)
	get.HandleFunc("/stream/{streamer}/info", wrapAuth(apiStreamerInfo))
	get.HandleFunc("/stream/{streamer}/status", wrapAuth(apiStreamerStatus))
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

		err := authStore.Verify(token)
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

		handler(w, r)
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

func apiStreamerInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamerLogin := vars["streamer"]
	resp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{streamerLogin},
	})
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		jsonErr(w, fmt.Sprintf("helix returned error: %s", resp.Error), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, resp.Data.Users)
}

func apiStreamerStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamerLogin := vars["streamer"]
	resp, err := client.GetStreams(&helix.StreamsParams{
		UserLogins: []string{streamerLogin},
	})
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		jsonErr(w, fmt.Sprintf("helix returned error: %s", resp.Error), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, resp.Data.Streams)
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
