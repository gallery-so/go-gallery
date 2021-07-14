package glry_db

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/glry_core"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const galleryColName = "glry_galleries"

//-------------------------------------------------------------
type GLRYgalleryID string
type GLRYgalleryStorage struct {
	VersionInt    int64      `bson:"version"       json:"version"` // schema version for this model
	IDstr         GLRYcollID `bson:"_id"           json:"id"`
	CreationTimeF float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool       `bson:"deleted"`

	OwnerUserIDstr string   `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	CollectionsLst []string `bson:"collections,omitempty"          json:"collections"`
}

type GLRYgallery struct {
	VersionInt    int64      `bson:"version"       json:"version"` // schema version for this model
	IDstr         GLRYcollID `bson:"_id"           json:"id"`
	CreationTimeF float64    `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool       `bson:"deleted"`

	OwnerUserIDstr string           `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	CollectionsLst []GLRYcollection `bson:"collections,omitempty"          json:"collections"`
}

//-------------------------------------------------------------
func GalleryCreate(pGallery *GLRYgalleryStorage,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) error {

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	return mp.Insert(pCtx, pGallery)

}

//-------------------------------------------------------------
func GalleryGetByUserID(pUserIDstr GLRYuserID,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYgallery, error) {

	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	result := []*GLRYgallery{}

	if err := mp.Aggregate(pCtx, newGalleryPipeline(bson.M{"owner_user_id": pUserIDstr}), result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func GalleryGetByID(pIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) ([]*GLRYgallery, error) {
	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := glry_core.NewMongoPersister(0, collectionColName, pRuntime)

	result := []*GLRYgallery{}

	if err := mp.Aggregate(pCtx, newGalleryPipeline(bson.M{"_id": pIDstr}), result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func newGalleryPipeline(matchFilter bson.M) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from": "glry_collections",
			"let":  bson.M{"childArray": "$collections"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$in": []string{"$_id", "$$childArray"},
					},
				}}},
				{{Key: "$lookup", Value: bson.M{
					"from":         "glry_nfts",
					"foreignField": "_id",
					"localField":   "nfts",
					"as":           "nfts",
				}}},
				{{Key: "$unwind", Value: "$nfts"}},
			},
			"as": "children",
		}}},
		{{Key: "$unwind", Value: "$collections"}},
	}
}
