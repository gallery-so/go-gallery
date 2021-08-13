package persist

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const galleryColName = "galleries"

// GalleryDB represents a group of collections of NFTs in the database.
// Collections of NFTs will be represented as a list of collection IDs creating
// a join relationship in the database
// This struct will only be used in database operations
type GalleryDB struct {
	Version      int64   `bson:"version"       json:"version"` // schema version for this model
	ID           DBID    `bson:"_id"           json:"id" binding:"required"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"`

	OwnerUserID DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Collections []DBID `bson:"collections"          json:"collections"`
}

// Gallery represents a group of collections of NFTS in the application.
// Collections are represented as structs instead of IDs
// This struct will be decoded from a find database operation and used throughout
// the application where GalleryDB is not used
type Gallery struct {
	Version      int64   `bson:"version"       json:"version"` // schema version for this model
	ID           DBID    `bson:"_id"           json:"id" binding:"required"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"`

	OwnerUserID DBID          `bson:"owner_user_id" json:"owner_user_id"`
	Collections []*Collection `bson:"collections"          json:"collections"`
}

// GalleryUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryUpdateInput struct {
	Collections []DBID `bson:"collections" json:"collections"`
}

// GalleryCreate inserts a new gallery into the database and returns the ID of the new gallery
func GalleryCreate(pCtx context.Context, pGallery *GalleryDB,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, galleryColName, pRuntime)

	return mp.insert(pCtx, pGallery)
}

// GalleryUpdate updates a gallery in the database by ID, also ensuring the gallery
// is owned by a given authorized user.
// pUpdate is a struct that contains bson tags representing the fields to be updated
func GalleryUpdate(pCtx context.Context, pIDstr DBID,
	pOwnerUserID DBID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, galleryColName, pRuntime)

	return mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pOwnerUserID}, pUpdate)
}

// GalleryAddCollections adds collections to the specified gallery
func GalleryAddCollections(pCtx context.Context, pID DBID, pUserID DBID, pCollectionIDs []DBID, pRuntime *runtime.Runtime) error {
	mp := newStorage(0, galleryColName, pRuntime)

	return mp.Push(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, "collections", pCollectionIDs)
}

// GalleryGetByUserID gets a gallery by its owner user ID and will variably return
// hidden collections depending on the auth status of the caller
func GalleryGetByUserID(pCtx context.Context, pUserID DBID, pAuth bool,
	pRuntime *runtime.Runtime) ([]*Gallery, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, galleryColName, pRuntime)

	result := []*Gallery{}

	if err := mp.aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserID, "deleted": false}, pAuth), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// GalleryGetByID gets a gallery by its ID and will variably return
// hidden collections depending on the auth status of the caller
func GalleryGetByID(pCtx context.Context, pID DBID, pAuth bool,
	pRuntime *runtime.Runtime) ([]*Gallery, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, galleryColName, pRuntime)

	result := []*Gallery{}

	if err := mp.aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pID, "deleted": false}, pAuth), &result, opts); err != nil {
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
	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from":     "collections",
			"let":      bson.M{"childArray": "$collections"},
			"pipeline": newCollectionPipeline(innerMatch),
			"as":       "collections",
		}}},
	}
}
