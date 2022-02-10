package auth

import (
	"crypto/rand"
	"encoding/hex"

	kv "github.com/strimertul/kilovolt/v8"

	"go.uber.org/zap"

	"github.com/strimertul/stulbe/database"
)

type Storage struct {
	db        *database.DBModule
	users     UserList
	secretKey []byte
	logger    *zap.Logger
}

type Options struct {
	Logger              *zap.Logger
	ForgeGenerateSecret bool
}

const secretKey = "stulbe-auth/secret"

func Init(db *database.DBModule, options Options) (*Storage, error) {
	store := &Storage{
		db:        db,
		users:     nil,
		logger:    options.Logger,
		secretKey: nil,
	}

	// Get user/session lists from DB, if we can
	err := db.GetJSON(usersKey, &store.users)
	if err != nil {
		if err == kv.ErrorKeyNotFound {
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
		hexkey, err := db.GetKey(secretKey)
		if err != nil {
			if err == kv.ErrorKeyNotFound {
				// Generate random key
				err = store.generateSecret()
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		store.secretKey, err = hex.DecodeString(hexkey)
		if err != nil {
			return nil, err
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
	hexkey := hex.EncodeToString(db.secretKey)
	err = db.db.PutKey(secretKey, hexkey)
	if err != nil {
		return err
	}
	db.logger.Info("generated secret key", zap.String("key", hexkey))
	return nil
}
