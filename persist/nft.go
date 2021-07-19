package persist

import (
	"context"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	nftColName           = "nfts"
	nftCollectionColName = "nft_collections"
)

type Nft struct {
	VersionInt    int64   `bson:"version"              json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id,omitempty"                  json:"id" binding:"required"`
	CreationTimeF float64 `bson:"creation_time"        json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	NameStr           string `bson:"name"                 json:"name"`
	DescriptionStr    string `bson:"description"          json:"description"`
	CollectorsNoteStr string `bson:"collectors_note" json:"collectors_note"`
	OwnerUserIdStr    DbId   `bson:"owner_user_id" json:"user_id"`

	ExternalURLstr      string   `bson:"external_url"         json:"external_url"`
	TokenMetadataUrlStr string   `bson:"token_metadata_url" json:"token_metadata_url"`
	CreatorAddressStr   string   `bson:"creator_address"      json:"creator_address"`
	CreatorNameStr      string   `bson:"creator_name" json:"creator_name"`
	OwnerAddressStr     string   `bson:"owner_address" json:"owner_address"`
	Contract            Contract `bson:"contract"     json:"asset_contract"`

	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenSeaIDstr      string `bson:"opensea_id"       json:"opensea_id"`
	OpenSeaTokenIDstr string `bson:"opensea_token_id" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURLstr             string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURLstr    string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURLstr      string `bson:"image_preview_url"   json:"image_preview_url"`
	ImageOriginalUrlStr     string `bson:"image_original_url" json:"image_original_url"`
	AnimationUrlStr         string `bson:"animation_url" json:"animation_url"`
	AnimationOriginalUrlStr string `bson:"animation_original_url" json:"animation_original_url"`

	AcquisitionDateStr string `bson:"acquisition_date" json:"acquisition_date"`
}

type Contract struct {
	ContractAddressStr      string `bson:"contract_address"     json:"address"`
	ContractNameStr         string `bson:"contract_name" json:"name"`
	ContractDescription     string `bson:"contract_description" json:"description"`
	ContractExternalLinkStr string `bson:"contract_external_link" json:"external_link"`
	ContractSchemaNameStr   string `bson:"contract_schema_name" json:"schema_name"`
	ContractSymbolStr       string `bson:"contract_symbol" json:"symbol"`
	ContractTotalSupplyInt  int    `bson:"contract_total_supply" json:"total_supply"`
}

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

func NftCreate(pNFT *Nft,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, nftColName, pRuntime)

	return mp.Insert(pCtx, pNFT)
}

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

	if err := mp.Find(pCtx, bson.M{"owner_user_id": pUserIDstr}, &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func NftGetById(pIDstr DbId, pCtx context.Context, pRuntime *runtime.Runtime) ([]*Nft, error) {

	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, nftColName, pRuntime)
	result := []*Nft{}

	if err := mp.Find(pCtx, bson.M{"_id": pIDstr}, &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func NftUpdateById(pIDstr DbId, updatedNft *Nft, pCtx context.Context, pRuntime *runtime.Runtime) error {

	//------------------
	// VALIDATE
	if err := runtime.Validate(updatedNft, pRuntime); err != nil {
		return err
	}

	mp := NewMongoStorage(0, nftColName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pIDstr}, updatedNft)
}

func NftBulkUpsertOrRemove(walletAddress string, pNfts []*Nft, pCtx context.Context, pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, nftColName, pRuntime)

	// UPSERT
	// --------------------------------------------------------
	weWantToUpsertHere := true

	upsertModels := make([]mongo.WriteModel, len(pNfts))

	for i, v := range pNfts {

		if v.OpenSeaIDstr == "" {
			return errors.New("open sea id required for each nft")
		}

		now := float64(time.Now().UnixNano()) / 1000000000.0

		// TODO last updated

		upsertModels[i] = &mongo.UpdateOneModel{
			Upsert: &weWantToUpsertHere,
			Filter: bson.M{"owner_address": walletAddress, "opensea_id": v.OpenSeaIDstr},
			Update: bson.M{
				"$setOnInsert": bson.M{"_id": generateId(now), "created_at": now},
				"$set":         v,
			},
		}
	}

	if _, err := mp.collection.BulkWrite(pCtx, upsertModels); err != nil {
		return err
	}

	// FIND DIFFERENCE AND DELETE OUTLIERS
	// -------------------------------------------------------
	opts := &options.FindOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	dbNfts := []*Nft{}
	if err := mp.Find(pCtx, bson.M{"owner_address": walletAddress}, &dbNfts, opts); err != nil {
		return err
	}

	if len(dbNfts) > len(pNfts) {
		diff, err := findDifference(pNfts, dbNfts)
		if err != nil {
			return err
		}

		deleteModels := make([]mongo.WriteModel, len(diff))

		for i, v := range diff {
			deleteModels[i] = &mongo.UpdateOneModel{Filter: bson.M{"_id": v}, Update: bson.M{"$set": bson.M{"deleted": true}}}
		}

		if _, err := mp.collection.BulkWrite(pCtx, deleteModels); err != nil {
			return err
		}
	}

	return nil
}

func findDifference(nfts []*Nft, dbNfts []*Nft) ([]DbId, error) {
	currOpenseaIds := map[string]bool{}
	diff := []DbId{}

	for _, v := range nfts {
		currOpenseaIds[v.OpenSeaIDstr] = true
	}

	for _, v := range dbNfts {
		if !currOpenseaIds[v.OpenSeaIDstr] || v.OpenSeaIDstr == "" {
			diff = append(diff, v.IDstr)
		}
	}

	return diff, nil
}
