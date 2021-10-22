package mongodb

import (
	"context"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	tokenColName = "tokens"
)

var errNoTokensFound = errors.New("no tokens found")

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
func (t *TokenMongoRepository) GetByWallet(pCtx context.Context, pAddress persist.Address) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"owner_address": pAddress}, &result, opts)
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
func (t *TokenMongoRepository) GetByContract(pCtx context.Context, pAddress persist.Address) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"contract_address": pAddress}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByTokenIdentifiers gets tokens for a given contract address and token ID
func (t *TokenMongoRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pAddress persist.Address) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"token_id": pTokenID, "contract_address": pAddress}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
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
func (t *TokenMongoRepository) BulkUpsert(pCtx context.Context, pTokens []*persist.Token) error {

	ids := make(chan persist.DBID)
	errs := make(chan error)

	for _, v := range pTokens {

		go func(token *persist.Token) {

			query := bson.M{"token_id": token.TokenID, "contract_address": token.ContractAddress, "owner_address": token.OwnerAddress}
			returnID, err := t.mp.upsert(pCtx, query, token)
			if err != nil {
				errs <- err
				return
			}
			ids <- returnID
		}(v)
	}

	result := make([]persist.DBID, len(pTokens))
	for i := 0; i < len(pTokens); i++ {
		select {
		case id := <-ids:
			result[i] = id
		case err := <-errs:
			return err
		}
	}
	return nil
}

// Upsert will upsert a token into the database
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) Upsert(pCtx context.Context, pToken *persist.Token) error {

	_, err := t.mp.upsert(pCtx, bson.M{"token_id": pToken.TokenID, "contract_address": pToken.ContractAddress, "owner_address": pToken.OwnerAddress}, pToken)
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

// MostRecentBlock will find the most recent block stored for all tokens
func (t *TokenMongoRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {

	opts := options.Find()
	opts.SetLimit(1)
	opts.SetSort(bson.M{"latest_block": -1})

	res := []*persist.Token{}

	if err := t.mp.find(pCtx, bson.M{}, &res, opts); err != nil {
		return 0, err
	}

	if len(res) < 1 {
		return 0, errNoTokensFound
	}

	return res[0].LatestBlock, nil

}

// Count will find the most recent block stored for all tokens
func (t *TokenMongoRepository) Count(pCtx context.Context) (int64, error) {

	count, err := t.mp.count(pCtx, bson.M{})
	if err != nil {
		return 0, err
	}

	return count, nil

}
