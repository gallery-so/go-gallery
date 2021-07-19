package persist

import (
	"context"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionColName = "collections"
)

//-------------------------------------------------------------
type CollectionDb struct {
	VersionInt    int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id,omitempty"           json:"id" binding:"required"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	NameStr           string `bson:"name"          json:"name"`
	CollectorsNoteStr string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserIDstr    DbId   `bson:"owner_user_id" json:"owner_user_id"`
	NFTsLst           []DbId `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	HiddenBool bool `bson:"hidden" json:"hidden"`
}

type Collection struct {
	VersionInt    int64   `bson:"version"       json:"version"` // schema version for this model
	IDstr         DbId    `bson:"_id"           json:"id" binding:"required"`
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	NameStr           string `bson:"name"          json:"name"`
	CollectorsNoteStr string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserIDstr    string `bson:"owner_user_id" json:"owner_user_id"`
	NFTsLst           []*Nft `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	HiddenBool bool `bson:"hidden" json:"hidden"`
}

//-------------------------------------------------------------
func CollCreate(pColl *CollectionDb,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (DbId, error) {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	id, err := mp.Insert(pCtx, pColl)
	if err != nil {
		return "", err
	}

	return id, nil

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

	if err := mp.Aggregate(pCtx, newCollectionPipeline(bson.M{"owner_user_id": pUserIDstr}), &result, opts); err != nil {
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

	if err := mp.Aggregate(pCtx, newCollectionPipeline(bson.M{"_id": pIDstr}), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func CollUpdate(pIDstr DbId,
	pUserId DbId,
	pColl *Collection,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserId}, bson.M{"$set": pColl})
}

//-------------------------------------------------------------

// returns a collection that is empty except for a list of nfts
func CollGetUnassigned(pUserId DbId, pCtx context.Context, pRuntime *runtime.Runtime) (*Collection, error) {

	opts := &options.AggregateOptions{}
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.MaxTime = &dur
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	if err := mp.Aggregate(pCtx, newUnassignedCollectionPipeline(pUserId), &result, opts); err != nil {
		return nil, err
	}
	if len(result) != 1 {
		return nil, errors.New("multiple collections of unassigned nfts found")
	}

	return result[0], nil

}

func newUnassignedCollectionPipeline(pUserId DbId) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"owner_user_id": pUserId}}},
		{{Key: "$group", Value: bson.M{"_id": "unassigned", "nfts": bson.M{"$addToSet": "$nfts"}}}},
		{{Key: "$project", Value: bson.M{
			"nfts": bson.M{
				"$reduce": bson.M{
					"input":        "$nfts",
					"initialValue": []string{},
					"in": bson.M{
						"$setUnion": []string{"$$value", "$$this"},
					},
				},
			},
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from": "nfts",
			"let":  bson.M{"array": "$nfts"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$not": bson.M{"$in": []string{"$_id", "$$array"}},
					},
				}}},
			},
			"as": "nfts",
		}}},
	}

}

func newCollectionPipeline(matchFilter bson.M) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "nfts",
			"foreignField": "_id",
			"localField":   "nfts",
			"as":           "nfts",
		}}},
	}
}
