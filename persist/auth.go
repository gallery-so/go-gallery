package persist

import (
	"context"
	"errors"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

const (
	loginAttemptCollName = "user_login_attempts"
	noncesCollName       = "nonces"
)

//-------------------------------------------------------------

// USER_NONCE
type UserNonce struct {
	Version int64 `bson:"version" mapstructure:"version"`

	ID           DbID    `bson:"_id"           json:"id"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"       json:"deleted"`

	Value   string `bson:"value"   json:"value"`
	UserID  DbID   `bson:"user_id" json:"user_id"`
	Address string `bson:"address"     json:"address"`
}

// USER_LOGIN_ATTEMPT
type UserLoginAttempt struct {
	Version      int64   `bson:"version"`
	ID           DbID    `bson:"_id"`
	CreationTime float64 `bson:"creation_time"`
	Deleted      bool    `bson:"deleted"       json:"deleted"`

	Address        string `bson:"address"     json:"address"`
	Signature      string `bson:"signature"`
	NonceValue     string `bson:"nonce_value"`
	UserExists     bool   `bson:"user_exists"`
	SignatureValid bool   `bson:"signature_valid"`

	ReqHostAddr string              `bson:"req_host_addr"`
	ReqHeaders  map[string][]string `bson:"req_headers"`
}

//-------------------------------------------------------------
// LOGIN_ATTEMPT
//-------------------------------------------------------------
// CREATE
func AuthUserLoginAttemptCreate(pCtx context.Context, pLoginAttempt *UserLoginAttempt,
	pRuntime *runtime.Runtime) (DbID, error) {

	mp := NewMongoStorage(0, loginAttemptCollName, pRuntime)

	return mp.Insert(pCtx, pLoginAttempt)

}

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------
// GET
func AuthNonceGet(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) (*UserNonce, error) {

	mp := NewMongoStorage(0, noncesCollName, pRuntime)

	opts := options.Find()
	opts.SetSort(bson.M{"creation_time": -1})
	opts.SetLimit(1)

	result := []*UserNonce{}
	err := mp.Find(pCtx, bson.M{"address": pAddress}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errors.New("no nonce found")
	}

	return result[0], nil
}

//-------------------------------------------------------------
// CREATE
func AuthNonceCreate(pCtx context.Context, pNonce *UserNonce,
	pRuntime *runtime.Runtime) (DbID, error) {

	mp := NewMongoStorage(0, noncesCollName, pRuntime)

	return mp.Insert(pCtx, pNonce)

}
