package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	collectionColName = "collections"
)

var errUserIDRequired = errors.New("owner user id is required")
var errNoUnassignedNFTs = errors.New("no unassigned nfts")

// CollectionTokenRepository is a repository that stores collections in a MongoDB database
type CollectionTokenRepository struct {
	collectionsStorage *storage
	tokensStorage      *storage
	usersStorage       *storage
	unassignedCache    memstore.Cache
	cacheUpdateQueue   *memstore.UpdateQueue
	galleryRepo        *GalleryTokenRepository
}

type errNotAllNFTsOwnedByUser struct {
	userID persist.DBID
}

// NewCollectionTokenRepository creates a new instance of the collection mongo repository
func NewCollectionTokenRepository(mgoClient *mongo.Client, unassignedCache memstore.Cache, galleryRepo *GalleryTokenRepository) *CollectionTokenRepository {
	return &CollectionTokenRepository{
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		tokensStorage:      newStorage(mgoClient, 0, galleryDBName, tokenColName),
		usersStorage:       newStorage(mgoClient, 0, galleryDBName, usersCollName),
		unassignedCache:    unassignedCache,
		cacheUpdateQueue:   memstore.NewUpdateQueue(unassignedCache),
		galleryRepo:        galleryRepo,
	}
}

// Create inserts a single CollectionDB into the database and will return the ID of the inserted document
func (c *CollectionTokenRepository) Create(pCtx context.Context, pColl persist.CollectionTokenDB) (persist.DBID, error) {

	if pColl.OwnerUserID == "" {
		return "", errUserIDRequired
	}

	if pColl.Nfts == nil {
		pColl.Nfts = []persist.DBID{}
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
func (c *CollectionTokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]persist.CollectionToken, error) {

	result := []persist.CollectionToken{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := c.collectionsStorage.aggregate(pCtx, newCollectionTokenPipeline(fil), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID will form an aggregation pipeline to get a single collection by ID and
// variably show hidden collections depending on the auth status of the caller
func (c *CollectionTokenRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (persist.CollectionToken, error) {

	result := []persist.CollectionToken{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := c.collectionsStorage.aggregate(pCtx, newCollectionTokenPipeline(fil), &result); err != nil {
		return persist.CollectionToken{}, err
	}

	if len(result) != 1 {
		return persist.CollectionToken{}, persist.ErrCollectionNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// Update will update a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionTokenRepository) Update(pCtx context.Context, pIDstr persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate); err != nil {
		return err
	}
	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil

}

// UpdateNFTs will update a collections NFTs ensuring that the collection is owned
// by a given authorized user as well as that no other collection contains the NFTs
// being included in the updated collection. This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionTokenRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {

	users := []*persist.User{}
	err := c.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return persist.ErrUserNotFoundByID{ID: pUserID}
	}

	ct, err := c.tokensStorage.count(pCtx, bson.M{"_id": bson.M{"$in": pUpdate.Nfts}, "owner_address": bson.M{"$in": users[0].Addresses}})
	if err != nil {
		return err
	}
	if int(ct) != len(pUpdate.Nfts) {
		return errNotAllNFTsOwnedByUser{pUserID}
	}

	// TODO this is to ensure that the NFTs are not being shared between collections
	// if err := c.collectionsStorage.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
	// 	if err != ErrDocumentNotFound {
	// 		return err
	// 	}
	// }

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pID}, pUpdate); err != nil {
		return err
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// UpdateUnsafe will update a single collection by ID
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionTokenRepository) UpdateUnsafe(pCtx context.Context, pIDstr persist.DBID, pUpdate interface{}) error {

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate); err != nil {
		return err
	}

	return nil
}

// UpdateNFTsUnsafe will update a collections NFTs ensuring that
// no other collection contains the NFTs being included in the updated collection.
// This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionTokenRepository) UpdateNFTsUnsafe(pCtx context.Context, pID persist.DBID, pUpdate persist.CollectionTokenUpdateNftsInput) error {

	// TODO this is to ensure that the NFTs are not being shared between collections
	// if err := c.mp.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
	// 	if err != ErrDocumentNotFound {
	// 		return err
	// 	}
	// }

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pID}, pUpdate); err != nil {
		return err
	}

	return nil
}

