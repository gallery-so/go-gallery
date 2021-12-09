package mongodb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const galleryColName = "galleries"

// GalleryTokenMongoRepository is a repository that stores collections in a MongoDB database
type GalleryTokenMongoRepository struct {
	galleriesStorage   *storage
	collectionsStorage *storage
	galleriesCache     memstore.Cache
	updateCacheQueue   *memstore.UpdateQueue
}

type errUserDoesNotOwnCollections struct {
	userID persist.DBID
}

// NewGalleryTokenMongoRepository creates a new instance of the collection mongo repository
func NewGalleryTokenMongoRepository(mgoClient *mongo.Client, galleriesCache memstore.Cache) *GalleryTokenMongoRepository {
	return &GalleryTokenMongoRepository{
		galleriesStorage:   newStorage(mgoClient, 0, galleryDBName, galleryColName),
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		galleriesCache:     galleriesCache,
		updateCacheQueue:   memstore.NewUpdateQueue(galleriesCache),
	}
}

// Create inserts a new gallery into the database and returns the ID of the new gallery
func (g *GalleryTokenMongoRepository) Create(pCtx context.Context, pGallery persist.GalleryTokenDB) (persist.DBID, error) {

	if pGallery.Collections == nil {
		pGallery.Collections = []persist.DBID{}
	}

	id, err := g.galleriesStorage.insert(pCtx, pGallery)
	if err != nil {
		return "", err
	}
	go g.resetCache(pCtx, pGallery.OwnerUserID)
	return id, nil
}

// Update updates a gallery in the database by ID, also ensuring the gallery
// is owned by a given authorized user.
// pUpdate is a struct that contains bson tags representing the fields to be updated
func (g *GalleryTokenMongoRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pOwnerUserID persist.DBID,
	pUpdate persist.GalleryTokenUpdateInput,
) error {
	ct, err := g.collectionsStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Collections}, "owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pUpdate.Collections) {
		return errUserDoesNotOwnCollections{pOwnerUserID}
	}

	if err := g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}

	go g.resetCache(pCtx, pOwnerUserID)
	return nil
}

// UpdateUnsafe updates a gallery in the database by ID
// pUpdate is a struct that contains bson tags representing the fields to be updated
func (g *GalleryTokenMongoRepository) UpdateUnsafe(pCtx context.Context, pIDstr persist.DBID,
	pUpdate persist.GalleryTokenUpdateInput,
) error {
	if err := g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}

	return nil
}

// AddCollections adds collections to the specified gallery
func (g *GalleryTokenMongoRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollectionIDs []persist.DBID) error {
	if err := g.galleriesStorage.push(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, "collections", pCollectionIDs); err != nil {
		return err
	}
	go g.resetCache(pCtx, pUserID)
	return nil
}

// GetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryTokenMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pAuth bool) ([]persist.GalleryToken, error) {

	galleries := []persist.GalleryToken{}

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

func (g *GalleryTokenMongoRepository) getByUserIDSkipCache(pCtx context.Context, pUserID persist.DBID, pAuth bool) ([]persist.GalleryToken, error) {

	result := []persist.GalleryToken{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryTokenPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}, pAuth), &result); err != nil {
		return nil, err
	}
	go func() {
		asJSON, err := json.Marshal(result)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal galleries to json")
			return
		}
		g.updateCacheQueue.QueueUpdate(fmt.Sprintf("%s-%t", pUserID, pAuth), asJSON, galleriesTTL)
	}()

	return result, nil
}

// GetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryTokenMongoRepository) GetByID(pCtx context.Context, pID persist.DBID, pAuth bool) (persist.GalleryToken, error) {

	result := []persist.GalleryToken{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryTokenPipeline(bson.M{"_id": pID, "deleted": false}, pAuth), &result); err != nil {
		return persist.GalleryToken{}, err
	}

	if len(result) != 1 {
		return persist.GalleryToken{}, persist.ErrGalleryNotFoundByID{ID: pID}
	}

	return result[0], nil
}

func (g *GalleryTokenMongoRepository) resetCache(pCtx context.Context, ownerUserID persist.DBID) error {
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

func newGalleryTokenPipeline(matchFilter bson.M, pAuth bool) mongo.Pipeline {

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
		newCollectionTokenPipeline(innerMatch),
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

func (e errUserDoesNotOwnCollections) Error() string {
	return fmt.Sprintf("user with ID %v does not own all collections to be inserted", e.userID)
}
