package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
	// "github.com/davecgh/go-spew/spew"
)

// ReqHeaders is a type that holds the headers for a request
type ReqHeaders map[string][]string

// UserNonce represents a short lived nonce that holds a value to be signed
// by a user cryptographically to prove they are the owner of a given address.
type UserNonce struct {
	Version      NullInt32  `json:"version"`
	ID           DBID       `json:"id"`
	CreationTime time.Time  `json:"created_at"`
	Deleted      NullBool   `json:"-"`
	LastUpdated  time.Time  `json:"last_updated"`
	Value        NullString `json:"value"`
	Address      Address    `json:"address"`
	Chain        Chain      `json:"chain"`
	L1Chain      Chain      `json:"l1_chain"`
}

// UserLoginAttempt represents a single attempt for a user to login despite the success
// of the login. Can be used in debugging and logging purposes.
type UserLoginAttempt struct {
	Version        NullInt32  `json:"version"`
	ID             DBID       `json:"id"`
	CreationTime   time.Time  `json:"created_at"`
	Deleted        NullBool   `json:"-"`
	Address        Wallet     `json:"address"`
	Signature      NullString `json:"signature"`
	NonceValue     NullString `json:"nonce_value"`
	UserExists     NullBool   `json:"user_exists"`
	SignatureValid NullBool   `json:"signature_valid"`
	ReqHostAddr    NullString `json:"req_host_addr"`
	ReqHeaders     ReqHeaders `json:"req_headers"`
}

// CreateLoginAttemptInput is a type that holds the input for creating a login attempt
type CreateLoginAttemptInput struct {
	Address        Wallet     `json:"address"`
	Signature      string     `json:"signature"`
	NonceValue     string     `json:"nonce_value"`
	UserExists     bool       `json:"user_exists"`
	SignatureValid bool       `json:"signature_valid"`
	ReqHostAddr    string     `json:"req_host_addr"`
	ReqHeaders     ReqHeaders `json:"req_headers"`
}

// Scan implements the sql.Scanner interface for the ReqHeaders type
func (h *ReqHeaders) Scan(src interface{}) error {
	if src == nil {
		*h = make(ReqHeaders)
		return nil
	}
	return json.Unmarshal(src.([]byte), h)
}

// Value implements the driver.Valuer interface for the ReqHeaders type
func (h ReqHeaders) Value() (driver.Value, error) {
	return json.Marshal(h)
}

// ErrNonceNotFoundForAddress is returned when no nonce is found for a given address
type ErrNonceNotFoundForAddress struct {
	L1ChainAddress L1ChainAddress
}

func (e ErrNonceNotFoundForAddress) Error() string {
	return fmt.Sprintf("no nonce found for address: %v", e.L1ChainAddress)
}
