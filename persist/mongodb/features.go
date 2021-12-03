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
	featuresStorage *storage
}

// NewFeaturesMongoRepository returns a new instance of a feature flag repository
func NewFeaturesMongoRepository(mgoClient *mongo.Client) *FeaturesMongoRepository {
	featureStorage := newStorage(mgoClient, 0, galleryDBName, featuresCollName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	featureNameIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err := featureStorage.createIndex(ctx, featureNameIndex)
	if err != nil {
		panic(err)
	}
	return &FeaturesMongoRepository{
		featuresStorage: featureStorage,
	}
}

// GetByRequiredTokens returns an feature by given token identifiers
func (c *FeaturesMongoRepository) GetByRequiredTokens(pCtx context.Context, pRequiredtokens map[persist.TokenIdentifiers]uint64) ([]*persist.FeatureFlag, error) {

	result := []*persist.FeatureFlag{}
	keys := make([]persist.TokenIdentifiers, len(pRequiredtokens))
	i := 0
	for k := range pRequiredtokens {
		keys[i] = k
	}
	err := c.featuresStorage.find(pCtx, bson.M{"required_token": bson.M{"$in": keys}}, &result)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, persist.ErrFeatureNotFoundByTokenIdentifiers{TokenIdentifiers: keys}
	}

	for i, feature := range result {
		if feature.RequiredAmount > pRequiredtokens[feature.RequiredToken] {
			result = append(result[:i], result[i+1:]...)
		}
	}

	return result, nil
}

// GetByName returns an feature by a given name
func (c *FeaturesMongoRepository) GetByName(pCtx context.Context, pName string) (*persist.FeatureFlag, error) {

	opts := options.Find()
	opts.SetSort(bson.M{"created_at": -1})
	opts.SetLimit(1)

	result := []*persist.FeatureFlag{}
	err := c.featuresStorage.find(pCtx, bson.M{"name": pName}, &result, opts)

	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, persist.ErrFeatureNotFoundByName{Name: pName}
	}

	return result[0], nil
}

// GetAll returns all features
func (c *FeaturesMongoRepository) GetAll(pCtx context.Context) ([]*persist.FeatureFlag, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.FeatureFlag{}
	err := c.featuresStorage.find(pCtx, bson.M{}, &result, opts)

	if err != nil {
		return nil, err
	}

	return result, nil
}
