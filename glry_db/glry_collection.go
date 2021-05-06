package glry_db

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
type GLRYcollection struct {
	VersionInt     int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr          string  `bson:"_id"           json:"id"`
	CreationTimeF  float64 `bson:"creation_time" json:"creation_time"`
	
	NameStr        string  `bson:"name"          json:"name"`
	DescriptionStr string  `bson:"description"   json:"description"`
	OwnerStr       string  `bson:"owner"         json:"owner"`
	DeletedBool    bool    `bson:"deleted"`

	NFTsLst []string `bson:"nfts" json:"nfts"`
}

//-------------------------------------------------------------
func CollCreate(pColl *GLRYcollection,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) *gfcore.Gf_error {

	collNameStr := "glry_collections"
	gErr := gfcore.Mongo__insert(pColl,
		collNameStr,
		map[string]interface{}{
			"nft_name":       pColl.NameStr,
			"caller_err_msg": "failed to insert a new Collection into the DB",
		},
		pCtx,
		pRuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
func CollGetByID(pIDstr string,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) (*GLRYcollection, *gfcore.Gf_error) {


	var coll *GLRYcollection
	err := pRuntimeSys.Mongo_db.Collection("glry_collections").FindOne(pCtx, bson.M{
			"_id":     pIDstr,
			"deleted": false,
		}).Decode(&coll)

	if err != nil {
		gf_err := gfcore.Mongo__handle_error("failed to query GLRYcollection by ID",
			"mongodb_find_error",
			map[string]interface{}{"id": pIDstr,},
			err, "glry_db", pRuntimeSys)
		return nil, gf_err
	}

	return coll, nil
}
