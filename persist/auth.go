package persist

import (
	"context"
	"errors"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

const (
	loginAttemptCollName = "glry_users_login_attempts"
	noncesCollName       = "glry_nonces"
)

//-------------------------------------------------------------

// USER_NONCE
type UserNonce struct {
	VersionInt int64 `bson:"version" mapstructure:"version"`

	// nonces are shortlived, and not something to be persisted across DB's
	// other than mongo. so use mongo-native ID generation
	ID            primitive.ObjectID `bson:"_id"           mapstructure:"_id"`
	CreationTimeF float64            `bson:"creation_time" mapstructure:"creation_time"`
	DeletedBool   bool               `bson:"deleted"       mapstructure:"deleted"`

	ValueStr   string `bson:"value"   mapstructure:"value"`
	UserIDstr  DbId   `bson:"user_id" mapstructure:"user_id"`
	AddressStr string `bson:"address"     json:"address"`
}

// USER_LOGIN_ATTEMPT
type UserLoginAttempt struct {
	VersionInt    int64   `bson:"version"`
	ID            DbId    `bson:"_id"`
	CreationTimeF float64 `bson:"creation_time"`

	AddressStr         string `bson:"address"     json:"address"`
	SignatureStr       string `bson:"signature"`
	NonceValueStr      string `bson:"nonce_value"`
	UserExistsBool     bool   `bson:"user_exists"`
	SignatureValidBool bool   `bson:"signature_valid"`

	ReqHostAddrStr string              `bson:"req_host_addr"`
	ReqHeaders     map[string][]string `bson:"req_headers"`
}

//-------------------------------------------------------------
// LOGIN_ATTEMPT
//-------------------------------------------------------------
// CREATE
func AuthUserLoginAttemptCreate(pLoginAttempt *UserLoginAttempt,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, loginAttemptCollName, pRuntime)

	return mp.Insert(pCtx, pLoginAttempt)

}

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------
// GET
func AuthNonceGet(pAddress string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*UserNonce, error) {

	mp := NewMongoStorage(0, noncesCollName, pRuntime)

	opts := options.Find()
	opts.SetSort(map[string]interface{}{"created_at": -1})
	opts.SetLimit(1)

	result := []*UserNonce{}
	err := mp.Find(pCtx, bson.M{"addresses": bson.M{"$in": []string{pAddress}}}, result, opts)

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
func AuthNonceCreate(pNonce *UserNonce,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, noncesCollName, pRuntime)

	return mp.Insert(pCtx, pNonce)

}
