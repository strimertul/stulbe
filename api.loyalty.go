package stulbe

import (
	"net/http"
	"strings"

	kv "github.com/strimertul/kilovolt/v8"

	"github.com/gorilla/mux"
)

const loyaltyConfigKey = "loyalty/config"

const loyaltyRewardsKey = "loyalty/rewards"

type loyaltyRewardStorage []loyaltyReward

const loyaltyGoalsKey = "loyalty/goals"

type loyaltyGoalStorage []loyaltyGoal

type loyaltyReward struct {
	Enabled       bool   `json:"enabled"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Image         string `json:"image"`
	Price         int64  `json:"price"`
	CustomRequest string `json:"required_info,omitempty"`
	Cooldown      int64  `json:"cooldown"`
}

type loyaltyGoal struct {
	Enabled      bool             `json:"enabled"`
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Image        string           `json:"image"`
	TotalGoal    int64            `json:"total"`
	Contributed  int64            `json:"contributed"`
	Contributors map[string]int64 `json:"contributors"`
}

const loyaltyPointsPrefix = "loyalty/points/"

type loyaltyPointsEntry struct {
	Points int64 `json:"points"`
}

// Subset of the actual Loyalty config
type loyaltyConfig struct {
	Currency string `json:"currency"`
	Points   struct {
		Interval      int64 `json:"interval"` // in seconds!
		Amount        int64 `json:"amount"`
		ActivityBonus int64 `json:"activity_bonus"`
	} `json:"points"`
}

func (b *Backend) apiLoyaltyConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelID"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	configKey := userNamespace(channel) + loyaltyConfigKey

	data := loyaltyConfig{}
	err = b.DB.GetJSON(configKey, &data)
	if err != nil && err != kv.ErrorKeyNotFound {
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

func (b *Backend) apiLoyaltyRewards(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelID"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rewardKey := userNamespace(channel) + loyaltyRewardsKey

	data := loyaltyRewardStorage{}
	err = b.DB.GetJSON(rewardKey, &data)
	if err != nil && err != kv.ErrorKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func (b *Backend) apiLoyaltyGoals(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelID"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	goalKey := userNamespace(channel) + loyaltyGoalsKey

	data := loyaltyGoalStorage{}
	err = b.DB.GetJSON(goalKey, &data)
	if err != nil && err != kv.ErrorKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func (b *Backend) apiLoyaltyUserData(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelID"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userid := vars["uid"]
	if !strings.HasPrefix(userid, "U") {
		jsonErr(w, "invalid user id", http.StatusBadRequest)
		return
	}
	user, err := b.GetUserByID(userid[1:])
	if err != nil {
		jsonErr(w, "error fetching username: "+err.Error(), http.StatusInternalServerError)
		return
	}

	pointsKey := userNamespace(channel) + loyaltyPointsPrefix + user.Login
	var data loyaltyPointsEntry
	err = b.DB.GetJSON(pointsKey, &data)
	if err != nil {
		if err != kv.ErrorKeyNotFound {
			jsonErr(w, "error fetching points: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			data = loyaltyPointsEntry{Points: 0}
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
