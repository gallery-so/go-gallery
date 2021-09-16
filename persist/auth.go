package persist

import (
	"context"
	"errors"
	"strings"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

const (
	loginAttemptCollName = "user_login_attempts"
	noncesCollName       = "nonces"
)

// UserNonce represents a short lived nonce that holds a value to be signed
// by a user cryptographically to prove they are the owner of a given address.
type UserNonce struct {
	Version int64 `bson:"version" mapstructure:"version"`

	ID           DBID               `bson:"_id"           json:"id"`
	CreationTime primitive.DateTime `bson:"created_at" json:"created_at"`
	Deleted      bool               `bson:"deleted"       json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Value   string `bson:"value"   json:"value"`
	UserID  DBID   `bson:"user_id" json:"user_id"`
	Address string `bson:"address"     json:"address"`
}

// UserLoginAttempt represents a single attempt for a user to login despite the success
// of the login. Can be used in debugging and logging purposes.
type UserLoginAttempt struct {
	Version      int64              `bson:"version"`
	ID           DBID               `bson:"_id"`
	CreationTime primitive.DateTime `bson:"created_at"`
	Deleted      bool               `bson:"deleted"       json:"-"`

	Address        string `bson:"address"     json:"address"`
	Signature      string `bson:"signature"`
	NonceValue     string `bson:"nonce_value"`
	UserExists     bool   `bson:"user_exists"`
	SignatureValid bool   `bson:"signature_valid"`

	ReqHostAddr string              `bson:"req_host_addr"`
	ReqHeaders  map[string][]string `bson:"req_headers"`
}

// AuthUserLoginAttemptCreate inserts a single login attempt into the database and will return the ID of the inserted attempt
func AuthUserLoginAttemptCreate(pCtx context.Context, pLoginAttempt *UserLoginAttempt,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, loginAttemptCollName, pRuntime)

	return mp.insert(pCtx, pLoginAttempt)

}

// AuthNonceGet returns the most recent nonce for a given address
func AuthNonceGet(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (*UserNonce, error) {

	mp := newStorage(0, noncesCollName, pRuntime)

	opts := options.Find()
	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*UserNonce{}
	err := mp.find(pCtx, bson.M{"address": strings.ToLower(pAddress)}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errors.New("no nonce found")
	}

	return result[0], nil
}

// AuthNonceCreate inserts a new nonce into the database and will return the ID of the inserted nonce
func AuthNonceCreate(pCtx context.Context, pNonce *UserNonce,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, noncesCollName, pRuntime)

	return mp.insert(pCtx, pNonce)

}
