package glry_db

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//-------------------------------------------------------------
type GLRYcollID string
type GLRYcollection struct {
	VersionInt    int64      `bson:"version"       json:"version"` // schema version for this model
	IDstr         GLRYcollID `bson:"_id"           json:"id"`
	CreationTimeF float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool       `bson:"deleted"`

	NameStr           string   `bson:"name,omitempty"          json:"name"`
	CollectorsNoteStr string   `bson:"collectors_note,omitempty"   json:"collectors_note"`
	OwnerUserIDstr    string   `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	NFTsLst           []string `bson:"nfts,omitempty"          json:"nfts"`

	// collections can be hidden from public-viewing
	HiddenBool bool `bson:"hidden,omitempty" json:"hidden"`
}

//-------------------------------------------------------------
func CollCreate(pColl *GLRYcollection,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {

	collNameStr := "glry_collections"
	gErr := gf_core.Mongo__insert(pColl,
		collNameStr,
		map[string]interface{}{
			"coll_name":      pColl.NameStr,
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
func CollGetByUserID(pUserIDstr GLRYuserID,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYcollection, *gf_core.Gf_error) {

	find_opts := options.Find()
	c, gErr := gf_core.MongoFind(bson.M{
		"owner_user_id": pUserIDstr,
		"deleted":       false,
	},
		find_opts,
		map[string]interface{}{
			"user_id":            pUserIDstr,
			"caller_err_msg_str": "failed to get collections from DB by user_id",
		},
		pRuntime.RuntimeSys.Mongo_db.Collection("glry_collections"),
		pCtx,
		pRuntime.RuntimeSys)

	if gErr != nil {
		return nil, gErr
	}

	var collsLst []*GLRYcollection
	err := c.All(pCtx, collsLst)
	if err != nil {
		gf_err := gf_core.Mongo__handle_error("failed to decode mongodb result of query to get Miners",
			"mongodb_cursor_decode",
			map[string]interface{}{},
			err, "gf_eth_monitor_core", pRuntime.RuntimeSys)

		return nil, gf_err
	}

	return collsLst, nil
}

//-------------------------------------------------------------
func CollGetByID(pIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollection, *gf_core.Gf_error) {

	var coll *GLRYcollection
	err := pRuntime.RuntimeSys.Mongo_db.Collection("glry_collections").FindOne(pCtx, bson.M{
		"_id":     pIDstr,
		"deleted": false,
	}).Decode(&coll)

	if err != nil {
		gf_err := gf_core.Mongo__handle_error("failed to query GLRYcollection by ID",
			"mongodb_find_error",
			map[string]interface{}{"id": pIDstr},
			err, "glry_db", pRuntime.RuntimeSys)
		return nil, gf_err
	}

	return coll, nil
}

//-------------------------------------------------------------
// CREATE_ID
func CollCreateID(pNameStr string,
	pOwnerUserIDstr string,
	pCreationTimeUNIXf float64) GLRYcollID {

	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pNameStr))
	h.Write([]byte(pOwnerUserIDstr))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID := GLRYcollID(hexStr)
	return ID
}
