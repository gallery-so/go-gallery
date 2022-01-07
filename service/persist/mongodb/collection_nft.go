package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CollectionRepository is a repository that stores collections in a MongoDB database
type CollectionRepository struct {
	collectionsStorage *storage
	nftsStorage        *storage
	galleriesStorage   *storage
	usersStorage       *storage
	unassignedCache    memstore.Cache
	cacheUpdateQueue   *memstore.UpdateQueue
	galleryRepo        *GalleryRepository
	nftRepo            *NFTRepository
}

// NewCollectionRepository creates a new instance of the collection mongo repository
func NewCollectionRepository(mgoClient *mongo.Client, unassignedCache memstore.Cache, galleryRepo *GalleryRepository, nftRepo *NFTRepository) *CollectionRepository {
	return &CollectionRepository{
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		nftsStorage:        newStorage(mgoClient, 0, galleryDBName, nftColName),
		usersStorage:       newStorage(mgoClient, 0, galleryDBName, usersCollName),
		galleriesStorage:   newStorage(mgoClient, 0, galleryDBName, galleryColName),
		unassignedCache:    unassignedCache,
		cacheUpdateQueue:   memstore.NewUpdateQueue(unassignedCache),
		galleryRepo:        galleryRepo,
		nftRepo:            nftRepo,
	}
}

// Create inserts a single CollectionDB into the database and will return the ID of the inserted document
func (c *CollectionRepository) Create(pCtx context.Context, pColl persist.CollectionDB) (persist.DBID, error) {

	if pColl.OwnerUserID == "" {
		return "", errors.New("owner_user_id is required")
	}

	if pColl.NFTs == nil {
		pColl.NFTs = []persist.DBID{}
	} /* else {
		TODO this is to ensure that the NFTs are not being shared between collections

		if err := c.mp.pullAll(pCtx, bson.M{"owner_user_id": pColl.OwnerUserID}, "nfts", pColl.Nfts); err != nil {
			if err != ErrDocumentNotFound {
				return "", err
			}
		}
	}*/

	id, err := c.collectionsStorage.insert(pCtx, pColl)
	if err != nil {
		return "", err
	}

	go c.galleryRepo.resetCache(pCtx, pColl.OwnerUserID)
	go c.RefreshUnassigned(pCtx, pColl.OwnerUserID)
	return id, nil
}

