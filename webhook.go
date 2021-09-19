package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix"
)

type eventSubNotification struct {
	Subscription helix.EventSubSubscription `json:"subscription"`
	Challenge    string                     `json:"challenge"`
	Event        json.RawMessage            `json:"event"`
}

const MAX_ARCHIVE = 100

func webhookCallback(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.WithError(err).Error("Could not read request body")
		return
	}
	defer req.Body.Close()

	// Verify signature for webhook
	if !helix.VerifyEventSubNotification(webhookSecret, req.Header, string(body)) {
		log.Error("Received invalid webhook")
		return
	}
	var vals eventSubNotification
	err = jsoniter.ConfigFastest.Unmarshal(body, &vals)
	if err != nil {
		log.Println(err)
		return
	}
	// if there's a challenge in the request, respond with only the challenge to verify your eventsub.
	if vals.Challenge != "" {
		w.Write([]byte(vals.Challenge))
		return
	}
	err = db.PutKey(userNamespace(vars["user"])+"stulbe/ev/webhook", body)
	if err != nil {
		log.WithError(err).Error("Could not store event in KV")
	}
	var archive []eventSubNotification
	err = db.GetJSON(userNamespace(vars["user"])+"stulbe/last-webhooks", &archive)
	if err != nil {
		archive = []eventSubNotification{}
	}
	archive = append(archive, vals)
	if len(archive) > MAX_ARCHIVE {
		archive = archive[len(archive)-MAX_ARCHIVE:]
	}
	err = db.PutJSON(userNamespace(vars["user"])+"stulbe/last-webhooks", archive)
	if err != nil {
		log.WithError(err).Error("Could not store archive in KV")
	}
}
