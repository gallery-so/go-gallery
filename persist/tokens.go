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

// TTB represents time til blockchain so that data isn't old in DB
var TTB = time.Minute * 10

// Token represents an individual Token token
type Token struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	TokenURI       string                 `bson:"token_uri" json:"token_uri"`
	TokenID        string                 `bson:"token_id" json:"token_id"`
	OwnerAddress   string                 `bson:"owner_address" json:"owner_address"`
	PreviousOwners []string               `bson:"previous_owners" json:"previous_owners"`
	LastBlockNum   string                 `bson:"last_block_num" json:"last_block_num"`
	TokenMetadata  map[string]interface{} `bson:"token_metadata" json:"token_metadata"`

	TokenContract TokenContract `bson:"token_contract" json:"token_contract"`
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

// TokenCreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func TokenCreateBulk(pCtx context.Context, pERC721s []*Token,
	pRuntime *runtime.Runtime) ([]DBID, error) {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

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

// TokenCreate inserts an ERC721 into the database
func TokenCreate(pCtx context.Context, pERC721 *Token,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	return mp.insert(pCtx, pERC721)
}

// TokenGetByWallet gets ERC721 tokens for a given wallet address
func TokenGetByWallet(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress), "last_updated": bson.M{"$gt": time.Now().Add(-TTB)}}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenGetByContract gets ERC721 tokens for a given contract
func TokenGetByContract(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"token_contract.contract_address": strings.ToLower(pAddress), "last_updated": bson.M{"$gt": time.Now().Add(-TTB)}}, &result, opts)
	if err != nil {
		return nil, err
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
	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	result := []*Token{}

	err := mp.find(pCtx, bson.M{"token_id": pTokenID, "token_contract.contract_address": strings.ToLower(pAddress), "last_updated": bson.M{"$lt": time.Now().Add(-TTB)}}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// TokenBulkUpsert will create a bulk operation on the database to upsert many erc721s for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func TokenBulkUpsert(pCtx context.Context, pERC721s []*Token, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

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
func TokenUpdateByID(pCtx context.Context, pID DBID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	return mp.update(pCtx, bson.M{"_id": pID}, pUpdate)
}
