package mongodb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// GalleryRepository is a repository that stores collections in a MongoDB database
type GalleryRepository struct {
	galleriesStorage   *storage
	collectionsStorage *storage
	galleriesCache     memstore.Cache
	cacheUpdateQueue   *memstore.UpdateQueue
}

var errNoUserIDProvided = errors.New("no user ID provided")

// NewGalleryRepository creates a new instance of the collection mongo repository
func NewGalleryRepository(mgoClient *mongo.Client, galleriesCache memstore.Cache) *GalleryRepository {
	return &GalleryRepository{
		galleriesStorage:   newStorage(mgoClient, 0, galleryDBName, galleryColName),
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		galleriesCache:     galleriesCache,
		cacheUpdateQueue:   memstore.NewUpdateQueue(galleriesCache),
	}
}

// Create inserts a new gallery into the database and returns the ID of the new gallery
func (g *GalleryRepository) Create(pCtx context.Context, pGallery persist.GalleryDB) (persist.DBID, error) {

	if pGallery.Collections == nil {
		pGallery.Collections = []persist.DBID{}
	}

	err := ensureCollsOwnedByUser(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}

	colls, err := ensureAllCollsAccountedFor(pCtx, g, pGallery.Collections, pGallery.OwnerUserID)
	if err != nil {
		return "", err
	}
	pGallery.Collections = colls

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
func (g *GalleryRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pOwnerUserID persist.DBID,
	pUpdate persist.GalleryUpdateInput,
) error {

	err := ensureCollsOwnedByUser(pCtx, g, pUpdate.Collections, pOwnerUserID)
	if err != nil {
		return err
	}

	colls, err := ensureAllCollsAccountedFor(pCtx, g, pUpdate.Collections, pOwnerUserID)
	if err != nil {
		return err
	}

	pUpdate.Collections = colls

	if err = g.galleriesStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}
	go g.resetCache(pCtx, pOwnerUserID)
	return nil
}

// AddCollections adds collections to the specified gallery
func (g *GalleryRepository) AddCollections(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pCollectionIDs []persist.DBID) error {

	err := ensureCollsOwnedByUser(pCtx, g, pCollectionIDs, pUserID)
	if err != nil {
		return err
	}

	gallery := persist.GalleryTokenDB{}
	if err := g.galleriesStorage.find(pCtx, bson.M{"_id": pID}, &gallery); err != nil {
		return err
	}

	total := append(gallery.Collections, pCollectionIDs...)

	colls, err := ensureAllCollsAccountedFor(pCtx, g, total, pUserID)
	if err != nil {
		return err
	}
	up := persist.GalleryUpdateInput{
		Collections: colls,
	}

	if err := g.Update(pCtx, pID, pUserID, up); err != nil {
		return err
	}

	go g.resetCache(pCtx, pUserID)
	return nil
}

// GetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.Gallery, error) {

	fromCache, err := g.galleriesCache.Get(pCtx, pUserID.String())
	if err == nil && len(fromCache) > 0 {
		logrus.Info("gallery cache hit")
		galleries := []persist.Gallery{}
		err = json.Unmarshal(fromCache, &galleries)
		if err != nil {
			return nil, err
		}
		if len(galleries) > 0 {
			return galleries, nil
		}
	}

	logrus.Info("gallery cache miss")

	return g.getByUserIDSkipCache(pCtx, pUserID)
}

func (g *GalleryRepository) getByUserIDSkipCache(pCtx context.Context, pUserID persist.DBID) ([]persist.Gallery, error) {

	result := []persist.Gallery{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}), &result); err != nil {
		return nil, err
	}

	go func() {
		asJSON, err := json.Marshal(result)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal galleries to json")
			return
		}
		g.cacheUpdateQueue.QueueUpdate(pUserID.String(), asJSON, galleriesTTL)
	}()

	return result, nil
}

// GetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func (g *GalleryRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Gallery, error) {

	result := []persist.Gallery{}

	if err := g.galleriesStorage.aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pID, "deleted": false}), &result); err != nil {
		return persist.Gallery{}, err
	}

	if len(result) != 1 {
		return persist.Gallery{}, persist.ErrGalleryNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// RefreshCache deletes what is in the cache for a given user
func (g *GalleryRepository) RefreshCache(pCtx context.Context, pUserID persist.DBID) error {
	return g.galleriesCache.Delete(pCtx, pUserID.String())
}

func (g *GalleryRepository) resetCache(pCtx context.Context, ownerUserID persist.DBID) error {
	_, err := g.getByUserIDSkipCache(pCtx, ownerUserID)
	if err != nil {
		return err
	}

	return nil
}

func newGalleryPipeline(matchFilter bson.M) mongo.Pipeline {

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

func ensureCollsOwnedByUser(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pOwnerUserID persist.DBID) error {
	ct, err := g.collectionsStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pColls}, "owner_user_id": pOwnerUserID})
	if err != nil {
		return err
	}

	if int(ct) != len(pColls) {
		return errUserDoesNotOwnCollections{pOwnerUserID}
	}
	return nil
}

func ensureAllCollsAccountedFor(pCtx context.Context, g *GalleryRepository, pColls []persist.DBID, pUserID persist.DBID) ([]persist.DBID, error) {
	ct, err := g.collectionsStorage.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return nil, err
	}

	if int(ct) != len(pColls) {
		if int64(len(pColls)) < ct {
			return addUnaccountedForCollections(pCtx, g, pUserID, pColls)
		}
		return nil, errUserDoesNotOwnCollections{pUserID}
	}
	return pColls, nil
}

func addUnaccountedForCollections(pCtx context.Context, g *GalleryRepository, pUserID persist.DBID, pColls []persist.DBID) ([]persist.DBID, error) {
	colls := make([]persist.CollectionTokenDB, 0, len(pColls)*2)
	err := g.collectionsStorage.find(pCtx, bson.M{"owner_user_id": pUserID}, &colls)
	if err != nil {
		return nil, err
	}

	ids := make([]persist.DBID, 0, len(pColls))
	for _, v := range colls {
		ids = append(ids, v.ID)
	}

	return appendDifference(pColls, ids), nil
}
