package stulbe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix/v2"
	kv "github.com/strimertul/kilovolt/v8"

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

func (b *Backend) BindRoutes() *mux.Router {
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api").Subrouter()
	b.bindApiRoutes(apiRouter)
	router.HandleFunc(b.redirectURL.Path, b.authorizeCallback)
	router.HandleFunc(b.webhookURL.Path+"/{user}", b.webhookCallback)
	router.HandleFunc("/ws", b.wrapAuth(func(w http.ResponseWriter, r *http.Request) {
		// Get user context
		claims := r.Context().Value(authKey).(*auth.UserClaims)
		b.Hub.CreateWebsocketClient(w, r, kv.ClientOptions{
			Namespace: userNamespace(claims.User),
		})
	}))
	router.Use(Cors)
	return router
}

func (b *Backend) bindApiRoutes(r *mux.Router) {
	get := r.Methods("GET", "OPTIONS").Subrouter()
	post := r.Methods("POST", "OPTIONS").Subrouter()

	// Auth endpoint (for privileged apps)
	post.HandleFunc("/auth", b.apiAuth)

	// Loyalty endpoints (public)
	get.HandleFunc("/stream/{channelid}/loyalty/config", b.apiLoyaltyConfig)
	get.HandleFunc("/stream/{channelid}/loyalty/rewards", b.apiLoyaltyRewards)
	get.HandleFunc("/stream/{channelid}/loyalty/goals", b.apiLoyaltyGoals)
	get.HandleFunc("/stream/{channelid}/loyalty/info/{uid}", b.apiLoyaltyUserData)

	post.HandleFunc("/twitch/authorize", b.wrapAuth(b.apiTwitchAuthRedirect))
	get.HandleFunc("/twitch/user", b.wrapAuth(b.apiTwitchUserData))
	get.HandleFunc("/twitch/list", b.wrapAuth(b.apiTwitchListSubscriptions))
	post.HandleFunc("/twitch/clear", b.wrapAuth(b.apiTwitchClearSubscriptions))
}

func Cors(next http.Handler) http.Handler {
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
func (b *Backend) wrapAuth(handler http.HandlerFunc) http.HandlerFunc {
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

		claims, err := b.Auth.Verify(token)
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

func (b *Backend) apiAuth(w http.ResponseWriter, r *http.Request) {
	var authPayload api.AuthRequest
	err := json.NewDecoder(r.Body).Decode(&authPayload)
	if err != nil {
		jsonErr(w, fmt.Sprintf("invalid json body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	user, token, err := b.Auth.Authenticate(authPayload.User, authPayload.AuthKey, jwt.StandardClaims{
		ExpiresAt: time.Now().Add(time.Hour * 24 * 7).Unix(),
	})

	if err != nil {
		if err == auth.ErrInvalidKey || err == auth.ErrUserNotFound {
			jsonErr(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		b.httpLogger.Error("internal error while authenticating", zap.Error(err))
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
func (b *Backend) GetChannelByID(channelid string) (string, error) {
	channelName, ok := b.channelcache.Get(channelid)
	if !ok {
		resp, err := b.Client.GetChannelInformation(&helix.GetChannelInformationParams{
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
		b.channelcache.Add(channelid, channelName)
	}
	return channelName.(string), nil
}

func (b *Backend) GetUserByID(userid string) (helix.User, error) {
	username, ok := b.usercache.Get(userid)
	if !ok {
		resp, err := b.Client.GetUsers(&helix.UsersParams{
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
		b.usercache.Add(userid, username)
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
