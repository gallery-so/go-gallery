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

// ERC721 represents an individual ERC721 token
type ERC721 struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	TokenURI       string   `bson:"token_uri" json:"token_uri"`
	TokenID        string   `bson:"token_id" json:"token_id"`
	OwnerAddress   string   `bson:"owner_address" json:"owner_address"`
	PreviousOwners []string `bson:"previous_owners" json:"previous_owners"`
	LastBlockNum   string   `bson:"last_block_num" json:"last_block_num"`

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

// ERC721Create inserts an ERC721 into the database
func ERC721Create(pCtx context.Context, pERC721 *ERC721,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	return mp.insert(pCtx, pERC721)
}

// ERC721GetByWallet gets ERC721 tokens for a given wallet address
func ERC721GetByWallet(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*ERC721, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	result := []*ERC721{}

	err := mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ERC721GetByContract gets ERC721 tokens for a given contract
func ERC721GetByContract(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*ERC721, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	result := []*ERC721{}

	err := mp.find(pCtx, bson.M{"token_contract.contract_address": strings.ToLower(pAddress), "last_updated": bson.M{"$lt": time.Now().Add(-TTB)}}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ERC721BulkUpsert will create a bulk operation on the database to upsert many erc721s for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func ERC721BulkUpsert(pCtx context.Context, pERC721s []*ERC721, pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.InfraDBName, tokenColName, pRuntime)

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	errs := []error{}
	wg.Add(len(pERC721s))

	for _, v := range pERC721s {

		go func(erc721 *ERC721) {
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
