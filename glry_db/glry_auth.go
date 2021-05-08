package glry_db

import (
	"fmt"
	"context"
	"crypto/md5"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/bson"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
type GLRYuserID string
type GRLYuser struct {
	VersionInt     int64   `bson:"version"` // schema version for this model
	IDstr          string  `bson:"_id"           json:"id"`
	CreationTimeF  float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool    bool    `bson:"deleted"`

	UsernameStr string `bson:"username" json:"username"`
	AddressStr  string `bson:"address"  json:"address"`

	// FIX?? - this nonce changes on every user login, should it be stored in some
	//         other DB coll/structure, separate from the user?
	NonceInt int
}

//-------------------------------------------------------------
func AuthUserCreate(pUser *GRLYuser,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) (*gfcore.Gf_error) {


	return nil
}



//-------------------------------------------------------------
func AuthUserGetByAddress(pAddressStr string,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) (*GRLYuser, *gfcore.Gf_error) {



	

	

	var user *GRLYuser
	err := pRuntimeSys.Mongo_db.Collection("glry_users").FindOne(pCtx, bson.M{
			"address": pAddressStr,
			"deleted": false,
		}).Decode(&user)

	if err != nil {
		gf_err := gfcore.Mongo__handle_error("failed to get user GRLYuser by Address",
			"mongodb_find_error",
			map[string]interface{}{"address": pAddressStr,},
			err, "glry_db", pRuntimeSys)
		return nil, gf_err
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