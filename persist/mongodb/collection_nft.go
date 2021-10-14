package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CollectionMongoRepository is a repository that stores collections in a MongoDB database
type CollectionMongoRepository struct {
	mp           *storage
	nmp          *storage
	nnmp         *storage
	redisClients *memstore.Clients
}

// NewCollectionMongoRepository creates a new instance of the collection mongo repository
func NewCollectionMongoRepository(mgoClient *mongo.Client, redisClients *memstore.Clients) *CollectionMongoRepository {
	return &CollectionMongoRepository{
		mp:           newStorage(mgoClient, 0, galleryDBName, collectionColName),
		nmp:          newStorage(mgoClient, 0, galleryDBName, nftColName),
		nnmp:         newStorage(mgoClient, 0, galleryDBName, usersCollName),
		redisClients: redisClients,
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

	if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pColl.OwnerUserID)); err != nil {
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
	if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err != nil {
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

	users := []*persist.User{}
	err := c.nnmp.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return fmt.Errorf("user not found")
	}

	ct, err := c.nmp.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Nfts}, "owner_address": bson.M{"$in": users[0].Addresses}})
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

	if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err != nil {
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
		logrus.Infof("claiming for address: %s", pWalletAddresses[i])
		pWalletAddresses[i] = strings.ToLower(addr)
	}

	nftsToBeRemoved := []*persist.NFTDB{}

	if err := c.nmp.find(pCtx, bson.M{"_id": bson.M{"$nin": pUpdate.Nfts}, "owner_address": bson.M{"$in": pWalletAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	for _, nft := range nftsToBeRemoved {
		logrus.Infof("removing nft: %+v", *nft)
	}

	if len(nftsToBeRemoved) > 0 {
		idsToPull := make([]persist.DBID, len(nftsToBeRemoved))
		for i, nft := range nftsToBeRemoved {
			idsToPull[i] = nft.ID
		}

		if err := c.mp.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToPull); err != nil {
			if _, ok := err.(*DocumentNotFoundError); !ok {
				return err
			}
		}

		if err := c.nmp.delete(pCtx, bson.M{"_id": bson.M{"$in": idsToPull}}); err != nil {
			return err
		}

		if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err != nil {
			return err
		}
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

	nftsToBeRemoved := []*persist.NFTDB{}

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

	if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err != nil {
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

	if err := c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err != nil {
		return err
	}

	return c.mp.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, update)
}

// GetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionMongoRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID) (*persist.Collection, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Collection{}

	if cachedResult, err := c.redisClients.Get(pCtx, memstore.CollUnassignedRDB, string(pUserID)); err == nil && cachedResult != "" {
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			return nil, err
		}
		return result[0], nil
	}

	countColls, err := c.mp.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return nil, err
	}

	users := []*persist.User{}
	err = c.nnmp.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, fmt.Errorf("user not found")
	}

	if countColls == 0 {
		nfts := []*persist.NFT{}
		err = c.nmp.find(pCtx, bson.M{"owner_address": bson.M{"$in": users[0].Addresses}}, &nfts)
		if err != nil {
			return nil, err
		}
		collNfts := []*persist.CollectionNFT{}
		for _, nft := range nfts {
			collNfts = append(collNfts, nftToCollectionNft(nft))
		}

		result = []*persist.Collection{{Nfts: collNfts}}
	} else {
		if err := c.mp.aggregate(pCtx, newUnassignedCollectionPipeline(pUserID, users[0].Addresses), &result, opts); err != nil {
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

	if err := c.redisClients.Set(pCtx, memstore.CollUnassignedRDB, string(pUserID), string(toCache), collectionUnassignedTTL); err != nil {
		return nil, err
	}

	return result[0], nil

}

// RefreshUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionMongoRepository) RefreshUnassigned(pCtx context.Context, pUserID persist.DBID) error {
	return c.redisClients.Delete(pCtx, memstore.CollUnassignedRDB, string(pUserID))
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

func nftToCollectionNft(nft *persist.NFT) *persist.CollectionNFT {
	return &persist.CollectionNFT{
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
