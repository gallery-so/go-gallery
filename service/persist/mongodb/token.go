package mongodb

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
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
	tokensStorage *storage
	usersStorage  *storage
	galleryRepo   *GalleryTokenMongoRepository
}

// NewTokenMongoRepository creates a new instance of the collection mongo repository
func NewTokenMongoRepository(mgoClient *mongo.Client, galleryRepo *GalleryTokenMongoRepository) *TokenMongoRepository {
	tokenStorage := newStorage(mgoClient, 0, galleryDBName, tokenColName)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	tokenIdentifiersIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "contract_address", Value: 1},
			{Key: "token_id", Value: 1},
		},
	}
	blockNumberIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "block_number", Value: -1},
		},
	}

	tiName, err := tokenStorage.createIndex(ctx, tokenIdentifiersIndex)
	if err != nil {
		panic(err)
	}
	bnName, err := tokenStorage.createIndex(ctx, blockNumberIndex)
	if err != nil {
		panic(err)
	}

	logrus.Infof("created indexes %s and %s", tiName, bnName)
	return &TokenMongoRepository{
		tokensStorage: tokenStorage,
		usersStorage:  newStorage(mgoClient, 0, galleryDBName, usersCollName),
		galleryRepo:   galleryRepo,
	}
}

// CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func (t *TokenMongoRepository) CreateBulk(pCtx context.Context, pTokens []persist.Token) ([]persist.DBID, error) {

	nfts := make([]interface{}, len(pTokens))

	for i, v := range pTokens {
		nfts[i] = v
	}

	ids, err := t.tokensStorage.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Create inserts a token into the database
func (t *TokenMongoRepository) Create(pCtx context.Context, pToken persist.Token) (persist.DBID, error) {

	return t.tokensStorage.insert(pCtx, pToken)
}

