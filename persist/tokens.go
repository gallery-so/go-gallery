package persist

import (
	"context"
	"math/big"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	tokenColName = "tokens"
)

// ERC721 represents an individual ERC721 token
type ERC721 struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	TokenURI     string   `bson:"token_uri" json:"token_uri"`
	TokenID      *big.Int `bson:"token_id" json:"token_id"`
	OwnerAddress string   `bson:"owner_address" json:"owner_address"`

	TokenContract TokenContract `bson:"token_contract"`
}

// TokenContract represents the contract for a given ERC721
type TokenContract struct {
	Address   string `bson:"contract_address" json:"contract_address"`
	Symbol    string `bson:"symbol" json:"symbol"`
	TokenName string `bson:"token_name" json:"token_name"`
}

// ERC721CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func ERC721CreateBulk(pCtx context.Context, pERC721s []*ERC721,
	pRuntime *runtime.Runtime) ([]DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, nftColName, pRuntime)

	nfts := make([]interface{}, len(pERC721s))

	for i, v := range pERC721s {
		nfts[i] = v
	}

	ids, err := mp.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// ERC721Create inserts an ERC721 into the database
func ERC721Create(pCtx context.Context, pERC721 *ERC721,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, nftColName, pRuntime)

	return mp.insert(pCtx, pERC721)
}

// ERC721GetByAddress inserts an ERC721 into the database
func ERC721GetByAddress(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*ERC721, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.GalleryDBName, nftColName, pRuntime)

	result := []*ERC721{}

	err := mp.find(pCtx, bson.M{"owner_address": pAddress}, result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ERC721BulkUpsert will create a bulk operation on the database to upsert many erc721s for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func ERC721BulkUpsert(pCtx context.Context, walletAddress string, pERC721s []*ERC721, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, nftColName, pRuntime)

	upsertModels := make([]mongo.WriteModel, len(pERC721s))

	for i, v := range pERC721s {

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
			Filter: bson.M{"token_id": v.TokenID},
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

// ERC721RemoveDifference will update all nfts that are not in the given slice of erc721s with having an
// empty owner address
func ERC721RemoveDifference(pCtx context.Context, pNfts []*Nft, pWalletAddress string, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, nftColName, pRuntime)
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
		diff, err := findDifferenceOfNFTs(pNfts, dbNfts)
		if err != nil {
			return err
		}

		deleteModels := make([]mongo.WriteModel, len(diff))

		for i, v := range diff {
			deleteModels[i] = &mongo.UpdateOneModel{Filter: bson.M{"_id": v}, Update: bson.M{"$set": bson.M{"owner_address": ""}}}
		}

		if _, err := mp.collection.BulkWrite(pCtx, deleteModels); err != nil {
			return err
		}
	}

	return nil
}

func findDifference(nfts []*ERC721, dbNfts []*ERC721) ([]DBID, error) {
	currOpenseaIds := map[string]bool{}

	for _, v := range nfts {
		currOpenseaIds[v.TokenID.String()] = true
	}

	diff := []DBID{}
	for _, v := range dbNfts {
		if !currOpenseaIds[v.TokenID.String()] {
			diff = append(diff, v.ID)
		}
	}

	return diff, nil
}
