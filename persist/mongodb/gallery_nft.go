package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GalleryMongoRepository is a repository that stores collections in a MongoDB database
type GalleryMongoRepository struct {
	galleriesStorage   *storage
	collectionsStorage *storage
	galleriesCache     memstore.Cache
}

var errNoUserIDProvided = errors.New("no user ID provided")

// NewGalleryMongoRepository creates a new instance of the collection mongo repository
func NewGalleryMongoRepository(mgoClient *mongo.Client, galleriesCache memstore.Cache) *GalleryMongoRepository {
	return &GalleryMongoRepository{
		galleriesStorage:   newStorage(mgoClient, 0, galleryDBName, galleryColName),
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		galleriesCache:     galleriesCache,
	}
}

// Create inserts a new gallery into the database and returns the ID of the new gallery
func (g *GalleryMongoRepository) Create(pCtx context.Context, pGallery *persist.GalleryDB) (persist.DBID, error) {

	if pGallery.Collections == nil {
		pGallery.Collections = []persist.DBID{}
	}

	id, err := g.galleriesStorage.insert(pCtx, pGallery)
	if err != nil {
		return "", err
	}

	go g.resetCache(pCtx, pGallery.OwnerUserID)

	return id, err
}

// Update updates a gallery in the database by ID, also ensuring the gallery
// is owned by a given authorized user.
// pUpdate is a struct that contains bson tags representing the fields to be updated
func (g *GalleryMongoRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pOwnerUserID persist.DBID,
	pUpdate *persist.GalleryUpdateInput,
) error {

	ct, err := g.collectionsStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Collections}, "owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pUpdate.Collections) {
		return errUserDoesNotOwnCollections{pOwnerUserID}
	}

	if err = g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}
	go g.resetCache(pCtx, pOwnerUserID)
	return nil
}

// AddCollections adds collections to the specified gallery
func (g *GalleryMongoRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollectionIDs []persist.DBID) error {
	if err := g.galleriesStorage.push(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, "collections", pCollectionIDs); err != nil {
		return err
	}
	go g.resetCache(pCtx, pUserID)
	return nil
}

// GetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pAuth bool,
) ([]*persist.Gallery, error) {

	galleries := []*persist.Gallery{}

	fromCache, err := g.galleriesCache.Get(pCtx, fmt.Sprintf("%s-%t", pUserID, pAuth))
	if err != nil || fromCache == nil || len(fromCache) == 0 {
		return g.getByUserIDSkipCache(pCtx, pUserID, pAuth)
	}

	err = json.Unmarshal(fromCache, &galleries)
	if err != nil {
		return nil, err
	}

	return galleries, nil
}

func (g *GalleryMongoRepository) getByUserIDSkipCache(pCtx context.Context, pUserID persist.DBID, pAuth bool) ([]*persist.Gallery, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Gallery{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}, pAuth), &result, opts); err != nil {
		return nil, err
	}

	go func() {
		asJSON, err := json.Marshal(result)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal galleries to json")
			return
		}
		g.galleriesCache.Set(pCtx, fmt.Sprintf("%s-%t", pUserID, pAuth), asJSON, time.Hour)
	}()

	return result, nil
}

// GetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryMongoRepository) GetByID(pCtx context.Context, pID persist.DBID, pAuth bool) (*persist.Gallery, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Gallery{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pID, "deleted": false}, pAuth), &result, opts); err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrGalleryNotFoundByID{ID: pID}
	}

	return result[0], nil
}

func (g *GalleryMongoRepository) resetCache(pCtx context.Context, ownerUserID persist.DBID) error {
	if ownerUserID == "" {
		return errNoUserIDProvided
	}
	_, err := g.getByUserIDSkipCache(pCtx, ownerUserID, true)
	if err != nil {
		return err
	}

	_, err = g.getByUserIDSkipCache(pCtx, ownerUserID, false)
	if err != nil {
		return err
	}

	return nil
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