// GetByUserID will form an aggregation pipeline to get all collections owned by a user
// and variably show hidden collections depending on the auth status of the caller
func (c *CollectionRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]persist.Collection, error) {

	result := []persist.Collection{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := c.collectionsStorage.aggregate(pCtx, newCollectionPipeline(fil), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID will form an aggregation pipeline to get a single collection by ID and
// variably show hidden collections depending on the auth status of the caller
func (c *CollectionRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (persist.Collection, error) {

	result := []persist.Collection{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := c.collectionsStorage.aggregate(pCtx, newCollectionPipeline(fil), &result); err != nil {
		return persist.Collection{}, err
	}

	if len(result) != 1 {
		return persist.Collection{}, persist.ErrCollectionNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// Update will update a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionRepository) Update(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, pUpdate); err != nil {
		return err
	}

	if err := c.galleriesStorage.update(pCtx, bson.M{"owner_user_id": pUserID, "collections": bson.M{"$in": []interface{}{pID}}}, bson.M{"last_updated": persist.LastUpdatedTime(time.Time{})}); err != nil {
		logrus.WithError(err).Error("failed to update last_updated time for galleries")
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// UpdateNFTs will update a collections NFTs ensuring that the collection is owned
// by a given authorized user as well as that no other collection contains the NFTs
// being included in the updated collection. This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionUpdateNftsInput) error {

	users := []*persist.User{}
	err := c.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return fmt.Errorf("user not found")
	}

	ct, err := c.nftsStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.NFTs}, "owner_address": bson.M{"$in": users[0].Addresses}})
	if err != nil {
		return err
	}
	if int(ct) != len(pUpdate.NFTs) {
		return errors.New("not all nfts are owned by the user")
	}

	// TODO this is to ensure that the NFTs are not being shared between collections
	// if err := c.mp.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
	// 	if err != ErrDocumentNotFound {
	// 		return err
	// 	}
	// }

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pID}, pUpdate); err != nil {
		return err
	}

	if err := c.galleriesStorage.update(pCtx, bson.M{"owner_user_id": pUserID, "collections": bson.M{"$in": []interface{}{pID}}}, bson.M{"last_updated": persist.LastUpdatedTime(time.Time{})}); err != nil {
		logrus.WithError(err).Error("failed to update last_updated time for galleries")
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// ClaimNFTs will remove all NFTs from anyone's collections EXCEPT the user who is claiming them
func (c *CollectionRepository) ClaimNFTs(pCtx context.Context, pUserID persist.DBID, pWalletAddresses []persist.Address, pUpdate persist.CollectionUpdateNftsInput) error {

	// What in the world was I thinking when I wrote these three lines of code...
	// for i, addr := range pWalletAddresses {
	// 	pWalletAddresses[i] = addr
	// }

	nftsToBeRemoved := []*persist.NFT{}

	if err := c.nftsStorage.find(pCtx, bson.M{"_id": bson.M{"$nin": pUpdate.NFTs}, "owner_address": bson.M{"$in": pWalletAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	if len(nftsToBeRemoved) > 0 {
		idsToPull := make([]persist.DBID, len(nftsToBeRemoved))
		for i, nft := range nftsToBeRemoved {
			idsToPull[i] = nft.ID
		}

		if err := c.collectionsStorage.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToPull); err != nil {
			if err != ErrDocumentNotFound {
				return err
			}
		}

		if err := c.nftsStorage.delete(pCtx, bson.M{"_id": bson.M{"$in": idsToPull}}); err != nil {
			return err
		}

	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	go c.nftRepo.resetCache(pCtx, pUserID)
	return nil
}

// RemoveNFTsOfAddresses will remove all NFTs from a user's collections that are associated with
// an array of addresses
func (c *CollectionRepository) RemoveNFTsOfAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.Address) error {

	// Once again :facepalm:
	// for i, addr := range pAddresses {
	// 	pAddresses[i] = addr
	// }

	nftsToBeRemoved := []*persist.NFT{}

	if err := c.nftsStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": pAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	idsToBePulled := make([]persist.DBID, len(nftsToBeRemoved))
	for i, nft := range nftsToBeRemoved {
		idsToBePulled[i] = nft.ID
	}

	if err := c.collectionsStorage.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToBePulled); err != nil {
		return err
	}

	if err := c.nftsStorage.delete(pCtx, bson.M{"_id": bson.M{"$in": idsToBePulled}}); err != nil {
		return err
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)

	return nil
}

// Delete will delete a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
func (c *CollectionRepository) Delete(pCtx context.Context, pID persist.DBID, pUserID persist.DBID) error {

	update := &persist.CollectionUpdateDeletedInput{Deleted: true}

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pID, "owner_user_id": pUserID}, update); err != nil {
		return err
	}

	if err := c.galleriesStorage.update(pCtx, bson.M{"owner_user_id": pUserID, "collections": bson.M{"$in": []interface{}{pID}}}, bson.M{"last_updated": persist.LastUpdatedTime(time.Time{})}); err != nil {
		logrus.WithError(err).Error("failed to update last_updated time for galleries")
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// GetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID) (persist.Collection, error) {

	result := []persist.Collection{}

	cachedResult, err := c.unassignedCache.Get(pCtx, pUserID.String())
	if err == nil && len(cachedResult) > 0 {
		err := json.Unmarshal(cachedResult, &result)
		if err != nil {
			return persist.Collection{}, err
		}
		if len(result) > 0 {
			return result[0], nil
		}
	}

	countColls, err := c.collectionsStorage.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return persist.Collection{}, err
	}

	users := []*persist.User{}
	err = c.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return persist.Collection{}, err
	}
	if len(users) != 1 {
		return persist.Collection{}, fmt.Errorf("user not found")
	}

	if countColls == 0 {
		nfts := []persist.NFT{}
		err = c.nftsStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": users[0].Addresses}}, &nfts)
		if err != nil {
			return persist.Collection{}, err
		}
		collNfts := []persist.CollectionNFT{}
		for _, nft := range nfts {
			collNfts = append(collNfts, nftToCollectionNft(nft))
		}

		result = []persist.Collection{{NFTs: collNfts}}
	} else {
		if err := c.collectionsStorage.aggregate(pCtx, newUnassignedCollectionPipeline(pUserID, users[0].Addresses), &result); err != nil {
			return persist.Collection{}, err
		}
	}

	toCache, err := json.Marshal(result)
	if err != nil {
		return persist.Collection{}, err
	}

	c.cacheUpdateQueue.QueueUpdate(pUserID.String(), toCache, collectionUnassignedTTL)

	if len(result) > 0 {
		return result[0], nil
	}
	return persist.Collection{}, errors.New("no nfts for user")
}

// RefreshUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionRepository) RefreshUnassigned(pCtx context.Context, pUserID persist.DBID) error {
	err := c.unassignedCache.Delete(pCtx, pUserID.String())
	if err != nil {
		return err
	}
	_, err = c.GetUnassigned(pCtx, pUserID)
	return err
}

func newUnassignedCollectionPipeline(pUserID persist.DBID, pOwnerAddresses []persist.Address) mongo.Pipeline {
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

func nftToCollectionNft(nft persist.NFT) persist.CollectionNFT {
	return persist.CollectionNFT{
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
