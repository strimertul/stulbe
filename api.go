package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
	"github.com/strimertul/stulbe/api"
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
		token := r.Header.Get("Authorization")
		parts := strings.Fields(token)

		// Auth required, fail if not provided
		if len(parts) < 2 || strings.ToLower(parts[0]) != "bearer" {
			unauthorized(w)
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
	jsoniter.ConfigFastest.NewEncoder(w).Encode(resp.Data.Users)
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
	jsoniter.ConfigFastest.NewEncoder(w).Encode(resp.Data.Streams)
}

func unauthorized(w http.ResponseWriter) {
	msg, _ := jsoniter.ConfigFastest.MarshalToString(api.ResponseError{Ok: false, Error: "authentication required"})
	jsonErr(w, msg, http.StatusUnauthorized)
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
