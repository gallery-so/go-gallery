package glry_db

import (
	"fmt"
	"context"
	"crypto/md5"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
)

//-------------------------------------------------------------
type GLRYuserID         string
type GLRYuserAddress    string
type GLRYloginAttemptID string

type GLRYuser struct {
	VersionInt     int64      `bson:"version"` // schema version for this model
	IDstr          GLRYuserID `bson:"_id"           json:"id"`
	CreationTimeF  float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool    bool       `bson:"deleted"`

	NameStr      string            `bson:"name"     json:"name"`
	AddressesLst []GLRYuserAddress `bson:"addresses json:"addresses"` // IMPORTANT!! - users can have multiple addresses associated with their account

	// LAST_SEEN - last time user logged or out? or some other metric?
	// FINISH!!  - nothing is setting this yet.
	LastSeenTimeF float64 
}

type GLRYuserNonce struct {
	VersionInt int64 `bson:"version"` // schema version for this model

	// nonces are shortlived, and not something to be persisted across DB's
	// other than mongo. so use mongo-native ID generation
	ID             primitive.ObjectID `bson:"_id"`
	CreationTimeF  float64            `bson:"creation_time"`
	DeletedBool    bool               `bson:"deleted"`

	ValueStr   string          `bson:"value"`
	UserIDstr  GLRYuserID      `bson:"user_id"`
	AddressStr GLRYuserAddress `bson:"address"`
}

type GLRYuserLoginAttempt struct {
	VersionInt     int64              `bson:"version"`
	ID             GLRYloginAttemptID `bson:"_id"`
	CreationTimeF  float64            `bson:"creation_time"`

	AddressStr    GLRYuserAddress `bson:"address"`
	SignatureStr  string          `bson:"signature"`
	NonceValueStr string          `bson:"nonce_value"`
	UsernameStr   string          `bson:"username"`
	ValidBool     bool            `bson:"valid"`

	ReqHostAddrStr string              `bson:"req_host_addr"`
	ReqHeaders     map[string][]string `bson:"req_headers"`
}

// FINISH!! - persist this in the DB
// USER_UPDATE - every update to user records is persisted
//               as an event
type GLRYuserUpdate struct {
	CreationTimeF float64 `bson:"creation_time"`
	NameNewStr    string  `bson:"name_new"`
}

//-------------------------------------------------------------
// LOGIN_ATTEMPT
//-------------------------------------------------------------
func AuthUserLoginAttempt(pLoginAttempt *GLRYuserLoginAttempt,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	collNameStr := "glry_users_login_attempts"
	gErr := gfcore.Mongo__insert(pLoginAttempt,
		collNameStr,
		map[string]interface{}{
			"address":        pLoginAttempt.AddressStr,
			"username":       pLoginAttempt.UsernameStr,
			"caller_err_msg": "failed to insert a new GLRYuserLoginAttempt into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// USER
//-------------------------------------------------------------
// USER_CREATE
func AuthUserCreate(pUser *GLRYuser,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {


	collNameStr := "glry_users"
	gErr := gfcore.Mongo__insert(pUser,
		collNameStr,
		map[string]interface{}{
			"user_name":       pUser.NameStr,
			"caller_err_msg": "failed to insert a new GLRYuser into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// USER_DELETE
func AuthUserDelete(pUserID GLRYuserID,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	
	_, err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_users").UpdateMany(pCtx, bson.M{
			"_id":     pUserID,
			"deleted": false,
		},

		// mark user as deleted
		bson.M{"$set": bson.M{
				"deleted": true,
			},
		})

	if err != nil {
		gErr := gfcore.Mongo__handle_error("failed to update GLRYuser as deleted by ID",
			"mongodb_update_error",
			map[string]interface{}{"user_id": pUserID,},
			err, "glry_db", pRuntime.RuntimeSys)
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// USER_GET_BY_ADDRESS
func AuthUserGetByAddress(pAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuser, *gfcore.Gf_error) {


	var user *GLRYuser
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_users").FindOne(pCtx, bson.M{
			"addresses": bson.M{"$in": bson.A{pAddressStr, }},
			"deleted": false,
		}).Decode(&user)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		gErr := gfcore.Mongo__handle_error("failed to get user GLRYuser by Address",
			"mongodb_find_error",
			map[string]interface{}{"address": pAddressStr,},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gErr
	}

	return user, nil
}

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------
// NONCE_GET
func AuthNonceGet(pUserAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuserNonce, *gfcore.Gf_error) {

	var nonce *GLRYuserNonce
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_user_nonces").FindOne(pCtx, bson.M{
			"address": pUserAddressStr,
			"deleted": false,
		}).Decode(&nonce)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		gErr := gfcore.Mongo__handle_error("failed to get user GLRYuserNonce by Address",
			"mongodb_find_error",
			map[string]interface{}{"address": pUserAddressStr,},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gErr
	}

	return nonce, nil
}

//-------------------------------------------------------------
// NONCE_CREATE
func AuthNonceCreate(pNonce *GLRYuserNonce,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {


	collNameStr := "glry_user_nonces"
	gErr := gfcore.Mongo__insert(pNonce,
		collNameStr,
		map[string]interface{}{
			"nonce_address":  pNonce.AddressStr,
			"caller_err_msg": "failed to insert a new GLRYuser into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// VAR
//-------------------------------------------------------------
// CREATE_ID
func AuthUserCreateID(pUsernameStr string,
	pAddressStr        GLRYuserAddress,
	pCreationTimeUNIXf float64) GLRYuserID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pUsernameStr))
	h.Write([]byte(string(pAddressStr)))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYuserID(hexStr)
	return ID
}

// CREATE_LOGIN_ATTEMPT_ID
func AuthUserLoginAttemptCreateID(pUsernameStr string,
	pAddressStr        GLRYuserAddress,
	pSignatureStr      string,
	pCreationTimeUNIXf float64) GLRYloginAttemptID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pUsernameStr))
	h.Write([]byte(string(pAddressStr)))
	h.Write([]byte(string(pSignatureStr)))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYloginAttemptID(hexStr)
	return ID
}