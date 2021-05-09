package glry_db

import (
	"fmt"
	"context"
	"crypto/md5"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
)

//-------------------------------------------------------------
type GLRYuserID string
type GLRYuser struct {
	VersionInt     int64      `bson:"version"` // schema version for this model
	IDstr          GLRYuserID `bson:"_id"           json:"id"`
	CreationTimeF  float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool    bool       `bson:"deleted"`

	NameStr    string `bson:"name"    json:"name"`
	AddressStr string `bson:"address" json:"address"`

	// FIX?? - this nonce changes on every user login, should it be stored in some
	//         other DB coll/structure, separate from the user?
	NonceInt int
}

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
func AuthUserDelete(pUserID *GLRYuserID,
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
func AuthUserGetByAddress(pAddressStr string,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYuser, *gfcore.Gf_error) {


	var user *GLRYuser
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_users").FindOne(pCtx, bson.M{
			"address": pAddressStr,
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
// CREATE_ID
func AuthUserCreateID(pUsernameStr string,
	pAddressStr        string,
	pCreationTimeUNIXf float64) GLRYuserID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pUsernameStr))
	h.Write([]byte(pAddressStr))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYuserID(hexStr)
	return ID
}