package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	// "github.com/davecgh/go-spew/spew"
)

// ReqHeaders is a type that holds the headers for a request
type ReqHeaders map[string][]string

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

	ReqHostAddr string     `bson:"req_host_addr"`
	ReqHeaders  ReqHeaders `bson:"req_headers"`
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

// Scan implements the sql.Scanner interface for the ReqHeaders type
func (h *ReqHeaders) Scan(src interface{}) error {
	return json.Unmarshal(src.([]byte), h)
}

// Value implements the driver.Valuer interface for the ReqHeaders type
func (h ReqHeaders) Value() (driver.Value, error) {
	return json.Marshal(h)
}

// ErrNonceNotFoundForAddress is returned when no nonce is found for a given address
type ErrNonceNotFoundForAddress struct {
	Address Address
}

func (e ErrNonceNotFoundForAddress) Error() string {
	return fmt.Sprintf("no nonce found for address: %v", e.Address)
}