// GetByWallet gets tokens for a given wallet address
func (t *TokenMongoRepository) GetByWallet(pCtx context.Context, pAddress persist.Address, limit, page int64) ([]persist.Token, error) {
	opts := options.Find()

	if limit > 0 {
		opts.SetSkip(limit * page)
		opts.SetLimit(limit)
	}
	opts.SetSort(bson.M{"block_number": -1})

	result := []persist.Token{}

	cur, err := t.tokensStorage.collection.Find(pCtx, bson.M{"owner_address": pAddress}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(pCtx)
	if err := cur.All(pCtx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByUserID gets ERC721 tokens for a given userID
func (t *TokenMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, limit, page int64) ([]persist.Token, error) {
	opts := options.Find()
	if limit > 0 {
		opts.SetSkip(limit * page)
		opts.SetLimit(limit)
	}
	opts.SetSort(bson.M{"block_number": -1})

	result := []*persist.User{}
	err := t.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &result, opts)
	if err != nil {
		return nil, err
	}
	if len(result) != 1 {
		return nil, persist.ErrUserNotFoundByID{ID: pUserID}
	}
	user := result[0]
	tokens := []persist.Token{}
	resultChan := make(chan []persist.Token)
	errChan := make(chan error)
	for _, v := range user.Addresses {
		go func(addr persist.Address) {
			tokensForAddress, err := t.GetByWallet(pCtx, addr, limit, page)
			if err != nil {
				errChan <- err
			}
			resultChan <- tokensForAddress
		}(v)
	}

	for i := 0; i < len(user.Addresses); i++ {
		select {
		case t := <-resultChan:
			if len(t)+len(tokens) > int(limit) {
				tokens = append(tokens, t[:int(limit)-len(tokens)]...)
				break
			}
			tokens = append(tokens, t...)
		case err := <-errChan:
			return nil, err
		}
	}

	return tokens, nil
}

// GetByContract gets ERC721 tokens for a given contract
func (t *TokenMongoRepository) GetByContract(pCtx context.Context, pAddress persist.Address, limit, page int64) ([]persist.Token, error) {
	opts := options.Find()
	if limit > 0 {
		opts.SetSkip(limit * page)
		opts.SetLimit(limit)
	}
	opts.SetSort(bson.M{"block_number": -1})

	result := []persist.Token{}

	cur, err := t.tokensStorage.collection.Find(pCtx, bson.M{"contract_address": pAddress}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(pCtx)
	if err := cur.All(pCtx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByTokenIdentifiers gets tokens for a given contract address and token ID
func (t *TokenMongoRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pAddress persist.Address, limit, page int64) ([]persist.Token, error) {
	opts := options.Find()

	if limit > 0 {
		opts.SetSkip(limit * page)
		opts.SetLimit(limit)
	}
	opts.SetSort(bson.M{"block_number": -1})

	result := []persist.Token{}

	cur, err := t.tokensStorage.collection.Find(pCtx, bson.M{"token_id": pTokenID, "contract_address": pAddress}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(pCtx)
	if err := cur.All(pCtx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByID gets tokens for a given DB ID
func (t *TokenMongoRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Token, error) {

	result := []persist.Token{}

	cur, err := t.tokensStorage.collection.Find(pCtx, bson.M{"_id": pID})
	if err != nil {
		return persist.Token{}, err
	}
	defer cur.Close(pCtx)
	if err := cur.All(pCtx, &result); err != nil {
		return persist.Token{}, err
	}

	if len(result) != 1 {
		return persist.Token{}, persist.ErrTokenNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// BulkUpsert will create a bulk operation on the database to upsert many tokens for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) BulkUpsert(pCtx context.Context, pTokens []persist.Token) error {

	upsertModels := make([]updateModel, 0, len(pTokens))
	updateModels := make([]updateModel, 0, len(pTokens))
	delModels := make([]bson.M, 0, len(pTokens))

	for _, v := range pTokens {

		query := bson.M{"token_id": v.TokenID, "contract_address": v.ContractAddress}
		nextQuery := bson.M{"token_id": v.TokenID, "contract_address": v.ContractAddress, "block_number": bson.M{"$lte": v.BlockNumber}}
		if v.TokenType == persist.TokenTypeERC1155 {
			query["owner_address"] = v.OwnerAddress
			nextQuery["owner_address"] = v.OwnerAddress
		}

		if v.TokenType == persist.TokenTypeERC1155 && v.Quantity.BigInt() != nil {
			if v.Quantity.BigInt().Cmp(big.NewInt(0)) == 0 {
				delModels = append(delModels, query)
				continue
			}
		}

		setDocs := make(bson.D, 0, 3)
		nextSetDocs := make(bson.D, 0, 1)

		asBSON, err := bson.MarshalWithRegistry(CustomRegistry, v)
		if err != nil {
			return err
		}

		asMap := bson.M{}
		err = bson.UnmarshalWithRegistry(CustomRegistry, asBSON, &asMap)
		if err != nil {
			return err
		}
		delete(asMap, "_id")
		delete(asMap, "ownership_history")
		delete(asMap, "created_at")
		delete(asMap, "block_number")
		asMap["last_updated"] = time.Now()

		for k := range query {
			delete(asMap, k)
		}

		setDocs = append(setDocs, bson.E{Key: "$setOnInsert", Value: bson.M{"_id": persist.GenerateID(), "created_at": time.Now(), "block_number": v.BlockNumber}})

		if v.TokenType == persist.TokenTypeERC1155 {

			ownerDoc := bson.E{Key: "$set", Value: bson.M{"block_number": v.BlockNumber}}
			nextSetDocs = append(nextSetDocs, ownerDoc)

			updateModels = append(updateModels, updateModel{
				query:   nextQuery,
				setDocs: nextSetDocs,
			})
		}

		if v.TokenType == persist.TokenTypeERC721 {
			ownerHistory := bson.E{Key: "$push", Value: bson.M{"ownership_history": bson.M{"$each": v.OwnershipHistoty, "$sort": bson.M{"block": -1}}}}
			setDocs = append(setDocs, ownerHistory)

			delete(asMap, "owner_address")

			ownerDoc := bson.E{Key: "$set", Value: bson.M{"owner_address": v.OwnerAddress, "block_number": v.BlockNumber}}
			nextSetDocs = append(nextSetDocs, ownerDoc)

			updateModels = append(updateModels, updateModel{
				query:   nextQuery,
				setDocs: nextSetDocs,
			})
		}

		setDocs = append(setDocs, bson.E{Key: "$set", Value: asMap})

		upsertModels = append(upsertModels, updateModel{
			query:   query,
			setDocs: setDocs,
		})

	}

	now := time.Now()
	if err := t.tokensStorage.bulkUpdate(pCtx, upsertModels, true); err != nil {
		return err
	}
	logrus.Infof("Bulk upserted %d models in %s", len(upsertModels), time.Since(now))
	nextNow := time.Now()
	if err := t.tokensStorage.bulkUpdate(pCtx, updateModels, false); err != nil {
		return err
	}
	logrus.Infof("Bulk updated %d models in %s", len(updateModels), time.Since(nextNow))
	finalNow := time.Now()
	if err := t.tokensStorage.bulkDelete(pCtx, delModels); err != nil {
		return err
	}
	logrus.Infof("Bulk deleted %d models in %s", len(delModels), time.Since(finalNow))

	return nil
}

// Upsert will upsert a token into the database
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) Upsert(pCtx context.Context, pToken persist.Token) error {

	query := bson.M{"token_id": pToken.TokenID, "contract_address": pToken.ContractAddress}
	if pToken.TokenType == persist.TokenTypeERC1155 {
		query["owner_address"] = pToken.OwnerAddress
	}

	_, err := t.tokensStorage.upsert(pCtx, query, pToken)
	return err
}

// UpdateByIDUnsafe will update a given token by its DB ID and owner user ID
func (t *TokenMongoRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	if err := t.tokensStorage.update(pCtx, bson.M{"_id": pID}, pUpdate); err != nil {
		return err
	}

	return nil

}

// UpdateByID will update a given token by its DB ID and owner user ID
func (t *TokenMongoRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	users := []*persist.User{}
	err := t.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users, options.Find())
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return persist.ErrUserNotFoundByID{ID: pUserID}
	}
	user := users[0]

	if err := t.tokensStorage.update(pCtx, bson.M{"_id": pID, "owner_address": bson.M{"$in": user.Addresses}}, pUpdate); err != nil {
		return err
	}

	go t.galleryRepo.resetCache(pCtx, pUserID)
	return nil

}

// MostRecentBlock will find the most recent block stored for all tokens
func (t *TokenMongoRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {

	opts := options.Find()
	opts.SetLimit(1)
	opts.SetSort(bson.M{"block_number": -1})

	res := []*persist.Token{}

	if err := t.tokensStorage.find(pCtx, bson.M{}, &res, opts); err != nil {
		return 0, err
	}

	if len(res) < 1 {
		return 0, errNoTokensFound
	}

	return res[0].BlockNumber, nil

}

// Count will find the most recent block stored for all tokens
func (t *TokenMongoRepository) Count(pCtx context.Context, countType persist.TokenCountType) (int64, error) {

	filter := bson.M{}

	switch countType {
	case persist.CountTypeNoMetadata:
		filter = bson.M{"token_metadata": nil}
	case persist.CountTypeERC721:
		filter = bson.M{"token_type": persist.TokenTypeERC721}
	case persist.CountTypeERC1155:
		filter = bson.M{"token_type": persist.TokenTypeERC1155}
	}
	count, err := t.tokensStorage.count(pCtx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil

}
