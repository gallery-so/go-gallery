package glry_db

import (
	"fmt"
	"context"
	"crypto/md5"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/bson"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
)

//-------------------------------------------------------------
type GLRYcollID string
type GLRYcollection struct {
	VersionInt     int64      `bson:"version"       json:"version"` // schema version for this model
	IDstr          GLRYcollID `bson:"_id"           json:"id"`
	CreationTimeF  float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool    bool       `bson:"deleted"`

	NameStr        string     `bson:"name"          json:"name"`
	DescriptionStr string     `bson:"description"   json:"description"`
	OwnerUserIDstr string     `bson:"owner_user_id" json:"owner_user_id"`
	NFTsLst        []string   `bson:"nfts"          json:"nfts"`
}

//-------------------------------------------------------------
func CollCreate(pColl *GLRYcollection,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	collNameStr := "glry_collections"
	gErr := gfcore.Mongo__insert(pColl,
		collNameStr,
		map[string]interface{}{
			"coll_name":       pColl.NameStr,
			"caller_err_msg": "failed to insert a new GLRYcollection into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
func CollGetByID(pIDstr string,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollection, *gfcore.Gf_error) {

	var coll *GLRYcollection
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_collections").FindOne(pCtx, bson.M{
			"_id":     pIDstr,
			"deleted": false,
		}).Decode(&coll)
	
	if err != nil {
		gf_err := gfcore.Mongo__handle_error("failed to query GLRYcollection by ID",
			"mongodb_find_error",
			map[string]interface{}{"id": pIDstr,},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gf_err
	}

	return coll, nil
}

//-------------------------------------------------------------
// CREATE_ID
func CollCreateID(pNameStr string,
	pOwnerUserIDstr    string,
	pCreationTimeUNIXf float64) GLRYcollID {
	
	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pNameStr))
	h.Write([]byte(pOwnerUserIDstr))
	sum    := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID     := GLRYcollID(hexStr)
	return ID
}