package stulbe

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/strimertul/strimertul/modules/loyalty"
	"github.com/strimertul/stulbe/database"
)

func (b *Backend) apiLoyaltyConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	configKey := userNamespace(channel) + loyalty.ConfigKey

	data := loyalty.Config{}
	err = b.DB.GetJSON(configKey, &data)
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

func (b *Backend) apiLoyaltyRewards(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rewardKey := userNamespace(channel) + loyalty.RewardsKey

	data := loyalty.RewardStorage{}
	err = b.DB.GetJSON(rewardKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func (b *Backend) apiLoyaltyGoals(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelid"])
	if err != nil {
		jsonErr(w, "error fetching channel data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	goalKey := userNamespace(channel) + loyalty.GoalsKey

	data := loyalty.GoalStorage{}
	err = b.DB.GetJSON(goalKey, &data)
	if err != nil && err != database.ErrKeyNotFound {
		jsonErr(w, "error fetching data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, data)
}

func (b *Backend) apiLoyaltyUserData(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	channel, err := b.GetChannelByID(vars["channelid"])
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

	pointsKey := userNamespace(channel) + loyalty.PointsPrefix + user.Login
	var data loyalty.PointsEntry
	err = b.DB.GetJSON(pointsKey, &data)
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
