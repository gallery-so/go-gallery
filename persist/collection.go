package persist

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionColName = "collections"
)

// CollectionDB is the struct that represents a collection of NFTs in the database
// CollectionDB will not store the NFTs by value but instead by ID creating a join relationship
// between collections and NFTS
// This struct will only be used when updating or querying the database
type CollectionDB struct {
	Version      int64              `bson:"version" json:"version"` // schema version for this model
	ID           DBID               `bson:"_id" json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at" json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	Name           string `bson:"name"          json:"name"`
	CollectorsNote string `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    DBID   `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []DBID `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// Collection represents a collection of NFTs in the application. Collection will contain
// the value of each NFT represented as a struct as opposed to just the ID of the NFT
// This struct will always be decoded from a get database operation and will be used throughout
// the application where CollectionDB does not apply
type Collection struct {
	Version      int64              `bson:"version"       json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"           json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at" json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Name           string           `bson:"name"          json:"name"`
	CollectorsNote string           `bson:"collectors_note"   json:"collectors_note"`
	OwnerUserID    string           `bson:"owner_user_id" json:"owner_user_id"`
	Nfts           []*CollectionNft `bson:"nfts"          json:"nfts"`

	// collections can be hidden from public-viewing
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionUpdateInfoInput represents the data that will be changed when updating a collection's metadata
type CollectionUpdateInfoInput struct {
	Name           string `bson:"name" json:"name"`
	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`
}

// CollectionUpdateNftsInput represents the data that will be changed when updating a collection's NFTs
type CollectionUpdateNftsInput struct {
	Nfts []DBID `bson:"nfts" json:"nfts"`
}

// CollectionUpdateHiddenInput represents the data that will be changed when updating a collection's hidden status
type CollectionUpdateHiddenInput struct {
	Hidden bool `bson:"hidden" json:"hidden"`
}

// CollectionUpdateDeletedInput represents the data that will be changed when updating a collection's deleted status
type CollectionUpdateDeletedInput struct {
	Deleted bool `bson:"deleted" json:"-"`
}

// CollCreate inserts a single CollectionDB into the database and will return the ID of the inserted document
func CollCreate(pCtx context.Context, pColl *CollectionDB,
	pRuntime *runtime.Runtime) (DBID, error) {

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime).withRedis(CollectionsUnassignedRDB, pRuntime)

	if pColl.OwnerUserID == "" {
		return "", errors.New("owner_user_id is required")
	}

	if pColl.Nfts == nil {
		pColl.Nfts = []DBID{}
	} else {
		if err := mp.pull(pCtx, bson.M{"owner_user_id": pColl.OwnerUserID}, "nfts", pColl.Nfts); err != nil {
			if _, ok := err.(*DocumentNotFoundError); !ok {
				return "", err
			}
		}
	}

	if err := mp.cacheDelete(pCtx, string(pColl.OwnerUserID)); err != nil {
		return "", err
	}

	return mp.insert(pCtx, pColl)

}

// CollGetByUserID will form an aggregation pipeline to get all collections owned by a user
// and variably show hidden collections depending on the auth status of the caller
func CollGetByUserID(pCtx context.Context, pUserID DBID,
	pShowHidden bool,
	pRuntime *runtime.Runtime) ([]*Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime)

	result := []*Collection{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := mp.aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// CollGetByID will form an aggregation pipeline to get a single collection by ID and
// variably show hidden collections depending on the auth status of the caller
func CollGetByID(pCtx context.Context, pID DBID,
	pShowHidden bool,
	pRuntime *runtime.Runtime) ([]*Collection, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime)

	result := []*Collection{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := mp.aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// CollUpdate will update a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
// pUpdate will be a struct with bson tags that represent the fields to be updated
func CollUpdate(pCtx context.Context, pIDstr DBID,
	pUserID DBID,
	pUpdate interface{},
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime).withRedis(CollectionsUnassignedRDB, pRuntime)

	if err := mp.cacheDelete(pCtx, string(pUserID)); err != nil {
		return err
	}
	return mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate)
}

// CollUpdateNFTs will update a collections NFTs ensuring that the collection is owned
// by a given authorized user as well as that no other collection contains the NFTs
// being included in the updated collection. This is to ensure that the NFTs are not
// being shared between collections.
func CollUpdateNFTs(pCtx context.Context, pIDstr DBID,
	pUserID DBID,
	pUpdate *CollectionUpdateNftsInput,
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime).withRedis(CollectionsUnassignedRDB, pRuntime)

	if err := mp.pull(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", pUpdate.Nfts); err != nil {
		if _, ok := err.(*DocumentNotFoundError); !ok {
			return err
		}
	}

	if err := mp.cacheDelete(pCtx, string(pUserID)); err != nil {
		return err
	}

	return mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate)
}

// CollDelete will delete a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
func CollDelete(pCtx context.Context, pIDstr DBID,
	pUserID DBID,
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime).withRedis(CollectionsUnassignedRDB, pRuntime)
	update := &CollectionUpdateDeletedInput{Deleted: true}

	if err := mp.cacheDelete(pCtx, string(pUserID)); err != nil {
		return err
	}

	return mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, update)
}

// CollGetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func CollGetUnassigned(pCtx context.Context, pUserID DBID, skipCache bool, pRuntime *runtime.Runtime) (*Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, runtime.GalleryDBName, collectionColName, pRuntime).withRedis(CollectionsUnassignedRDB, pRuntime)
	defer mp.cacheClose()

	result := []*Collection{}

	if !skipCache {
		if cachedResult, err := mp.cacheGet(pCtx, string(pUserID)); err == nil && cachedResult != "" {
			err = json.Unmarshal([]byte(cachedResult), &result)
			if err != nil {
				return nil, err
			}
			return result[0], nil
		}
	}

	countColls, err := mp.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return nil, err
	}

	if countColls == 0 {
		nfts, err := NftGetByUserID(pCtx, pUserID, pRuntime)
		if err != nil {
			return nil, err
		}
		collNfts := []*CollectionNft{}
		for _, nft := range nfts {
			collNfts = append(collNfts, nftToCollectionNft(nft))
		}

		result = []*Collection{{Nfts: collNfts}}
	} else {
		if err := mp.aggregate(pCtx, newUnassignedCollectionPipeline(pUserID), &result, opts); err != nil {
			return nil, err
		}
	}
	if len(result) != 1 {
		return nil, errors.New("multiple collections of unassigned nfts found")
	}

	toCache, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	if err := mp.cacheSet(pCtx, string(pUserID), string(toCache), collectionUnassignedTTL); err != nil {
		return nil, err
	}

	return result[0], nil

}

func newUnassignedCollectionPipeline(pUserID DBID) mongo.Pipeline {
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
							{"$eq": []interface{}{"$owner_user_id", pUserID}},
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
				{{Key: "$addFields", Value: bson.M{
					"sort": bson.M{
						"$indexOfArray": []string{"$$array", "$_id"},
					}},
				}},
				{{Key: "$sort", Value: bson.M{"sort": 1}}},
				{{Key: "$unset", Value: []string{"sort"}}},
			},
			"as": "nfts",
		}}},
	}
}

func nftToCollectionNft(nft *Nft) *CollectionNft {
	return &CollectionNft{
		ID:                nft.ID,
		Name:              nft.Name,
		CreationTime:      nft.CreationTime,
		ImageURL:          nft.ImageURL,
		ImageThumbnailURL: nft.ImageThumbnailURL,
		ImagePreviewURL:   nft.ImagePreviewURL,
		OwnerUserID:       nft.OwnerUserID,
	}
}
