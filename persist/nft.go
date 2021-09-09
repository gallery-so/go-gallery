package persist

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	nftColName           = "nfts"
	nftCollectionColName = "nft_collections"
)

// Nft represents an nft both in the database and throughout the application
type Nft struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
	OwnerUserID    DBID   `bson:"owner_user_id" json:"user_id"`

	Name             string   `bson:"name"                 json:"name"`
	Description      string   `bson:"description"          json:"description"`
	ExternalURL      string   `bson:"external_url"         json:"external_url"`
	TokenMetadataURL string   `bson:"token_metadata_url" json:"token_metadata_url"`
	CreatorAddress   string   `bson:"creator_address"      json:"creator_address"`
	CreatorName      string   `bson:"creator_name" json:"creator_name"`
	OwnerAddress     string   `bson:"owner_address" json:"owner_address"`
	Contract         Contract `bson:"contract"     json:"asset_contract"`

	OpenSeaID int `bson:"opensea_id"       json:"opensea_id"`
	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenSeaTokenID string `bson:"opensea_token_id" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURL             string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL    string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL      string `bson:"image_preview_url"   json:"image_preview_url"`
	ImageOriginalURL     string `bson:"image_original_url" json:"image_original_url"`
	AnimationURL         string `bson:"animation_url" json:"animation_url"`
	AnimationOriginalURL string `bson:"animation_original_url" json:"animation_original_url"`

	AcquisitionDateStr string `bson:"acquisition_date" json:"acquisition_date"`
}

// Contract represents a smart contract's information for a given NFT
type Contract struct {
	ContractAddress      string `bson:"contract_address"     json:"address"`
	ContractName         string `bson:"contract_name" json:"name"`
	ContractImage        string `bson:"contract_image_url" json:"image_url"`
	ContractDescription  string `bson:"contract_description" json:"description"`
	ContractExternalLink string `bson:"contract_external_link" json:"external_link"`
	ContractSchemaName   string `bson:"contract_schema_name" json:"schema_name"`
	ContractSymbol       string `bson:"contract_symbol" json:"symbol"`
	ContractTotalSupply  string `bson:"contract_total_supply" json:"total_supply"`
}

// UpdateNFTInfoInput represents a MongoDB input to update the user defined info
// associated with a given NFT in the DB
type UpdateNFTInfoInput struct {
	CollectorsNote string `bson:"collectors_note"`
}

// NftCreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func NftCreateBulk(pCtx context.Context, pNfts []*Nft,
	pRuntime *runtime.Runtime) ([]DBID, error) {

	mp := newStorage(0, nftColName, pRuntime)

	nfts := make([]interface{}, len(pNfts))

	for i, v := range pNfts {
		nfts[i] = v
	}

	ids, err := mp.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// NftCreate inserts an NFT into the database
func NftCreate(pCtx context.Context, pNFT *Nft,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, nftColName, pRuntime)

	return mp.insert(pCtx, pNFT)
}

// NftGetByUserID finds an nft by its owner user id
func NftGetByUserID(pCtx context.Context, pUserID DBID,
	pRuntime *runtime.Runtime) ([]*Nft, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, nftColName, pRuntime)
	result := []*Nft{}

	if err := mp.find(pCtx, bson.M{"owner_user_id": pUserID}, &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// NftGetByID finds an nft by its id
func NftGetByID(pCtx context.Context, pID DBID, pRuntime *runtime.Runtime) ([]*Nft, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, nftColName, pRuntime)
	result := []*Nft{}

	if err := mp.find(pCtx, bson.M{"_id": pID}, &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// NftUpdateByID updates an nft by its id, also ensuring that the NFT is owned
// by a given authorized user
// pUpdate is a struct that has bson tags representing the fields to be updated
func NftUpdateByID(pCtx context.Context, pID DBID, pUserID DBID, pUpdate interface{}, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, nftColName, pRuntime)

	return mp.update(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, pUpdate)
}

// NftBulkUpsert will create a bulk operation on the database to upsert many nfts for a given wallet address
// This function's primary purpose is to be used when syncing a user's NFTs from an external provider
func NftBulkUpsert(pCtx context.Context, walletAddress string, pNfts []*Nft, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, nftColName, pRuntime)

	upsertModels := make([]mongo.WriteModel, len(pNfts))

	for i, v := range pNfts {

		now := primitive.NewDateTimeFromTime(time.Now())

		asMap, err := structToBsonMap(v)
		if err != nil {
			return err
		}

		asMap["last_updated"] = now

		// we don't need ID because we are searching by opensea ID.
		// this will cause update conflict if not ""
		delete(asMap, "_id")

		// set created at if this is a new insert
		if _, ok := asMap["created_at"]; !ok {
			asMap["created_at"] = now
		}

		upsertModels[i] = &mongo.UpdateOneModel{
			Upsert: boolin(true),
			Filter: bson.M{"opensea_id": v.OpenSeaID},
			Update: bson.M{
				"$setOnInsert": bson.M{
					"_id": generateID(asMap),
				},
				"$set": asMap,
			},
		}
	}

	if _, err := mp.collection.BulkWrite(pCtx, upsertModels); err != nil {
		return err
	}

	return nil
}

// NftRemoveDifference will update all nfts that are not in the given slice of nfts with having an
// empty owner id and address
func NftRemoveDifference(pCtx context.Context, pNfts []*Nft, pWalletAddress string, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, nftColName, pRuntime)
	// FIND DIFFERENCE AND DELETE OUTLIERS
	// -------------------------------------------------------
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	dbNfts := []*Nft{}
	if err := mp.find(pCtx, bson.M{"owner_address": pWalletAddress}, &dbNfts, opts); err != nil {
		return err
	}

	if len(dbNfts) > len(pNfts) {
		diff, err := findDifference(pNfts, dbNfts)
		if err != nil {
			return err
		}

		deleteModels := make([]mongo.WriteModel, len(diff))

		for i, v := range diff {
			deleteModels[i] = &mongo.UpdateOneModel{Filter: bson.M{"_id": v}, Update: bson.M{"$set": bson.M{"owner_user_id": "", "owner_address": ""}}}
		}

		if _, err := mp.collection.BulkWrite(pCtx, deleteModels); err != nil {
			return err
		}
	}

	return nil
}

// NftOpenseaCacheSet adds a set of nfts to the opensea cache under a given wallet address
func NftOpenseaCacheSet(pCtx context.Context, pWalletAddress string, pNfts []*Nft, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, nftColName, pRuntime).withRedis(OpenseaAssetsRDB, pRuntime)
	defer mp.cacheClose()

	toCache, err := json.Marshal(pNfts)
	if err != nil {
		return err
	}
	return mp.cacheSet(pCtx, string(pWalletAddress), toCache, openseaAssetsTTL)
}

// NftOpenseaCacheGet gets a set of nfts from the opensea cache under a given wallet address
func NftOpenseaCacheGet(pCtx context.Context, pWalletAddress string, pRuntime *runtime.Runtime) ([]*Nft, error) {

	mp := newStorage(0, nftColName, pRuntime).withRedis(OpenseaAssetsRDB, pRuntime)
	defer mp.cacheClose()

	result, err := mp.cacheGet(pCtx, string(pWalletAddress))
	if err != nil {
		return nil, err
	}

	nfts := []*Nft{}
	if err := json.Unmarshal([]byte(result), &nfts); err != nil {
		return nil, err
	}
	return nfts, nil
}

func findDifference(nfts []*Nft, dbNfts []*Nft) ([]DBID, error) {
	currOpenseaIds := map[int]bool{}

	for _, v := range nfts {
		currOpenseaIds[v.OpenSeaID] = true
	}

	diff := []DBID{}
	for _, v := range dbNfts {
		if !currOpenseaIds[v.OpenSeaID] {
			diff = append(diff, v.ID)
		}
	}

	return diff, nil
}
