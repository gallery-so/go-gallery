package persist

import (
	"context"
	"fmt"
	// "github.com/davecgh/go-spew/spew"
)

// UserNonce represents a short lived nonce that holds a value to be signed
// by a user cryptographically to prove they are the owner of a given address.
type UserNonce struct {
	Version int64 `bson:"version" mapstructure:"version"`

	ID           DBID            `bson:"_id"           json:"id"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted"       json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	Value   string  `bson:"value"   json:"value"`
	Address Address `bson:"address"     json:"address"`
}

// UserLoginAttempt represents a single attempt for a user to login despite the success
// of the login. Can be used in debugging and logging purposes.
type UserLoginAttempt struct {
	Version      int64        `bson:"version"`
	ID           DBID         `bson:"_id"`
	CreationTime CreationTime `bson:"created_at"`
	Deleted      bool         `bson:"deleted"       json:"-"`

	Address        Address `bson:"address"     json:"address"`
	Signature      string  `bson:"signature"`
	NonceValue     string  `bson:"nonce_value"`
	UserExists     bool    `bson:"user_exists"`
	SignatureValid bool    `bson:"signature_valid"`

	ReqHostAddr string              `bson:"req_host_addr"`
	ReqHeaders  map[string][]string `bson:"req_headers"`
}

// NonceRepository is the interface for interacting with the auth nonce persistence layer
type NonceRepository interface {
	Get(context.Context, Address) (UserNonce, error)
	Create(context.Context, UserNonce) error
}

// LoginAttemptRepository is the interface for interacting with the auth login attempt persistence layer
type LoginAttemptRepository interface {
	Create(context.Context, UserLoginAttempt) (DBID, error)
}

// ErrNonceNotFoundForAddress is returned when no nonce is found for a given address
type ErrNonceNotFoundForAddress struct {
	Address Address
}

func (e ErrNonceNotFoundForAddress) Error() string {
	return fmt.Sprintf("no nonce found for address: %v", e.Address)
}
