package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionColName = "collections"
)

// CollectionMongoRepository is a repository that stores collections in a MongoDB database
type CollectionMongoRepository struct {
	mp  *storage
	nmp *storage
}

// NewCollectionMongoRepository creates a new instance of the collection mongo repository
func NewCollectionMongoRepository() *CollectionMongoRepository {
	return &CollectionMongoRepository{
		mp:  newStorage(0, galleryDBName, collectionColName),
		nmp: newStorage(0, galleryDBName, nftColName),
	}
}

// Create inserts a single CollectionDB into the database and will return the ID of the inserted document
func (c *CollectionMongoRepository) Create(pCtx context.Context, pColl *persist.CollectionDB,
) (persist.DBID, error) {

	if pColl.OwnerUserID == "" {
		return "", errors.New("owner_user_id is required")
	}

	if pColl.Nfts == nil {
		pColl.Nfts = []persist.DBID{}
	} else {
		if err := c.mp.pullAll(pCtx, bson.M{"owner_user_id": pColl.OwnerUserID}, "nfts", pColl.Nfts); err != nil {
			if _, ok := err.(*DocumentNotFoundError); !ok {
				return "", err
			}
		}
	}

	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pColl.OwnerUserID)); err != nil {
		return "", err
	}

	return c.mp.insert(pCtx, pColl)

}

// GetByUserID will form an aggregation pipeline to get all collections owned by a user
// and variably show hidden collections depending on the auth status of the caller
func (c *CollectionMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID,
	pShowHidden bool,
) ([]*persist.Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Collection{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := c.mp.aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID will form an aggregation pipeline to get a single collection by ID and
// variably show hidden collections depending on the auth status of the caller
func (c *CollectionMongoRepository) GetByID(pCtx context.Context, pID persist.DBID,
	pShowHidden bool,
) ([]*persist.Collection, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Collection{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := c.mp.aggregate(pCtx, newCollectionPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// Update will update a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionMongoRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pUserID persist.DBID,
	pUpdate interface{},
) error {
	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}
	return c.mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate)
}

// UpdateNFTs will update a collections NFTs ensuring that the collection is owned
// by a given authorized user as well as that no other collection contains the NFTs
// being included in the updated collection. This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionMongoRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID,
	pUserID persist.DBID,
	pUpdate *persist.CollectionUpdateNftsInput,
) error {

	uRepo := NewUserMongoRepository()
	user, err := uRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return err
	}

	ct, err := c.nmp.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Nfts}, "owner_address": bson.M{"$in": user.Addresses}})
	if err != nil {
		return err
	}
	if int(ct) != len(pUpdate.Nfts) {
		return errors.New("not all nfts are owned by the user")
	}

	if err := c.mp.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
		if _, ok := err.(*DocumentNotFoundError); !ok {
			return err
		}
	}

	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}

	return c.mp.update(pCtx, bson.M{"_id": pID}, pUpdate)
}

