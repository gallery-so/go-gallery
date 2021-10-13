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

// GalleryMongoRepository is a repository that stores collections in a MongoDB database
type GalleryMongoRepository struct {
	mp  *storage
	nmp *storage
}

// NewGalleryMongoRepository creates a new instance of the collection mongo repository
func NewGalleryMongoRepository(mgoClient *mongo.Client) *GalleryMongoRepository {
	return &GalleryMongoRepository{
		mp:  newStorage(mgoClient, 0, galleryDBName, galleryColName),
		nmp: newStorage(mgoClient, 0, galleryDBName, collectionColName),
	}
}

// Create inserts a new gallery into the database and returns the ID of the new gallery
func (g *GalleryMongoRepository) Create(pCtx context.Context, pGallery *persist.GalleryDB,
) (persist.DBID, error) {

	if pGallery.Collections == nil {
		pGallery.Collections = []persist.DBID{}
	}

	return g.mp.insert(pCtx, pGallery)
}

// Update updates a gallery in the database by ID, also ensuring the gallery
// is owned by a given authorized user.
// pUpdate is a struct that contains bson tags representing the fields to be updated
func (g *GalleryMongoRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pOwnerUserID persist.DBID,
	pUpdate *persist.GalleryUpdateInput,
) error {

	ct, err := g.nmp.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Collections}, "owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pUpdate.Collections) {
		return errors.New("user does not own all collections to be inserted")
	}

	return g.mp.update(pCtx, bson.M{"_id": pIDstr}, pUpdate)
}

// AddCollections adds collections to the specified gallery
func (g *GalleryMongoRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollectionIDs []persist.DBID) error {
	return g.mp.push(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, "collections", pCollectionIDs)
}

// GetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pAuth bool,
) ([]*persist.Gallery, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Gallery{}

	if err := g.mp.aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}, pAuth), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryMongoRepository) GetByID(pCtx context.Context, pID persist.DBID, pAuth bool,
) ([]*persist.Gallery, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Gallery{}

	if err := g.mp.aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pID, "deleted": false}, pAuth), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func newGalleryPipeline(matchFilter bson.M, pAuth bool) mongo.Pipeline {

	andExpr := []bson.M{
		{"$in": []string{"$_id", "$$childArray"}},
		{"$eq": []interface{}{"$deleted", false}},
	}
	if !pAuth {
		andExpr = append(andExpr, bson.M{"$eq": []interface{}{"$hidden", false}})
	}

	innerMatch := bson.M{
		"$expr": bson.M{
			"$and": andExpr,
		},
	}

	collectionPipeline := append(
		newCollectionPipeline(innerMatch),
		bson.D{{Key: "$addFields", Value: bson.M{
			"sort": bson.M{
				"$indexOfArray": []string{"$$childArray", "$_id"},
			}},
		}},
		bson.D{{Key: "$sort", Value: bson.M{"sort": 1}}},
		bson.D{{Key: "$unset", Value: []string{"sort"}}},
	)

	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from":     "collections",
			"let":      bson.M{"childArray": "$collections"},
			"pipeline": collectionPipeline,
			"as":       "collections",
		}}},
	}
}
