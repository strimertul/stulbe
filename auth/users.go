package auth

import (
	"errors"
	"time"

	"github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrInvalidKey       = errors.New("invalid auth")
	ErrTokenParseFailed = errors.New("couldnt parse jwt")
	ErrTokenExpired     = errors.New("token expired")
)

type UserList map[string]User

const usersKey = "stulbe-auth/users"

type User struct {
	User    string    `json:"user"`
	AuthKey []byte    `json:"authkey"`
	Level   UserLevel `json:"level"`
}

type UserClaims struct {
	User  string    `json:"user"`
	Level UserLevel `json:"level"`
	jwt.StandardClaims
}

type UserLevel string

const (
	ULAdmin    UserLevel = "admin"
	ULStreamer UserLevel = "streamer"
)

func (db *Storage) saveUsers() error {
	return db.db.PutJSON(usersKey, db.users)
}

func (db *Storage) AddUser(user string, key string, level UserLevel) error {
	// Hash password
	password, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	db.users[user] = User{
		User:    user,
		AuthKey: password,
		Level:   level,
	}
	return db.saveUsers()
}

func (db *Storage) DeleteUser(user string) error {
	delete(db.users, user)
	return db.saveUsers()
}

func (db *Storage) GetUser(username string) (User, bool) {
	user, ok := db.users[username]
	return user, ok
}

func (db *Storage) CountUsers() int {
	return len(db.users)
}

func (db *Storage) Authenticate(username string, key string, claims jwt.StandardClaims) (UserClaims, string, error) {
	user, ok := db.GetUser(username)
	if !ok {
		return UserClaims{}, "", ErrUserNotFound
	}

	err := bcrypt.CompareHashAndPassword(user.AuthKey, []byte(key))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return UserClaims{}, "", ErrInvalidKey
		}
		return UserClaims{}, "", err
	}

	userClaims := UserClaims{
		User:           user.User,
		Level:          user.Level,
		StandardClaims: claims,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, userClaims)

	signedToken, err := token.SignedString(db.secretKey)

	return userClaims, signedToken, err
}

func (db *Storage) Verify(token string) (*UserClaims, error) {
	tk, err := jwt.ParseWithClaims(
		token,
		&UserClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return db.secretKey, nil
		},
	)

	if err != nil {
		return nil, err
	}

	claims, ok := tk.Claims.(*UserClaims)
	if !ok {
		return nil, ErrTokenParseFailed
	}

	if claims.ExpiresAt < time.Now().Unix() {
		return nil, ErrTokenExpired
	}

	return claims, nil
}
