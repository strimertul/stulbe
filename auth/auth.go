package auth

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/sirupsen/logrus"
	"github.com/strimertul/stulbe/database"
)

type Storage struct {
	db        *database.DB
	users     UserList
	secretKey []byte
	logger    logrus.FieldLogger
}

type Options struct {
	Logger              logrus.FieldLogger
	ForgeGenerateSecret bool
}

const secretKey = "stulbe-auth/secret"

func Init(db *database.DB, options Options) (*Storage, error) {
	store := &Storage{
		db:        db,
		users:     nil,
		logger:    options.Logger,
		secretKey: nil,
	}

	// Get user/session lists from DB, if we can
	err := db.GetJSON(usersKey, &store.users)
	if err != nil {
		if err == database.ErrKeyNotFound {
			store.users = make(UserList)
			store.logger.Warn("user storage not found, initializing new one")
		} else {
			return nil, err
		}
	}

	if options.ForgeGenerateSecret {
		err = store.generateSecret()
		if err != nil {
			return nil, err
		}
	} else {
		store.secretKey, err = db.GetKey(secretKey)
		if err != nil {
			if err == database.ErrKeyNotFound {
				// Generate random key
				err = store.generateSecret()
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}

	return store, nil
}

func (db *Storage) generateSecret() error {
	db.logger.Warn("no secret key found, generating one")
	db.secretKey = make([]byte, 32)
	_, err := rand.Read(db.secretKey)
	if err != nil {
		return err
	}
	err = db.db.PutKey(secretKey, db.secretKey)
	if err != nil {
		return err
	}
	db.logger.WithField("key", hex.EncodeToString(db.secretKey)).Info("generated secret key")
	return nil
}