// ClaimNFTs will remove all NFTs from anyone's collections EXCEPT the user who is claiming them
func (c *CollectionMongoRepository) ClaimNFTs(pCtx context.Context,
	pUserID persist.DBID,
	pWalletAddresses []string,
	pUpdate *persist.CollectionUpdateNftsInput,
) error {

	for i, addr := range pWalletAddresses {
		pWalletAddresses[i] = strings.ToLower(addr)
	}

	nftsToBeRemoved := []*persist.NftDB{}

	if err := c.nmp.find(pCtx, bson.M{"_id": bson.M{"$nin": pUpdate.Nfts}, "owner_address": bson.M{"$in": pWalletAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	idsToPull := make([]persist.DBID, len(nftsToBeRemoved))
	for i, nft := range nftsToBeRemoved {
		idsToPull[i] = nft.ID
	}

	if err := c.mp.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToPull); err != nil {
		if _, ok := err.(*DocumentNotFoundError); !ok {
			return err
		}
	}

	type update struct {
		OwnerAddress string `bson:"owner_address"`
	}

	if err := c.nmp.update(pCtx, bson.M{"_id": bson.M{"$in": idsToPull}}, &update{}); err != nil {
		if _, ok := err.(*DocumentNotFoundError); !ok {
			return err
		}
	}

	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}

	return nil
}

// RemoveNFTsOfAddresses will remove all NFTs from a user's collections that are associated with
// an array of addresses
func (c *CollectionMongoRepository) RemoveNFTsOfAddresses(pCtx context.Context,
	pUserID persist.DBID,
	pAddresses []string,
) error {

	for i, addr := range pAddresses {
		pAddresses[i] = strings.ToLower(addr)
	}

	nftsToBeRemoved := []*persist.NftDB{}

	if err := c.nmp.find(pCtx, bson.M{"owner_address": bson.M{"$in": pAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	idsToBePulled := make([]persist.DBID, len(nftsToBeRemoved))
	for i, nft := range nftsToBeRemoved {
		idsToBePulled[i] = nft.ID
	}

	if err := c.mp.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToBePulled); err != nil {
		return err
	}

	type update struct {
		OwnerAddress string `bson:"owner_address"`
	}

	if err := c.nmp.update(pCtx, bson.M{"_id": bson.M{"$in": idsToBePulled}}, &update{}); err != nil {
		if _, ok := err.(*DocumentNotFoundError); !ok {
			return err
		}
	}

	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}

	return nil
}

// Delete will delete a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
func (c *CollectionMongoRepository) Delete(pCtx context.Context, pIDstr persist.DBID,
	pUserID persist.DBID,
) error {

	update := &persist.CollectionUpdateDeletedInput{Deleted: true}

	if err := redis.Delete(pCtx, redis.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}

	return c.mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, update)
}

// GetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionMongoRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID, skipCache bool) (*persist.Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Collection{}

	if !skipCache {
		if cachedResult, err := redis.Get(pCtx, redis.CollUnassignedRDB, string(pUserID)); err == nil && cachedResult != "" {
			err = json.Unmarshal([]byte(cachedResult), &result)
			if err != nil {
				return nil, err
			}
			return result[0], nil
		}
	}

	countColls, err := c.mp.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return nil, err
	}

	if countColls == 0 {
		nRepo := NewNFTMongoRepository()
		nfts, err := nRepo.GetByUserID(pCtx, pUserID)
		if err != nil {
			return nil, err
		}
		collNfts := []*persist.CollectionNft{}
		for _, nft := range nfts {
			collNfts = append(collNfts, nftToCollectionNft(nft))
		}

		result = []*persist.Collection{{Nfts: collNfts}}
	} else {
		uRepo := NewUserMongoRepository()
		user, err := uRepo.GetByID(pCtx, pUserID)
		if err != nil {
			return nil, err
		}
		if err := c.mp.aggregate(pCtx, newUnassignedCollectionPipeline(pUserID, user.Addresses), &result, opts); err != nil {
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

	if err := redis.Set(pCtx, redis.CollUnassignedRDB, string(pUserID), string(toCache), collectionUnassignedTTL); err != nil {
		return nil, err
	}

	return result[0], nil

}

func newUnassignedCollectionPipeline(pUserID persist.DBID, pOwnerAddresses []string) mongo.Pipeline {
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
							{"$in": []interface{}{"$owner_address", pOwnerAddresses}},
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

func nftToCollectionNft(nft *persist.Nft) *persist.CollectionNft {
	return &persist.CollectionNft{
		ID:                nft.ID,
		Name:              nft.Name,
		CreationTime:      nft.CreationTime,
		ImageURL:          nft.ImageURL,
		ImageThumbnailURL: nft.ImageThumbnailURL,
		ImagePreviewURL:   nft.ImagePreviewURL,
		OwnerAddress:      nft.OwnerAddress,
		MultipleOwners:    nft.MultipleOwners,
	}
}
