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
	Version      int64   `bson:"version"       json:"version"` // schema version for this model
	ID           DbID    `bson:"_id,omitempty"           json:"id" binding:"required"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"`

	Name           string `bson:"name"          json:"name"`
	CollectorsNote string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DbID   `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []DbID `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

type Collection struct {
	Version      int64   `bson:"version"       json:"version"` // schema version for this model
	ID           DbID    `bson:"_id,omitempty"           json:"id" binding:"required"`
	CreationTime float64 `bson:"creation_time" json:"creation_time"`
	Deleted      bool    `bson:"deleted"`

	Name           string `bson:"name"          json:"name"`
	CollectorsNote string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    string `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []*Nft `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

type CollectionUpdateInfoInput struct {
	Name           string `bson:"name" json:"name"`
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

type CollectionUpdateNftsInput struct {
	Nfts []DbID `bson:"nfts" json:"nfts"`
}

type CollectionUpdateHiddenInput struct {
	Hidden bool `bson:"hidden" json:"hidden"`
}

type CollectionUpdateCollectorsNoteInput struct {
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

//-------------------------------------------------------------
func CollCreate(pCtx context.Context, pColl *CollectionDb,
	pRuntime *runtime.Runtime) (DbID, error) {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Insert(pCtx, pColl)

}

//-------------------------------------------------------------
func CollGetByUserID(pCtx context.Context, pUserID DbID,
	pShowHidden bool,
	pRuntime *runtime.Runtime) ([]*Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := mp.Aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func CollGetByID(pCtx context.Context, pID DbID,
	pShowHidden bool,
	pRuntime *runtime.Runtime) ([]*Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := mp.Aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

//-------------------------------------------------------------
func CollUpdate(pCtx context.Context, pIDstr DbID,
	pUserID DbID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate)
}

//-------------------------------------------------------------
func CollDelete(pCtx context.Context, pIDstr DbID,
	pUserID DbID,
	pRuntime *runtime.Runtime) error {

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	return mp.Update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, bson.M{"$set": bson.M{"deleted": true}})
}

//-------------------------------------------------------------

// returns a collection that is empty except for a list of nfts
func CollGetUnassigned(pCtx context.Context, pUserID DbID, pRuntime *runtime.Runtime) (*Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := NewMongoStorage(0, collectionColName, pRuntime)

	result := []*Collection{}

	if err := mp.Aggregate(pCtx, newUnassignedCollectionPipeline(pUserID), &result, opts); err != nil {
		return nil, err
	}
	if len(result) != 1 {
		return nil, errors.New("multiple collections of unassigned nfts found")
	}

	return result[0], nil

}

func newUnassignedCollectionPipeline(pUserID DbID) mongo.Pipeline {
	return mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"owner_user_id": pUserID, "deleted": false}}},
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
						"$and": []bson.M{
							{"$not": bson.M{"$in": []string{"$_id", "$$array"}}},
							{"$eq": []interface{}{"$deleted", false}},
						},
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
			"from": "nfts",
			"let":  bson.M{"array": "$nfts"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$and": []bson.M{
							{"$in": []string{"$_id", "$$array"}},
							{"$eq": []interface{}{"$deleted", false}},
						},
					},
				}}},
			},
			"as": "nfts",
		}}},
	}
}
