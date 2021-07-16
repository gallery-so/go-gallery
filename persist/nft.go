package persist

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------

const (
	nftColName           = "nfts"
	nftCollectionColName = "nft_collections"
)

type Nft struct {
	VersionInt    int64   `bson:"version,omitempty"              json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id,omitempty"                  json:"id"`
	CreationTimeF float64 `bson:"creation_time,omitempty"        json:"creation_time"`
	DeletedBool   bool    `bson:"deleted,omitempty"`

	NameStr           string `bson:"name,omitempty"                 json:"name"`
	DescriptionStr    string `bson:"description,omitempty"          json:"description"`
	CollectorsNoteStr string `bson:"collectors_note,omitempty" json:"collectors_note"`
	OwnerUserIdStr    DbId   `bson:"owner_user_id" json:"user_id"`

	ExternalURLstr      string   `bson:"external_url,omitempty"         json:"external_url"`
	TokenMetadataUrlStr string   `bson:"token_metadata_url,omitempty" json:"token_metadata_url"`
	CreatorAddressStr   string   `bson:"creator_address,omitempty"      json:"creator_address"`
	CreatorNameStr      string   `bson:"creator_name,omitempty" json:"creator_name"`
	OwnerAddressStr     string   `bson:"owner_address,omitempty" json:"owner_address"`
	Contract            Contract `bson:"contract,omitempty"     json:"asset_contract"`

	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenSeaIDstr      string `bson:"opensea_id,omitempty"       json:"opensea_id"`
	OpenSeaTokenIDstr string `bson:"opensea_token_id,omitempty" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURLstr             string `bson:"image_url,omitempty"           json:"image_url"`
	ImageThumbnailURLstr    string `bson:"image_thumbnail_url,omitempty" json:"image_thumbnail_url"`
	ImagePreviewURLstr      string `bson:"image_preview_url,omitempty"   json:"image_preview_url"`
	ImageOriginalUrlStr     string `bson:"image_original_url,omitempty" json:"image_original_url"`
	AnimationUrlStr         string `bson:"animation_url,omitempty" json:"animation_url"`
	AnimationOriginalUrlStr string `bson:"animation_original_url,omitempty" json:"animation_original_url"`

	AcquisitionDateStr string `bson:"acquisition_date,omitempty" json:"acquisition_date"`
}

type Contract struct {
	ContractAddressStr      string `bson:"contract_address,omitempty"     json:"address"`
	ContractNameStr         string `bson:"contract_name,omitempty" json:"name"`
	ContractDescription     string `bson:"contract_description,omitempty" json:"description"`
	ContractExternalLinkStr string `bson:"contract_external_link,omitempty" json:"external_link"`
	ContractSchemaNameStr   string `bson:"contract_schema_name,omitempty" json:"schema_name"`
	ContractSymbolStr       string `bson:"contract_symbol,omitempty" json:"symbol"`
	ContractTotalSupplyInt  int    `bson:"contract_total_supply,omitempty" json:"total_supply"`
}

//-------------------------------------------------------------
func NftCreateBulk(pNFTlst []*Nft,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]DbId, error) {

	mp := NewMongoStorage(0, nftColName, pRuntime)

	nfts := make([]interface{}, len(pNFTlst))

	for i, v := range pNFTlst {
		nfts[i] = v
	}

	ids, err := mp.InsertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil

}

//-------------------------------------------------------------
func NftCreate(pNFT *Nft,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, nftColName, pRuntime)

	return mp.Insert(pCtx, pNFT)

}

//-------------------------------------------------------------
func NftGetByUserId(pUserIDstr DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*Nft, error) {
	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}
	mp := NewMongoStorage(0, nftColName, pRuntime)
	result := []*Nft{}

	if err := mp.Find(pCtx, bson.M{"owner_user_id": pUserIDstr}, result, opts); err != nil {
		return nil, err
	}

	return nil, nil
}

//-------------------------------------------------------------

func NeftGetById(pIDstr DbId, pCtx context.Context, pRuntime *runtime.Runtime) ([]*Nft, error) {

	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, nftColName, pRuntime)
	result := []*Nft{}

	if err := mp.Find(pCtx, bson.M{"_id": pIDstr}, result, opts); err != nil {
		return nil, err
	}

	return result, nil

}

//-------------------------------------------------------------

func NftUpdateById(pIDstr DbId, updatedNft *Nft, pCtx context.Context, pRuntime *runtime.Runtime) error {

	//------------------
	// VALIDATE
	if err := runtime.Validate(updatedNft, pRuntime); err != nil {
		return err
	}

	mp := NewMongoStorage(0, nftColName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pIDstr}, updatedNft)

}