// ClaimNFTs will remove all NFTs from anyone's collections EXCEPT the user who is claiming them
func (c *CollectionTokenRepository) ClaimNFTs(pCtx context.Context, pUserID persist.DBID, pWalletAddresses []persist.Address, pUpdate persist.CollectionTokenUpdateNftsInput) error {

	nftsToBeRemoved := []*persist.Token{}

	if err := c.tokensStorage.find(pCtx, bson.M{"_id": bson.M{"$nin": pUpdate.Nfts}, "owner_address": bson.M{"$in": pWalletAddresses}}, &nftsToBeRemoved); err != nil {
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

		if err := c.tokensStorage.delete(pCtx, bson.M{"_id": bson.M{"$in": idsToPull}}); err != nil {
			return err
		}

	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// RemoveNFTsOfAddresses will remove all NFTs from a user's collections that are associated with
// an array of addresses
func (c *CollectionTokenRepository) RemoveNFTsOfAddresses(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.Address) error {

	for i, address := range pAddresses {
		pAddresses[i] = address
	}

	nftsToBeRemoved := []*persist.Token{}

	if err := c.tokensStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": pAddresses}}, &nftsToBeRemoved); err != nil {
		return err
	}

	idsToBePulled := make([]persist.DBID, len(nftsToBeRemoved))
	for i, nft := range nftsToBeRemoved {
		idsToBePulled[i] = nft.ID
	}

	if err := c.collectionsStorage.pullAll(pCtx, bson.M{"owner_user_id": pUserID}, "nfts", idsToBePulled); err != nil {
		return err
	}

	if err := c.tokensStorage.delete(pCtx, bson.M{"_id": bson.M{"$in": idsToBePulled}}); err != nil {
		return err
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)

	return nil
}

// Delete will delete a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
func (c *CollectionTokenRepository) Delete(pCtx context.Context, pIDstr persist.DBID, pUserID persist.DBID) error {

	update := &persist.CollectionTokenUpdateDeletedInput{Deleted: true}

	if err := c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, update); err != nil {
		return err
	}

	go c.galleryRepo.resetCache(pCtx, pUserID)
	go c.RefreshUnassigned(pCtx, pUserID)
	return nil
}

// GetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionTokenRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID) (persist.CollectionToken, error) {

	result := []persist.CollectionToken{}

	cachedResult, err := c.unassignedCache.Get(pCtx, pUserID.String())
	if err == nil && len(cachedResult) > 0 {
		err := json.Unmarshal(cachedResult, &result)
		if err != nil {
			return persist.CollectionToken{}, err
		}
		if len(result) > 0 {
			return result[0], nil
		}
	}

	countColls, err := c.collectionsStorage.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return persist.CollectionToken{}, err
	}

	users := []*persist.User{}
	err = c.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return persist.CollectionToken{}, err
	}
	if len(users) != 1 {
		return persist.CollectionToken{}, persist.ErrUserNotFoundByID{ID: pUserID}
	}

	if countColls == 0 {
		nfts := []persist.Token{}
		err = c.tokensStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": users[0].Addresses}}, &nfts)
		if err != nil {
			return persist.CollectionToken{}, err
		}
		collNfts := []persist.TokenInCollection{}
		for _, nft := range nfts {
			collNfts = append(collNfts, tokenToCollectionToken(nft))
		}

		result = []persist.CollectionToken{{Nfts: collNfts}}
	} else {
		if err := c.collectionsStorage.aggregate(pCtx, newUnassignedCollectionTokenPipeline(pUserID, users[0].Addresses), &result); err != nil {
			return persist.CollectionToken{}, err
		}
	}

	toCache, err := json.Marshal(result)
	if err != nil {
		return persist.CollectionToken{}, err
	}

	c.cacheUpdateQueue.QueueUpdate(pUserID.String(), toCache, collectionUnassignedTTL)

	if len(result) > 0 {
		return result[0], nil
	}
	return persist.CollectionToken{}, errNoUnassignedNFTs

}

// RefreshUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionTokenRepository) RefreshUnassigned(pCtx context.Context, pUserID persist.DBID) error {
	err := c.unassignedCache.Delete(pCtx, pUserID.String())
	if err != nil {
		return err
	}
	_, err = c.GetUnassigned(pCtx, pUserID)
	return err
}

func newUnassignedCollectionTokenPipeline(pUserID persist.DBID, pOwnerAddresses []persist.Address) mongo.Pipeline {
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
			"from": "tokens",
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

func newCollectionTokenPipeline(matchFilter bson.M) mongo.Pipeline {

	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from": "tokens",
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

func tokenToCollectionToken(nft persist.Token) persist.TokenInCollection {
	return persist.TokenInCollection{
		ID:              nft.ID,
		Name:            nft.Name,
		CreationTime:    nft.CreationTime,
		ContractAddress: nft.ContractAddress,
		OwnerAddress:    nft.OwnerAddress,
		Chain:           nft.Chain,
		Description:     nft.Description,
		TokenType:       nft.TokenType,
		TokenURI:        nft.TokenURI,
		TokenID:         nft.TokenID,
		Media:           nft.Media,
		TokenMetadata:   nft.TokenMetadata,
	}
}

func (e errNotAllNFTsOwnedByUser) Error() string {
	return fmt.Sprintf("not all nfts owned by user: %s", e.userID)
}
