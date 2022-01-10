package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const galleryColName = "galleries"

var errAllCollectionsNotAccountedFor = errors.New("all collections not accounted for")

// GalleryTokenRepository is a repository that stores collections in a MongoDB database
type GalleryTokenRepository struct {
	galleriesStorage   *storage
	collectionsStorage *storage
	galleriesCache     memstore.Cache
	updateCacheQueue   *memstore.UpdateQueue
}

type errUserDoesNotOwnCollections struct {
	userID persist.DBID
}

// NewGalleryTokenRepository creates a new instance of the collection mongo repository
func NewGalleryTokenRepository(mgoClient *mongo.Client, galleriesCache memstore.Cache) *GalleryTokenRepository {
	return &GalleryTokenRepository{
		galleriesStorage:   newStorage(mgoClient, 0, galleryDBName, galleryColName),
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		galleriesCache:     galleriesCache,
		updateCacheQueue:   memstore.NewUpdateQueue(galleriesCache),
	}
}

// Create inserts a new gallery into the database and returns the ID of the new gallery
func (g *GalleryTokenRepository) Create(pCtx context.Context, pGallery persist.GalleryTokenDB) (persist.DBID, error) {

	if pGallery.Collections == nil {
		pGallery.Collections = []persist.DBID{}
	}

	err := ensureCollsOwnedByUserToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}

	err = ensureAllCollsAccountedForToken(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
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
func (g *GalleryTokenRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pOwnerUserID persist.DBID,
	pUpdate persist.GalleryTokenUpdateInput,
) error {
	err := ensureCollsOwnedByUserToken(pCtx, g, pUpdate.Collections, pOwnerUserID)
	if err != nil {
		return err
	}

	err = ensureAllCollsAccountedForToken(pCtx, g, pUpdate.Collections, pOwnerUserID)
	if err != nil {
		return err
	}

	if err := g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}

	go g.resetCache(pCtx, pOwnerUserID)
	return nil
}

// UpdateUnsafe updates a gallery in the database by ID
// pUpdate is a struct that contains bson tags representing the fields to be updated
func (g *GalleryTokenRepository) UpdateUnsafe(pCtx context.Context, pIDstr persist.DBID,
	pUpdate persist.GalleryTokenUpdateInput,
) error {
	if err := g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}

	return nil
}

// AddCollections adds collections to the specified gallery
func (g *GalleryTokenRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollectionIDs []persist.DBID) error {
	if err := g.galleriesStorage.push(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, "collections", pCollectionIDs); err != nil {
		return err
	}
	go g.resetCache(pCtx, pUserID)
	return nil
}

// GetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.GalleryToken, error) {

	fromCache, err := g.galleriesCache.Get(pCtx, pUserID.String())
	if err == nil && len(fromCache) > 0 {
		galleries := []persist.GalleryToken{}
		err = json.Unmarshal(fromCache, &galleries)
		if err != nil {
			return nil, err
		}
		if len(galleries) > 0 {
			return galleries, nil
		}
	}
	return g.getByUserIDSkipCache(pCtx, pUserID)

}

func (g *GalleryTokenRepository) getByUserIDSkipCache(pCtx context.Context, pUserID persist.DBID) ([]persist.GalleryToken, error) {

	result := []persist.GalleryToken{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryTokenPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}), &result); err != nil {
		return nil, err
	}
	go func() {
		asJSON, err := json.Marshal(result)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal galleries to json")
			return
		}
		g.updateCacheQueue.QueueUpdate(pUserID.String(), asJSON, galleriesTTL)
	}()

	return result, nil
}

// GetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryTokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.GalleryToken, error) {

	result := []persist.GalleryToken{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryTokenPipeline(bson.M{"_id": pID, "deleted": false}), &result); err != nil {
		return persist.GalleryToken{}, err
	}

	if len(result) != 1 {
		return persist.GalleryToken{}, persist.ErrGalleryNotFoundByID{ID: pID}
	}

	return result[0], nil
}

func (g *GalleryTokenRepository) resetCache(pCtx context.Context, ownerUserID persist.DBID) error {
	_, err := g.getByUserIDSkipCache(pCtx, ownerUserID)
	if err != nil {
		return err
	}

	return nil
}

func newGalleryTokenPipeline(matchFilter bson.M) mongo.Pipeline {

	andExpr := []bson.M{
		{"$in": []string{"$_id", "$$childArray"}},
		{"$eq": []interface{}{"$deleted", false}},
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

func ensureCollsOwnedByUserToken(pCtx context.Context, g *GalleryTokenRepository, pColls []persist.DBID, pOwnerUserID persist.DBID) error {
	ct, err := g.collectionsStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pColls}, "owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pColls) {
		return errUserDoesNotOwnCollections{pOwnerUserID}
	}
	return nil
}

func ensureAllCollsAccountedForToken(pCtx context.Context, g *GalleryTokenRepository, pColls []persist.DBID, pOwnerUserID persist.DBID) error {
	ct, err := g.collectionsStorage.count(pCtx, bson.M{"owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pColls) {
		return errAllCollectionsNotAccountedFor
	}
	return nil
}
