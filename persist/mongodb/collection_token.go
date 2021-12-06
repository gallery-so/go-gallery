package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionColName = "collections"
)

var errUserIDRequired = errors.New("owner user id is required")

// CollectionTokenMongoRepository is a repository that stores collections in a MongoDB database
type CollectionTokenMongoRepository struct {
	collectionsStorage *storage
	tokensStorage      *storage
	usersStorage       *storage
	unassignedCache    memstore.Cache
}

type errNotAllNFTsOwnedByUser struct {
	userID persist.DBID
}

// NewCollectionTokenMongoRepository creates a new instance of the collection mongo repository
func NewCollectionTokenMongoRepository(mgoClient *mongo.Client, unassignedCache memstore.Cache) *CollectionTokenMongoRepository {
	return &CollectionTokenMongoRepository{
		collectionsStorage: newStorage(mgoClient, 0, galleryDBName, collectionColName),
		tokensStorage:      newStorage(mgoClient, 0, galleryDBName, tokenColName),
		usersStorage:       newStorage(mgoClient, 0, galleryDBName, usersCollName),
		unassignedCache:    unassignedCache,
	}
}

// Create inserts a single CollectionDB into the database and will return the ID of the inserted document
func (c *CollectionTokenMongoRepository) Create(pCtx context.Context, pColl *persist.CollectionTokenDB,
) (persist.DBID, error) {

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

	if err := c.unassignedCache.Delete(pCtx, string(pColl.OwnerUserID)); err != nil {
		return "", err
	}

	return c.collectionsStorage.insert(pCtx, pColl)

}

// GetByUserID will form an aggregation pipeline to get all collections owned by a user
// and variably show hidden collections depending on the auth status of the caller
func (c *CollectionTokenMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, pShowHidden bool) ([]*persist.CollectionToken, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.CollectionToken{}

	fil := bson.M{"owner_user_id": pUserID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}

	if err := c.collectionsStorage.aggregate(pCtx, newCollectionTokenPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID will form an aggregation pipeline to get a single collection by ID and
// variably show hidden collections depending on the auth status of the caller
func (c *CollectionTokenMongoRepository) GetByID(pCtx context.Context, pID persist.DBID, pShowHidden bool) (*persist.CollectionToken, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.CollectionToken{}

	fil := bson.M{"_id": pID, "deleted": false}
	if !pShowHidden {
		fil["hidden"] = false
	}
	if err := c.collectionsStorage.aggregate(pCtx, newCollectionTokenPipeline(fil), &result, opts); err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrCollectionNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// Update will update a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionTokenMongoRepository) Update(pCtx context.Context, pIDstr persist.DBID,
	pUserID persist.DBID,
	pUpdate interface{},
) error {
	if err := c.unassignedCache.Delete(pCtx, string(pUserID)); err != nil {
		return err
	}
	return c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, pUpdate)
}

// UpdateNFTs will update a collections NFTs ensuring that the collection is owned
// by a given authorized user as well as that no other collection contains the NFTs
// being included in the updated collection. This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionTokenMongoRepository) UpdateNFTs(pCtx context.Context, pID persist.DBID,
	pUserID persist.DBID,
	pUpdate *persist.CollectionTokenUpdateNftsInput,
) error {

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
	// if err := c.mp.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
	// 	if err != ErrDocumentNotFound {
	// 		return err
	// 	}
	// }
	if err := c.unassignedCache.Delete(pCtx, string(pUserID)); err != nil {
		return err
	}

	return c.collectionsStorage.update(pCtx, bson.M{"_id": pID}, pUpdate)
}

// UpdateUnsafe will update a single collection by ID
// pUpdate will be a struct with bson tags that represent the fields to be updated
func (c *CollectionTokenMongoRepository) UpdateUnsafe(pCtx context.Context, pIDstr persist.DBID,
	pUpdate interface{},
) error {

	return c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr}, pUpdate)
}

