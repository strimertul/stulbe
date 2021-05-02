package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
)

type APIError struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

func bindApiRoutes(r *mux.Router) {
	// Strimertul facing APIs
	stulRouter := r.PathPrefix("/stul").Subrouter()
	stulRouter.HandleFunc("/stream/{streamer}/info", apiStreamerInfo)
	stulRouter.HandleFunc("/stream/{streamer}/status", apiStreamerStatus)
}

func apiStreamerInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamerLogin := vars["streamer"]
	resp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{streamerLogin},
	})
	if err != nil {
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		jsonErr(w, fmt.Errorf("helix returned error: %s", resp.Error), http.StatusInternalServerError)
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
		jsonErr(w, err, http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		jsonErr(w, fmt.Errorf("helix returned error: %s", resp.Error), http.StatusInternalServerError)
		return
	}
	jsoniter.ConfigFastest.NewEncoder(w).Encode(resp.Data.Streams)
}

func jsonErr(w http.ResponseWriter, err error, code int) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	return jsoniter.ConfigFastest.NewEncoder(w).Encode(APIError{
		Ok:    false,
		Error: err.Error(),
	})
}
