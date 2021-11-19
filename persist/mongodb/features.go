package mongodb

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const featuresCollName = "features"

// FeaturesMongoRepository is a mongoDB repository for storing feature flags in the database
type FeaturesMongoRepository struct {
	mp *storage
}

// NewFeaturesMongoRepository returns a new instance of a feature flag repository
func NewFeaturesMongoRepository(mgoClient *mongo.Client) *FeaturesMongoRepository {
	featureStorage := newStorage(mgoClient, 0, galleryDBName, featuresCollName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	featureNameIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: 1},
			{Key: "deleted", Value: 1},
			{Key: "version", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err := featureStorage.createIndex(ctx, featureNameIndex)
	if err != nil {
		panic(err)
	}
	return &FeaturesMongoRepository{
		mp: featureStorage,
	}
}

// GetByTokenIdentifiers returns an feature by a given token identifiers
func (c *FeaturesMongoRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenIdentifiers persist.TokenIdentifiers) (*persist.FeatureFlag, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*persist.FeatureFlag{}
	err := c.mp.find(pCtx, bson.M{"token_identifiers": pTokenIdentifiers}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, persist.ErrFeatureNotFoundByTokenIdentifiers{TokenIdentifiers: pTokenIdentifiers}
	}

	return result[0], nil
}

// GetByName returns an feature by a given name
func (c *FeaturesMongoRepository) GetByName(pCtx context.Context, pName string) (*persist.FeatureFlag, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*persist.FeatureFlag{}
	err := c.mp.find(pCtx, bson.M{"name": pName}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, persist.ErrFeatureNotFoundByName{Name: pName}
	}

	return result[0], nil
}
