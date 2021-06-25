package glry_db

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
type GLRYnftID string
type GLRYnft struct {
	VersionInt    int64     `bson:"version"              json:"version"` // schema version for this model
	IDstr         GLRYnftID `bson:"_id"                  json:"id"`
	CreationTimeF float64   `bson:"creation_time"        json:"creation_time"`
	DeletedBool   bool      `bson:"deleted"`

	NameStr            string   `bson:"name"                 json:"name"`
	DescriptionStr     string   `bson:"description"          json:"description"`
	CollectionNamesLst []string `bson:"collection_names"     json:"collection_names"`

	ExternalURLstr     string `bson:"external_url"         json:"external_url"`
	CreatorAddressStr  string `bson:"creator_address"      json:"creator_address"`
	ContractAddressStr string `bson:"contract_address"     json:"contract_address"`

	// OPEN_SEA_TOKEN_ID
	OpenSeaIDstr      string `bson:"opensea_id"       json:"opensea_id"`
	OpenSeaTokenIDstr string `bson:"opensea_token_id" json:"opensea_token_id"` // add a comment describing what this is

	// IMAGES - OPENSEA
	ImageURLstr          string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURLstr string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURLstr   string `bson:"image_preview_url"   json:"image_preview_url"`

	HiddenBool bool `bson:"hidden"   json:"hidden"`
}

/*// IS THIS REALLY NECESSARY? - why not just import directly from v0 DB into the v1 DB GLRYnft format?
// DEPRECATED!! - will be removed once we fully migrate to v1 server/db schema.
//                legacy NFT type, this is the schema in the initial v0 prototype of the system.
type GLRYnftLegacy struct {

	// ID - for now generated by the DB
	IDstr string `bson:"_id" json:"id"`

	// removed from newer NFT model, NFT's might be associated with multiple Users,
	// so we dont want to limit to a single user.
	UserIDstr string `bson:"user_id" json:"user_id"`

	ImageURLstr       string `bson:"image_url"           json:"image_url"`
	DescriptionStr    string `bson:"description"         json:"description"`
	NameStr           string `bson:"name"                json:"name"`
	CollectionNameStr string `bson:"collection_name"     json:"collection_name"`

	ExternalURLstr       string    `bson:"external_url"        json:"external_url"`
	CreatedDateF         float64   `bson:"created_date"        json:"created_date"`
	CreatorAddressStr    string    `bson:"creator_address"     json:"creator_address"`
	ContractAddressStr   string    `bson:"contract_address"    json:"contract_address"`
	TokenIDstr           int64     `bson:"token_id"            json:"token_id"`
	ImageThumbnailURLstr string    `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURLstr   string    `bson:"image_preview_url"   json:"image_preview_url"`

	PositionInt int64 `bson:"position" json:"position"`
	HiddenBool  bool  `bson:"hidden"   json:"hidden"`
}*/

//-------------------------------------------------------------
func NFTcreateBulk(pNFTlst []*GLRYnft,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	IDsLst := []string{}
	recordsLst := []interface{}{}
	for _, n := range pNFTlst {
		IDsLst = append(IDsLst, string(n.IDstr))
		recordsLst = append(recordsLst, interface{}(n))
	}

	collNameStr := "glry_nfts"
	gErr := gfcore.Mongo__insert_bulk(IDsLst, recordsLst,
		collNameStr,
		map[string]interface{}{
			"nft_ids":            IDsLst,
			"caller_err_msg_str": "failed to bulk insert NFTs (GLRYnft) into DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
func NFTcreate(pNFT *GLRYnft,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	collNameStr := "glry_nfts"
	gErr := gfcore.Mongo__insert(pNFT,
		collNameStr,
		map[string]interface{}{
			"nft_name":       pNFT.NameStr,
			"nft_image_url":  pNFT.ImageURLstr,
			"caller_err_msg": "failed to insert a new NFT into the DB",
		},
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
func NFTgetByUserID(pUserIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYnft, *gfcore.Gf_error) {

	return nil, nil
}

//-------------------------------------------------------------

func NFTgetByID(pIDstr string, pCtx context.Context, pRuntime *glry_core.Runtime) ([]*GLRYnft, *gfcore.Gf_error) {

	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	col := pRuntime.RuntimeSys.Mongo_db.Collection("glry_nfts")

	cur, gErr := gfcore.Mongo__find(bson.M{"_id": pIDstr},
		opts,
		map[string]interface{}{},
		col,
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return nil, gErr
	}
	result := []*GLRYnft{}

	if err := cur.All(pCtx, &result); err != nil {
		return nil, gfcore.Error__create("nft id not found in query values",
			"mongodb_cursor_all",
			map[string]interface{}{}, err, "glry_core", pRuntime.RuntimeSys)
	}

	return result, nil

}

//-------------------------------------------------------------
func NFTcreateID(pNameStr string,
	pCreatorAddressStr string,
	pCreationTimeUNIXf float64) GLRYnftID {

	h := md5.New()
	h.Write([]byte(fmt.Sprint(pCreationTimeUNIXf)))
	h.Write([]byte(pNameStr))
	h.Write([]byte(pCreatorAddressStr))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum)
	ID := GLRYnftID(hexStr)
	return ID
}
