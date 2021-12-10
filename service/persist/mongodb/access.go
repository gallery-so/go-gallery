package mongodb

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const accessCollName = "access"

// AccessMongoRepository is a mongoDB repository for storing access states of users
type AccessMongoRepository struct {
	accessStorage *storage
}

// NewAccessMongoRepository returns a new instance of a feature flag repository
func NewAccessMongoRepository(mgoClient *mongo.Client) *AccessMongoRepository {
	accessStorage := newStorage(mgoClient, 0, galleryDBName, accessCollName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	featureNameIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err := accessStorage.createIndex(ctx, featureNameIndex)
	if err != nil {
		panic(err)
	}
	return &AccessMongoRepository{
		accessStorage: accessStorage,
	}
}

// GetByUserID returns an feature by a given token identifiers
func (c *AccessMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) (persist.Access, error) {

	opts := options.Find()
	opts.SetLimit(1)

	result := []persist.Access{}
	err := c.accessStorage.find(pCtx, bson.M{"user_id": pUserID}, &result, opts)

	if err != nil {
		return persist.Access{}, err
	}

	if len(result) == 0 {
		return persist.Access{}, persist.ErrAccessNotFoundByUserID{UserID: pUserID}
	}

	return result[0], nil
}

// HasRequiredTokens returns whether a user has the required tokens
func (c *AccessMongoRepository) HasRequiredTokens(pCtx context.Context, pUserID persist.DBID, pTokenIdentifiers []persist.TokenIdentifiers) (bool, error) {

	opts := options.Find()

	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*persist.Access{}
	requiredTokensOwned := map[persist.TokenIdentifiers]bool{}
	for _, tokenIdentifiers := range pTokenIdentifiers {
		requiredTokensOwned[tokenIdentifiers] = true
	}
	err := c.accessStorage.find(pCtx, bson.M{"user_id": pUserID, "required_tokens_owned": requiredTokensOwned}, &result, opts)

	if err != nil {
		return false, err
	}

	return len(result) > 0, nil
}

// UpsertRequiredTokensByUserID upserts the required tokens owned by a user
func (c *AccessMongoRepository) UpsertRequiredTokensByUserID(pCtx context.Context, pUserID persist.DBID, pRequiredTokensOwned map[persist.TokenIdentifiers]uint64, pBlock persist.BlockNumber) error {

	if _, err := c.accessStorage.upsert(pCtx, bson.M{"user_id": pUserID}, bson.M{
		"required_tokens_owned": pRequiredTokensOwned,
		"most_recent_block":     pBlock,
	}); err != nil {
		return err
	}
	return nil
}
