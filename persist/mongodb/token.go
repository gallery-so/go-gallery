package mongodb

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	tokenColName = "tokens"
)

// TokenMongoRepository is a repository that stores tokens in a MongoDB database
type TokenMongoRepository struct {
	mp  *storage
	nmp *storage
}

// NewTokenMongoRepository creates a new instance of the collection mongo repository
func NewTokenMongoRepository(mgoClient *mongo.Client) *TokenMongoRepository {
	return &TokenMongoRepository{
		mp:  newStorage(mgoClient, 0, galleryDBName, tokenColName),
		nmp: newStorage(mgoClient, 0, galleryDBName, usersCollName),
	}
}

// CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func (t *TokenMongoRepository) CreateBulk(pCtx context.Context, pERC721s []*persist.Token) ([]persist.DBID, error) {

	nfts := make([]interface{}, len(pERC721s))

	for i, v := range pERC721s {
		nfts[i] = v
	}

	ids, err := t.mp.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Create inserts a token into the database
func (t *TokenMongoRepository) Create(pCtx context.Context, pERC721 *persist.Token) (persist.DBID, error) {

	return t.mp.insert(pCtx, pERC721)
}

// GetByWallet gets tokens for a given wallet address
func (t *TokenMongoRepository) GetByWallet(pCtx context.Context, pAddress string) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByUserID gets ERC721 tokens for a given userID
func (t *TokenMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"owner_user_id": pUserID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByContract gets ERC721 tokens for a given contract
func (t *TokenMongoRepository) GetByContract(pCtx context.Context, pAddress string) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByNFTIdentifiers gets tokens for a given contract address and token ID
func (t *TokenMongoRepository) GetByNFTIdentifiers(pCtx context.Context, pTokenID string, pAddress string) (*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"token_id": pTokenID, "contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	if len(result) < 1 {
		return nil, persist.ErrTokenNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pAddress}
	}

	if len(result) > 1 {
		logrus.Errorf("found more than one token for contract address: %s token ID: %s", pAddress, pTokenID)
	}

	return result[0], nil
}

// GetByID gets tokens for a given DB ID
func (t *TokenMongoRepository) GetByID(pCtx context.Context, pID persist.DBID) (*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"_id": pID}, &result, opts)
	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrTokenNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// BulkUpsert will create a bulk operation on the database to upsert many tokens for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) BulkUpsert(pCtx context.Context, pERC721s []*persist.Token) error {

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	errs := []error{}
	wg.Add(len(pERC721s))

	for _, v := range pERC721s {

		go func(token *persist.Token) {
			defer wg.Done()
			_, err := t.mp.upsert(pCtx, bson.M{"token_id": token.TokenID, "contract_address": strings.ToLower(token.ContractAddress), "owner_address": token.OwnerAddress}, token)
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

// Upsert will upsert a token into the database
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) Upsert(pCtx context.Context, pToken *persist.Token) error {

	_, err := t.mp.upsert(pCtx, bson.M{"token_id": pToken.TokenID, "contract_address": strings.ToLower(pToken.ContractAddress), "owner_address": pToken.OwnerAddress}, pToken)
	return err
}

// UpdateByIDUnsafe will update a given token by its DB ID and owner user ID
func (t *TokenMongoRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	return t.mp.update(pCtx, bson.M{"_id": pID}, pUpdate)

}

// UpdateByID will update a given token by its DB ID and owner user ID
func (t *TokenMongoRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	users := []*persist.User{}
	err := t.nmp.find(pCtx, bson.M{"_id": pUserID}, &users, options.Find())
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return persist.ErrUserNotFoundByID{ID: pUserID}
	}
	user := users[0]

	return t.mp.update(pCtx, bson.M{"_id": pID, "owner_address": bson.M{"$in": user.Addresses}}, pUpdate)

}
