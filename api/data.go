package api

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
