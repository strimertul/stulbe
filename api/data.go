package api

type StatusResponse struct {
	Ok bool `json:"ok"`
}

type ResponseError struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

type AuthRequest struct {
	User    string `json:"user"`
	AuthKey string `json:"key"`
}

type AuthResponse struct {
	Ok    bool   `json:"ok"`
	User  string `json:"username"`
	Level string `json:"level"`
	Token string `json:"token"`
}

const KVKeyPrefix = "stulbe/"

type ExLoyaltyRedeem struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Channel     string `json:"channel"`
	RewardID    string `json:"reward_id"`
}

const KVExLoyaltyRedeem = "stulbe/loyalty/@redeem-rpc"
