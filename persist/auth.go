package persist

import (
	"context"
	"time"
	// "github.com/davecgh/go-spew/spew"
)

// UserNonce represents a short lived nonce that holds a value to be signed
// by a user cryptographically to prove they are the owner of a given address.
type UserNonce struct {
	Version int64 `bson:"version" mapstructure:"version"`

	ID           DBID      `bson:"_id"           json:"id"`
	CreationTime time.Time `bson:"created_at" json:"created_at"`
	Deleted      bool      `bson:"deleted"       json:"-"`
	LastUpdated  time.Time `bson:"last_updated" json:"last_updated"`

	Value   string `bson:"value"   json:"value"`
	Address string `bson:"address"     json:"address"`
}

// UserLoginAttempt represents a single attempt for a user to login despite the success
// of the login. Can be used in debugging and logging purposes.
type UserLoginAttempt struct {
	Version      int64     `bson:"version"`
	ID           DBID      `bson:"_id"`
	CreationTime time.Time `bson:"created_at"`
	Deleted      bool      `bson:"deleted"       json:"-"`

	Address        string `bson:"address"     json:"address"`
	Signature      string `bson:"signature"`
	NonceValue     string `bson:"nonce_value"`
	UserExists     bool   `bson:"user_exists"`
	SignatureValid bool   `bson:"signature_valid"`

	ReqHostAddr string              `bson:"req_host_addr"`
	ReqHeaders  map[string][]string `bson:"req_headers"`
}

// NonceRepository is the interface for interacting with the auth nonce persistence layer
type NonceRepository interface {
	Get(context.Context, string) (*UserNonce, error)
	Create(context.Context, *UserNonce) error
}

// LoginAttemptRepository is the interface for interacting with the auth login attempt persistence layer
type LoginAttemptRepository interface {
	Create(context.Context, *UserLoginAttempt) (DBID, error)
}