// UpdateNFTsUnsafe will update a collections NFTs ensuring that
// no other collection contains the NFTs being included in the updated collection.
// This is to ensure that the NFTs are not
// being shared between collections.
func (c *CollectionTokenMongoRepository) UpdateNFTsUnsafe(pCtx context.Context, pID persist.DBID,
	pUpdate *persist.CollectionTokenUpdateNftsInput,
) error {

	// TODO this is to ensure that the NFTs are not being shared between collections
	// if err := c.mp.pullAll(pCtx, bson.M{}, "nfts", pUpdate.Nfts); err != nil {
	// 	if err != ErrDocumentNotFound {
	// 		return err
	// 	}
	// }

	return c.collectionsStorage.update(pCtx, bson.M{"_id": pID}, pUpdate)
}

// ClaimNFTs will remove all NFTs from anyone's collections EXCEPT the user who is claiming them
func (c *CollectionTokenMongoRepository) ClaimNFTs(pCtx context.Context,
	pUserID persist.DBID,
	pWalletAddresses []persist.Address,
	pUpdate *persist.CollectionTokenUpdateNftsInput,
) error {

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

		if err := c.unassignedCache.Delete(pCtx, string(pUserID)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveNFTsOfAddresses will remove all NFTs from a user's collections that are associated with
// an array of addresses
func (c *CollectionTokenMongoRepository) RemoveNFTsOfAddresses(pCtx context.Context,
	pUserID persist.DBID,
	pAddresses []persist.Address,
) error {

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

	if err := c.unassignedCache.Delete(pCtx, string(pUserID)); err != nil {
		return err
	}

	return nil
}

// Delete will delete a single collection by ID, also ensuring that the collection is owned
// by a given authorized user.
func (c *CollectionTokenMongoRepository) Delete(pCtx context.Context, pIDstr persist.DBID,
	pUserID persist.DBID,
) error {

	update := &persist.CollectionTokenUpdateDeletedInput{Deleted: true}

	if err := c.unassignedCache.Delete(pCtx, string(pUserID)); err != nil {
		return err
	}

	return c.collectionsStorage.update(pCtx, bson.M{"_id": pIDstr, "owner_user_id": pUserID}, update)
}

// GetUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionTokenMongoRepository) GetUnassigned(pCtx context.Context, pUserID persist.DBID) (*persist.CollectionToken, error) {

	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.CollectionToken{}

	if cachedResult, err := c.unassignedCache.Get(pCtx, string(pUserID)); err == nil && string(cachedResult) != "" {
		err = json.Unmarshal([]byte(cachedResult), &result)
		if err != nil {
			return nil, err
		}
		return result[0], nil
	}

	countColls, err := c.collectionsStorage.count(pCtx, bson.M{"owner_user_id": pUserID})
	if err != nil {
		return nil, err
	}

	users := []*persist.User{}
	err = c.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, persist.ErrUserNotFoundByID{ID: pUserID}
	}

	if countColls == 0 {
		nfts := []*persist.Token{}
		err = c.tokensStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": users[0].Addresses}}, &nfts)
		if err != nil {
			return nil, err
		}
		collNfts := []*persist.TokenInCollection{}
		for _, nft := range nfts {
			collNfts = append(collNfts, tokenToCollectionToken(nft))
		}

		result = []*persist.CollectionToken{{Nfts: collNfts}}
	} else {
		if err := c.collectionsStorage.aggregate(pCtx, newUnassignedCollectionTokenPipeline(pUserID, users[0].Addresses), &result, opts); err != nil {
			return nil, err
		}
	}

	toCache, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	if err := c.unassignedCache.Set(pCtx, string(pUserID), string(toCache), collectionUnassignedTTL); err != nil {
		return nil, err
	}

	return result[0], nil

}

// RefreshUnassigned returns a collection that is empty except for a list of nfts that are not
// assigned to any collection
func (c *CollectionTokenMongoRepository) RefreshUnassigned(pCtx context.Context, pUserID persist.DBID) error {
	return c.unassignedCache.Delete(pCtx, string(pUserID))
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

func tokenToCollectionToken(nft *persist.Token) *persist.TokenInCollection {
	return &persist.TokenInCollection{
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
