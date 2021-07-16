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

//-------------------------------------------------------------
type GalleryDb struct {
	VersionInt    int64   `bson:"version,omitempty"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id,omitempty"           json:"id"`
	CreationTimeF float64 `bson:"creation_time,omitempty" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted,omitempty"`

	OwnerUserIDstr string `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	CollectionsLst []DbId `bson:"collections,omitempty"          json:"collections"`
}

type Gallery struct {
	VersionInt    int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id"           json:"id"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	OwnerUserIDstr string        `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	CollectionsLst []*Collection `bson:"collections,omitempty"          json:"collections"`
}

//-------------------------------------------------------------
func GalleryCreate(pGallery *GalleryDb,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Insert(pCtx, pGallery)

}

//-------------------------------------------------------------
func GalleryGetByUserID(pUserIDstr DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*Gallery, error) {

	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Gallery{}

	if err := mp.Aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserIDstr}), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func GalleryGetByID(pIDstr DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*Gallery, error) {
	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Gallery{}

	if err := mp.Aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pIDstr}), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func newGalleryPipeline(matchFilter bson.M) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from": "collections",
			"let":  bson.M{"childArray": "$collections"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$in": []string{"$_id", "$$childArray"},
					},
				}}},
				{{Key: "$lookup", Value: bson.M{
					"from":         "nfts",
					"foreignField": "_id",
					"localField":   "nfts",
					"as":           "nfts",
				}}},
			},
			"as": "children",
		}}},
	}
}
