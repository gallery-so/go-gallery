package glry_db

import (
	"fmt"
	"context"
	"crypto/md5"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"github.com/mitchellh/mapstructure"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
type GLRYuserID         string
type GLRYuserAddress    string
type GLRYloginAttemptID string
type GLRYuserJWTkeyID   string

// USER
type GLRYuser struct {
	VersionInt     int64      `bson:"version"` // schema version for this model
	IDstr          GLRYuserID `bson:"_id"           json:"id"`
	CreationTimeF  float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool    bool       `bson:"deleted"`

	UserNameStr    string            `bson:"name"         json:"name"`         // mutable
	AddressesLst   []GLRYuserAddress `bson:"addresses     json:"addresses"`    // IMPORTANT!! - users can have multiple addresses associated with their account
	DescriptionStr string            `bson:"description"  json:"description"`

	// LAST_SEEN - last time user logged or out? or some other metric?
	// FINISH!!  - nothing is setting this yet.
	LastSeenTimeF float64 
}

// USER_NONCE
type GLRYuserNonce struct {
	VersionInt int64 `bson:"version" mapstructure:"version"`

	// nonces are shortlived, and not something to be persisted across DB's
	// other than mongo. so use mongo-native ID generation
	ID             primitive.ObjectID `bson:"_id"           mapstructure:"_id"`
	CreationTimeF  float64            `bson:"creation_time" mapstructure:"creation_time"`
	DeletedBool    bool               `bson:"deleted"       mapstructure:"deleted"`

	ValueStr   string          `bson:"value"   mapstructure:"value"`
	UserIDstr  GLRYuserID      `bson:"user_id" mapstructure:"user_id"`
	AddressStr GLRYuserAddress `bson:"address" mapstructure:"address"`
}

// USER_JWT_KEY - is unique per user, and stored in the DB for now. 
type GLRYuserJWTkey struct {
	VersionInt    int64            `bson:"version"       mapstructure:"version"`
	ID            GLRYuserJWTkeyID `bson:"_id"           mapstructure:"_id"`
	CreationTimeF float64          `bson:"creation_time" mapstructure:"creation_time"`
	DeletedBool   bool             `bson:"deleted"       mapstructure:"deleted"`

	ValueStr   string          `bson:"value"   mapstructure:"value"`
	AddressStr GLRYuserAddress `bson:"address" mapstructure:"address"`
}

// USER_LOGIN_ATTEMPT
type GLRYuserLoginAttempt struct {
	VersionInt     int64              `bson:"version"`
	ID             GLRYloginAttemptID `bson:"_id"`
	CreationTimeF  float64            `bson:"creation_time"`

	AddressStr    GLRYuserAddress `bson:"address"`
	SignatureStr  string          `bson:"signature"`
	NonceValueStr string          `bson:"nonce_value"`
	UserExistsBool     bool       `bson:"user_exists"`
	SignatureValidBool bool       `bson:"signature_valid"`

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
// JWT
//-------------------------------------------------------------
// GET
func AuthUserJWTkeyGet(pUserAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuserJWTkey, *gf_core.Gf_error) {




	record, gErr := gf_core.MongoFindLatest(bson.M{
			"address": pUserAddressStr,
			"deleted": false,
		},
		"creation_time", // p_time_field_name_str
		map[string]interface{}{
			"address":            pUserAddressStr,
			"caller_err_msg_str": "failed to get JWT key from DB",
		},
		pRuntime.DB.MongoDB.Collection("glry_users_jwt_keys"),
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return nil, gErr
	}



	var JWTkey GLRYuserJWTkey
	err := mapstructure.Decode(record, &JWTkey)
	if err != nil {
		gErr := gf_core.Error__create("failed to load DB result of GLRYuserJWTkey into its struct",
			"mapstruct__decode",
			map[string]interface{}{
				"address": pUserAddressStr,
			},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gErr
	}

	// spew.Dump(JWTkey)

	return &JWTkey, nil
}

//-------------------------------------------------------------
// CREATE

func AuthUserJWTkeyCreate(pJWTkey *GLRYuserJWTkey,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {

	collNameStr := "glry_users_jwt_keys"
	gErr := gf_core.Mongo__insert(pJWTkey,
		collNameStr,
		map[string]interface{}{
			"address":        pJWTkey.AddressStr,
			"caller_err_msg": "failed to insert a new GLRYuserJWTkey into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// LOGIN_ATTEMPT
//-------------------------------------------------------------
// CREATE
func AuthUserLoginAttemptCreate(pLoginAttempt *GLRYuserLoginAttempt,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {
	
	
	collNameStr := "glry_users_login_attempts"
	gErr := gf_core.Mongo__insert(pLoginAttempt,
		collNameStr,
		map[string]interface{}{
			"address":        pLoginAttempt.AddressStr,
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
// UPDATE
func AuthUserUpdate(pAddressStr GLRYuserAddress,
	pUserNameStr        string,
	pUserDescriptionStr string,
	pCtx                context.Context,
	pRuntime            *glry_core.Runtime) *gf_core.Gf_error {

	
	//------------------
	fieldsToUpdate := bson.M{}
	if pUserNameStr != "" {
		fieldsToUpdate["username"] = pUserNameStr
	}

	if pUserDescriptionStr != "" {
		fieldsToUpdate["description"] = pUserNameStr
	}

	//------------------
	// UPDATE
	_, err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_users").UpdateMany(pCtx, bson.M{
			"address": pAddressStr,
			"deleted": false,
		},
		bson.M{"$set": fieldsToUpdate, })

	if err != nil {
		gErr := gf_core.Mongo__handle_error("failed to update GLRYuser",
			"mongodb_update_error",
			map[string]interface{}{"address": pAddressStr,},
			err, "glry_db", pRuntime.RuntimeSys)
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// EXISTS_BY_ADDRESS
func AuthUserExistsByAddr(pAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (bool, *gf_core.Gf_error) {

	countInt, gErr := gf_core.MongoCount(bson.M{
			"address": pAddressStr,
			"deleted": false,
		},
		map[string]interface{}{
			"address":        pAddressStr,
			"caller_err_msg": "failed to check if user exists by address in the DB",
		},
		pRuntime.RuntimeSys.Mongo_db.Collection("glry_users"),
		pCtx,
		pRuntime.RuntimeSys)
	
	if gErr != nil {
		return false, gErr
	}

	if countInt > 0 {
		return true, nil
	}
	return false, nil
}

//-------------------------------------------------------------
// CREATE
func AuthUserCreate(pUser *GLRYuser,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {


	collNameStr := "glry_users"
	gErr := gf_core.Mongo__insert(pUser,
		collNameStr,
		map[string]interface{}{
			"user_name":       pUser.UserNameStr,
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
// DELETE
func AuthUserDelete(pUserID GLRYuserID,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {

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
		gErr := gf_core.Mongo__handle_error("failed to update GLRYuser as deleted by ID",
			"mongodb_update_error",
			map[string]interface{}{"user_id": pUserID,},
			err, "glry_db", pRuntime.RuntimeSys)
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// GET_BY_ADDRESS
func AuthUserGetByAddress(pAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuser, *gf_core.Gf_error) {


	var user *GLRYuser
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_users").FindOne(pCtx, bson.M{
			"addresses": bson.M{"$in": bson.A{pAddressStr, }},
			"deleted": false,
		}).Decode(&user)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	
	if err != nil {
		gErr := gf_core.Mongo__handle_error("failed to get user GLRYuser by Address",
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
// GET
func AuthNonceGet(pUserAddressStr GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuserNonce, *gf_core.Gf_error) {




	record, gErr := gf_core.MongoFindLatest(bson.M{
			"address": pUserAddressStr,
			"deleted": false,
		},
		"creation_time", // p_time_field_name_str
		map[string]interface{}{
			"address":            pUserAddressStr,
			"caller_err_msg_str": "failed to get user GLRYuserNonce by Address",
		},
		pRuntime.DB.MongoDB.Collection("glry_user_nonces"),
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return nil, gErr
	}

	// NONCE_NOT_FOUND
	if record == nil {
		return nil, nil
	}

	// spew.Dump(record)
	
	var nonce GLRYuserNonce
	err := mapstructure.Decode(record, &nonce)
	if err != nil {
		gErr := gf_core.Error__create("failed to load DB result of GLRYuserNonce into its struct",
			"mapstruct__decode",
			map[string]interface{}{
				"address": pUserAddressStr,
			},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gErr
	}

	return &nonce, nil
}

//-------------------------------------------------------------
// CREATE
func AuthNonceCreate(pNonce *GLRYuserNonce,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {

	collNameStr := "glry_user_nonces"
	gErr := gf_core.Mongo__insert(pNonce,
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
func AuthUserCreateID(pAddressStr GLRYuserAddress,
	pCreationTimeUNIXf float64) GLRYuserID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(string(pAddressStr)))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYuserID(hexStr)
	return ID
}

//-------------------------------------------------------------
// CREATE_LOGIN_ATTEMPT_ID
func AuthUserLoginAttemptCreateID(pAddressStr GLRYuserAddress,
	pSignatureStr      string,
	pCreationTimeUNIXf float64) GLRYloginAttemptID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(string(pAddressStr)))
	h.Write([]byte(string(pSignatureStr)))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYloginAttemptID(hexStr)
	return ID
}

//-------------------------------------------------------------
// CREATE_JWT_KEY
func AuthUserJWTkeyCreateID(pAddressStr GLRYuserAddress,
	pJWTkeyStr         string,
	pCreationTimeUNIXf float64) GLRYuserJWTkeyID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(string(pAddressStr)))
	h.Write([]byte(string(pJWTkeyStr)))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYuserJWTkeyID(hexStr)
	return ID
}