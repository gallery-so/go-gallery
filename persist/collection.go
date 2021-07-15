package persist

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionColName = "collections"

//-------------------------------------------------------------
type CollectionDb struct {
	VersionInt    int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id"           json:"id"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	NameStr           string `bson:"name,omitempty"          json:"name"`
	CollectorsNoteStr string `bson:"collectors_note,omitempty"   json:"collectors_note"`
	OwnerUserIDstr    string `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	NFTsLst           []DbId `bson:"nfts,omitempty"          json:"nfts"`

	// collections can be hidden from public-viewing
	HiddenBool bool `bson:"hidden,omitempty" json:"hidden"`
}

type Collection struct {
	VersionInt    int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id"           json:"id"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	NameStr           string `bson:"name,omitempty"          json:"name"`
	CollectorsNoteStr string `bson:"collectors_note,omitempty"   json:"collectors_note"`
	OwnerUserIDstr    string `bson:"owner_user_id,omitempty" json:"owner_user_id"`
	NFTsLst           []Nft  `bson:"nfts,omitempty"          json:"nfts"`

	// collections can be hidden from public-viewing
	HiddenBool bool `bson:"hidden,omitempty" json:"hidden"`
}

//-------------------------------------------------------------
func CollCreate(pColl *CollectionDb,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Insert(pCtx, pColl)

}

//-------------------------------------------------------------
func CollGetByUserID(pUserIDstr DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*Collection, error) {

	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	if err := mp.Aggregate(pCtx, newCollectionPipeline(bson.M{"owner_user_id": pUserIDstr}), result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func CollGetByID(pIDstr DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*Collection, error) {

	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	if err := mp.Aggregate(pCtx, newCollectionPipeline(bson.M{"_id": pIDstr}), result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

func newCollectionPipeline(matchFilter bson.M) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "glry_nfts",
			"foreignField": "_id",
			"localField":   "nfts",
			"as":           "nfts",
		}}},
		{{Key: "$unwind", Value: "$nfts"}},
	}
}
