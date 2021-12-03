package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	nftColName = "nfts"
)

var errOwnerAddressRequired = errors.New("owner address required")

// NFTMongoRepository is a repository that stores collections in a MongoDB database
type NFTMongoRepository struct {
	nftsStorage  *storage
	usersStorage *storage
	openseaCache memstore.Cache
	galleryRepo  *GalleryMongoRepository
}

// NewNFTMongoRepository creates a new instance of the collection mongo repository
func NewNFTMongoRepository(mgoClient *mongo.Client, openseaCache memstore.Cache, galleryRepo *GalleryMongoRepository) *NFTMongoRepository {
	return &NFTMongoRepository{
		nftsStorage:  newStorage(mgoClient, 0, galleryDBName, nftColName),
		usersStorage: newStorage(mgoClient, 0, galleryDBName, usersCollName),
		openseaCache: openseaCache,
		galleryRepo:  galleryRepo,
	}
}

// CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func (n *NFTMongoRepository) CreateBulk(pCtx context.Context, pNfts []*persist.NFTDB) ([]persist.DBID, error) {

	nfts := make([]interface{}, len(pNfts))

	for i, v := range pNfts {
		nfts[i] = v
	}

	ids, err := n.nftsStorage.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Create inserts an NFT into the database
func (n *NFTMongoRepository) Create(pCtx context.Context, pNFT *persist.NFTDB) (persist.DBID, error) {
	return n.nftsStorage.insert(pCtx, pNFT)
}

// GetByUserID finds an nft by its owner user id
func (n *NFTMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]*persist.NFT, error) {

	users := []*persist.User{}
	err := n.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, persist.ErrUserNotFoundByID{ID: pUserID}
	}

	return n.GetByAddresses(pCtx, users[0].Addresses)
}

// GetByAddresses finds an nft by its owner user id
func (n *NFTMongoRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.Address) ([]*persist.NFT, error) {
	for i, v := range pAddresses {
		pAddresses[i] = v
	}

	result := []*persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": pAddresses}}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID finds an nft by its id
func (n *NFTMongoRepository) GetByID(pCtx context.Context, pID persist.DBID) (*persist.NFT, error) {

	result := []*persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"_id": pID}, &result); err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, persist.ErrNFTNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// GetByContractData finds an nft by its contract data
func (n *NFTMongoRepository) GetByContractData(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address) ([]*persist.NFT, error) {

	result := []*persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"opensea_token_id": pTokenID, "contract.contract_address": pContractAddress}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByOpenseaID finds an nft by its opensea ID
func (n *NFTMongoRepository) GetByOpenseaID(pCtx context.Context, pOpenseaID int, pWalletAddress persist.Address,
) ([]*persist.NFT, error) {

	result := []*persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"opensea_id": pOpenseaID, "owner_address": pWalletAddress}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateByID updates an nft by its id, also ensuring that the NFT is owned
// by a given authorized user
// pUpdate is a struct that has bson tags representing the fields to be updated
func (n *NFTMongoRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	users := []*persist.User{}
	err := n.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return persist.ErrUserNotFoundByID{ID: pUserID}
	}

	if err := n.nftsStorage.update(pCtx, bson.M{"_id": pID, "owner_address": bson.M{"$in": users[0].Addresses}}, pUpdate); err != nil {
		return err
	}
	go n.galleryRepo.resetCache(pCtx, pUserID)
	return nil
}

// BulkUpsert will create a bulk operation on the database to upsert many nfts for a given wallet address
// This function's primary purpose is to be used when syncing a user's NFTs from an external provider
func (n *NFTMongoRepository) BulkUpsert(pCtx context.Context, pNfts []*persist.NFTDB) ([]persist.DBID, error) {

	ids := make(chan persist.DBID)
	errs := make(chan error)

	for _, v := range pNfts {
		go func(nft *persist.NFTDB) {
			if nft.OwnerAddress == "" {
				errs <- errOwnerAddressRequired
				return
			}
			id, err := n.nftsStorage.upsert(pCtx, bson.M{"opensea_id": nft.OpenseaID, "owner_address": nft.OwnerAddress}, nft)
			if err != nil {
				errs <- err
			}
			ids <- id
		}(v)
	}

	result := make([]persist.DBID, len(pNfts))
	for i := 0; i < len(pNfts); i++ {
		select {
		case id := <-ids:
			result[i] = id
		case err := <-errs:
			return nil, err
		}
	}

	go func() {
		processedUsers := make(map[persist.DBID]bool)
		for _, nft := range pNfts {
			if nft.OwnerAddress != "" {
				users := []*persist.User{}
				err := n.usersStorage.find(pCtx, bson.M{"addresses": bson.M{"$in": nft.OwnerAddress}}, &users)
				if err != nil {
					logrus.WithError(err).Error("failed to find users for nft")
					continue
				}
				for _, user := range users {
					if !processedUsers[user.ID] {
						n.galleryRepo.resetCache(pCtx, user.ID)
						processedUsers[user.ID] = true
					}
				}
			}
		}
	}()

	return result, nil

}

// OpenseaCacheSet adds a set of nfts to the opensea cache under a given set of wallet addresses
func (n *NFTMongoRepository) OpenseaCacheSet(pCtx context.Context, pWalletAddresses []persist.Address, pNfts []*persist.NFT) error {
	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	toCache, err := json.Marshal(pNfts)
	if err != nil {
		return err
	}

	return n.openseaCache.Set(pCtx, fmt.Sprint(pWalletAddresses), toCache, openseaAssetsTTL)
}

// OpenseaCacheDelete deletes a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTMongoRepository) OpenseaCacheDelete(pCtx context.Context, pWalletAddresses []persist.Address) error {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	return n.openseaCache.Delete(pCtx, fmt.Sprint(pWalletAddresses))
}

// OpenseaCacheGet gets a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTMongoRepository) OpenseaCacheGet(pCtx context.Context, pWalletAddresses []persist.Address) ([]*persist.NFT, error) {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	result, err := n.openseaCache.Get(pCtx, fmt.Sprint(pWalletAddresses))
	if err != nil {
		return nil, err
	}

	nfts := []*persist.NFT{}
	if err := json.Unmarshal([]byte(result), &nfts); err != nil {
		return nil, err
	}
	return nfts, nil
}

func findDifference(nfts []*persist.NFTDB, dbNfts []*persist.NFTDB) ([]persist.DBID, error) {
	currOpenseaIds := map[int]bool{}

	for _, v := range nfts {
		currOpenseaIds[v.OpenseaID] = true
	}

	diff := []persist.DBID{}
	for _, v := range dbNfts {
		if !currOpenseaIds[v.OpenseaID] {
			diff = append(diff, v.ID)
		}
	}

	return diff, nil
}

func newNFTPipeline(matchFilter bson.M) mongo.Pipeline {

	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "history",
			"localField":   "_id",
			"foreignField": "nft_id",
			"as":           "ownership_history",
		}}},
		{{Key: "$set", Value: bson.M{"ownership_history": bson.M{"$arrayElemAt": []interface{}{"$ownership_history", 0}}}}},
		// {{Key: "$lookup", Value: bson.M{
		// 	"from": "users",
		// 	"let":  bson.M{"owners": "$owner_addresses"},
		// 	"pipeline": mongo.Pipeline{
		// 		{{Key: "$match", Value: bson.M{
		// 			"$expr": bson.M{
		// 				"$in": []interface{}{bson.M{"$first": "$addresses"}, "$$owners"},
		// 			},
		// 		}}},
		// 	},
		// 	"as": "owner_users",
		// }}},
	}
}
