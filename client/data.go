package client

import (
	"errors"

	"github.com/sirupsen/logrus"
)

type ClientOptions struct {
	Endpoint string
	Username string
	AuthKey  string

	Logger logrus.FieldLogger
}

var (
	ErrNotAuthenticated     = errors.New("not authenticated")
	ErrSubscriptionNotFound = errors.New("subscription not found")
)
