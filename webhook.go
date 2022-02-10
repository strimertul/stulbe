package stulbe

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix/v2"
)

type eventSubNotification struct {
	Subscription helix.EventSubSubscription `json:"subscription"`
	Challenge    string                     `json:"challenge"`
	Event        json.RawMessage            `json:"event"`
}

const MAX_ARCHIVE = 100

var webhookMutex sync.Mutex

func (b *Backend) webhookCallback(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		b.Log.Error("Could not read request body", zap.Error(err))
		return
	}
	defer req.Body.Close()

	// Verify signature for webhook
	if !helix.VerifyEventSubNotification(b.config.WebhookSecret, req.Header, string(body)) {
		b.Log.Error("Received invalid webhook")
		return
	}

	// Check if we processed this webhook already
	messageID := req.Header.Get("Twitch-Eventsub-Message-Id")
	timestamp := req.Header.Get("Twitch-Eventsub-Message-Timestamp")
	if messageID != "" {
		if b.webhookcache.Contains(messageID) {
			b.Log.Debug("Received duplicate webhook, ignoring", zap.String("messageID", messageID))
			_, _ = fmt.Fprintf(w, "Ok")
			return
		}
	}
	defer b.webhookcache.Add(messageID, timestamp)

	var vals eventSubNotification
	err = jsoniter.ConfigFastest.Unmarshal(body, &vals)
	if err != nil {
		b.Log.Error("cannot decode event", zap.Error(err))
		return
	}

	// if there's a challenge in the request, respond with only the challenge to verify your eventsub.
	if vals.Challenge != "" {
		_, err = w.Write([]byte(vals.Challenge))
		if err != nil {
			b.Log.Error("cannot write challenge", zap.Error(err))
		}
		return
	}
	webhookMutex.Lock()
	defer webhookMutex.Unlock()
	err = b.DB.PutKey(userNamespace(vars["user"])+"stulbe/ev/webhook", string(body))
	if err != nil {
		b.Log.Error("Could not store event in KV", zap.Error(err))
	}
	var archive []eventSubNotification
	err = b.DB.GetJSON(userNamespace(vars["user"])+"stulbe/last-webhooks", &archive)
	if err != nil {
		archive = []eventSubNotification{}
	}
	archive = append(archive, vals)
	if len(archive) > MAX_ARCHIVE {
		archive = archive[len(archive)-MAX_ARCHIVE:]
	}
	err = b.DB.PutJSON(userNamespace(vars["user"])+"stulbe/last-webhooks", archive)
	if err != nil {
		b.Log.Error("Could not store archive in KV", zap.Error(err))
	}
	_, _ = fmt.Fprintf(w, "Ok")
}
