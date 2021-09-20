package persist

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	tokenColName = "tokens"
)

const tokenPageSize = 50

// TTB represents time til blockchain so that data isn't old in DB
var TTB = time.Minute * 10

// Token represents an individual Token token
type Token struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
	OwnerUserID    DBID   `bson:"owner_user_id" json:"user_id"`
	ThumbnailURL   string `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL     string `bson:"preview_url" json:"preview_url"`

	TokenURI       string                 `bson:"token_uri" json:"token_uri"`
	TokenID        string                 `bson:"token_id" json:"token_id"`
	OwnerAddress   string                 `bson:"owner_address" json:"owner_address"`
	PreviousOwners []string               `bson:"previous_owners" json:"previous_owners"`
	TokenMetadata  map[string]interface{} `bson:"token_metadata" json:"token_metadata"`

	TokenContract TokenContract `bson:"token_contract" json:"token_contract"`
}

// CollectionToken represents a token within a collection
type CollectionToken struct {
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`

	OwnerUserID DBID          `bson:"owner_user_id" json:"user_id"`
	Contract    TokenContract `bson:"contract"     json:"asset_contract"`

	// IMAGES - OPENSEA
	ThumbnailURL  string                 `bson:"thumbnail_url" json:"thumbnail_url"`
	PreviewURL    string                 `bson:"preview_url" json:"preview_url"`
	TokenMetadata map[string]interface{} `bson:"token_metadata" json:"token_metadata"`
}

// TokenContract represents the contract for a given ERC721
type TokenContract struct {
	Address   string `bson:"contract_address" json:"contract_address"`
	Symbol    string `bson:"symbol" json:"symbol"`
	TokenName string `bson:"token_name" json:"token_name"`
}

type attribute struct {
	TraitType string `bson:"trait_type" json:"trait_type"`
	Value     string `bson:"value" json:"value"`
}

// TokenUpdateWithTransfer represents a token update occuring after a transfer event
type TokenUpdateWithTransfer struct {
	OwnerAddress   string   `bson:"owner_address"`
	PreviousOwners []string `bson:"previous_owners"`
	LastBlockNum   string   `bson:"last_block_num"`
}

// TokenUpdateInfoInput represents a token update to update the token's user inputted info
type TokenUpdateInfoInput struct {
	CollectorsNote string `json:"collectors_note"`
}

// TokenCreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func TokenCreateBulk(pCtx context.Context, pERC721s []*Token,
	pRuntime *runtime.Runtime) ([]DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

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

// TokenCreate inserts a token into the database
func TokenCreate(pCtx context.Context, pERC721 *Token,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	return mp.insert(pCtx, pERC721)
}

// TokenGetByWallet gets tokens for a given wallet address
func TokenGetByWallet(pCtx context.Context, pAddress string, pPageNumber int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	if pPageNumber != 0 && len(result) > pPageNumber*tokenPageSize {
		return result[(pPageNumber-1)*tokenPageSize : pPageNumber*tokenPageSize], nil
	}

	return result, nil
}

// TokenGetByUserID gets ERC721 tokens for a given userID
func TokenGetByUserID(pCtx context.Context, pUserID DBID, pPageNumber int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"owner_user_id": pUserID}, &result, opts)
	if err != nil {
		return nil, err
	}

	if pPageNumber != 0 && len(result) > pPageNumber*tokenPageSize {
		return result[(pPageNumber-1)*tokenPageSize : pPageNumber*tokenPageSize], nil
	}

	return result, nil
}

// TokenGetByContract gets ERC721 tokens for a given contract
func TokenGetByContract(pCtx context.Context, pAddress string, pPageNumber int,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"token_contract.contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}
	if pPageNumber != 0 && len(result) > pPageNumber*tokenPageSize {
		return result[(pPageNumber-1)*tokenPageSize : pPageNumber*tokenPageSize], nil
	}

	return result, nil
}

// TokenGetByTokenID gets tokens for a given contract address and token ID
func TokenGetByTokenID(pCtx context.Context, pTokenID string, pAddress string,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"token_id": pTokenID, "token_contract.contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByID gets tokens for a given DB ID
func TokenGetByID(pCtx context.Context, pID DBID,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"_id": pID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenBulkUpsert will create a bulk operation on the database to upsert many erc721s for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func TokenBulkUpsert(pCtx context.Context, pERC721s []*Token, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	errs := []error{}
	wg.Add(len(pERC721s))

	for _, v := range pERC721s {

		go func(erc721 *Token) {
			defer wg.Done()
			_, err := mp.upsert(pCtx, bson.M{"token_id": erc721.TokenID, "token_contract.contract_address": strings.ToLower(erc721.TokenContract.Address)}, erc721)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(v)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// TokenUpdateByID will update a given token by its DB ID
func TokenUpdateByID(pCtx context.Context, pID DBID, pUserID DBID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)

	return mp.update(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, pUpdate)
}

// TokensClaim will ensure that tokens can only be in collections owned by the user
func TokensClaim(pCtx context.Context, pUserID DBID, pIDs []DBID,
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, tokenColName, pRuntime)
	allColls, err := CollGetByUserID(pCtx, pUserID, true, pRuntime)
	if err != nil {
		return err
	}

	allCollIDs := make([]DBID, len(allColls))
	for i, v := range allColls {
		allCollIDs[i] = v.ID
	}

	up := bson.M{"collection_id": ""}

	return mp.update(pCtx, bson.M{"collection_id": bson.M{"$nin": allCollIDs}, "_id": bson.M{"$in": pIDs}}, up)
}
