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

const (
	nftColName = "nfts"
)

var nftsTTL = time.Hour * 24

var errOwnerAddressRequired = errors.New("owner address required")

// NFTRepository is a repository that stores collections in a MongoDB database
type NFTRepository struct {
	nftsStorage          *storage
	usersStorage         *storage
	openseaCache         memstore.Cache
	nftsCache            memstore.Cache
	nftsCacheUpdateQueue *memstore.UpdateQueue
	galleryRepo          *GalleryRepository
}

// NewNFTRepository creates a new instance of the collection mongo repository
func NewNFTRepository(mgoClient *mongo.Client, nftscache, openseaCache memstore.Cache, galleryRepo *GalleryRepository) *NFTRepository {
	return &NFTRepository{
		nftsStorage:          newStorage(mgoClient, 0, galleryDBName, nftColName),
		usersStorage:         newStorage(mgoClient, 0, galleryDBName, usersCollName),
		nftsCache:            nftscache,
		nftsCacheUpdateQueue: memstore.NewUpdateQueue(nftscache),
		openseaCache:         openseaCache,
		galleryRepo:          galleryRepo,
	}
}

// CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func (n *NFTRepository) CreateBulk(pCtx context.Context, pNfts []persist.NFTDB) ([]persist.DBID, error) {

	nfts := make([]interface{}, len(pNfts))

	for i, v := range pNfts {
		nfts[i] = v
	}

	ids, err := n.nftsStorage.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}

	go func() {
		usersFound := map[persist.DBID]bool{}
		for _, v := range pNfts {
			if v.OwnerAddress != "" {
				users := []persist.User{}
				err := n.usersStorage.find(pCtx, bson.M{"addresses": bson.M{"$in": v.OwnerAddress}}, &users)
				if err != nil {
					continue
				}
				for _, u := range users {
					if u.ID != "" && !usersFound[u.ID] {
						usersFound[u.ID] = true
						n.resetCache(pCtx, u.ID)
					}
				}
			}
		}
	}()
	return ids, nil
}

// Create inserts an NFT into the database
func (n *NFTRepository) Create(pCtx context.Context, pNFT persist.NFTDB) (persist.DBID, error) {
	id, err := n.nftsStorage.insert(pCtx, pNFT)
	if err != nil {
		return "", err
	}
	go func() {
		if pNFT.OwnerAddress != "" {
			users := []persist.User{}
			err := n.usersStorage.find(pCtx, bson.M{"addresses": bson.M{"$in": pNFT.OwnerAddress}}, &users)
			if err != nil {
				return
			}
			for _, u := range users {
				if u.ID != "" {
					n.resetCache(pCtx, u.ID)
				}
			}
		}
	}()
	return id, nil
}

// GetByUserID finds an nft by its owner user id
func (n *NFTRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.NFT, error) {

	nftsJSON, err := n.nftsCache.Get(pCtx, pUserID.String())
	if err == nil && len(nftsJSON) > 0 {
		nfts := []persist.NFT{}
		err = json.Unmarshal(nftsJSON, &nfts)
		if err != nil {
			return nil, err
		}

		if len(nfts) > 0 {
			return nfts, nil
		}
	}

	return n.getByUserIDSkipCache(pCtx, pUserID)
}

// GetByUserID finds an nft by its owner user id
func (n *NFTRepository) getByUserIDSkipCache(pCtx context.Context, pUserID persist.DBID) ([]persist.NFT, error) {

	users := []persist.User{}
	err := n.usersStorage.find(pCtx, bson.M{"_id": pUserID}, &users)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, persist.ErrUserNotFoundByID{ID: pUserID}
	}

	nfts, err := n.GetByAddresses(pCtx, users[0].Addresses)
	if err != nil {
		return nil, err
	}
	go func() {
		asJSON, err := json.Marshal(nfts)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal nfts")
			return
		}
		n.nftsCacheUpdateQueue.QueueUpdate(pUserID.String(), asJSON, nftsTTL)
	}()
	return nfts, nil
}

// GetByAddresses finds an nft by its owner user id
func (n *NFTRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.Address) ([]persist.NFT, error) {
	for i, v := range pAddresses {
		pAddresses[i] = v
	}

	result := []persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"owner_address": bson.M{"$in": pAddresses}}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID finds an nft by its id
func (n *NFTRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.NFT, error) {

	result := []persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"_id": pID}, &result); err != nil {
		return persist.NFT{}, err
	}

	if len(result) != 1 {
		return persist.NFT{}, persist.ErrNFTNotFoundByID{ID: pID}
	}

	return result[0], nil
}

// GetByContractData finds an nft by its contract data
func (n *NFTRepository) GetByContractData(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address) ([]persist.NFT, error) {

	result := []persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"opensea_token_id": pTokenID, "contract.contract_address": pContractAddress}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetByOpenseaID finds an nft by its opensea ID
func (n *NFTRepository) GetByOpenseaID(pCtx context.Context, pOpenseaID persist.NullInt64, pWalletAddress persist.Address) ([]persist.NFT, error) {

	result := []persist.NFT{}

	if err := n.nftsStorage.find(pCtx, bson.M{"opensea_id": pOpenseaID, "owner_address": pWalletAddress}, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// UpdateByID updates an nft by its id, also ensuring that the NFT is owned
// by a given authorized user
// pUpdate is a struct that has bson tags representing the fields to be updated
func (n *NFTRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

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
	go n.resetCache(pCtx, pUserID)
	return nil
}

// BulkUpsert will create a bulk operation on the database to upsert many nfts for a given wallet address
// This function's primary purpose is to be used when syncing a user's NFTs from an external provider
func (n *NFTRepository) BulkUpsert(pCtx context.Context, pUserID persist.DBID, pNfts []persist.NFTDB) ([]persist.DBID, error) {

	ids := make(chan persist.DBID)
	errs := make(chan error)

	for _, v := range pNfts {
		go func(nft persist.NFTDB) {
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

	go n.resetCache(pCtx, pUserID)

	return result, nil

}

// OpenseaCacheSet adds a set of nfts to the opensea cache under a given set of wallet addresses as well as ensures
// that the nfts for user cache is most up to date
func (n *NFTRepository) OpenseaCacheSet(pCtx context.Context, pWalletAddresses []persist.Address, pNfts []persist.NFT) error {
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
func (n *NFTRepository) OpenseaCacheDelete(pCtx context.Context, pWalletAddresses []persist.Address) error {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	return n.openseaCache.Delete(pCtx, fmt.Sprint(pWalletAddresses))
}

// OpenseaCacheGet gets a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTRepository) OpenseaCacheGet(pCtx context.Context, pWalletAddresses []persist.Address) ([]persist.NFT, error) {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	result, err := n.openseaCache.Get(pCtx, fmt.Sprint(pWalletAddresses))
	if err != nil {
		return nil, err
	}

	nfts := []persist.NFT{}
	if err := json.Unmarshal([]byte(result), &nfts); err != nil {
		return nil, err
	}
	return nfts, nil
}

func (n *NFTRepository) resetCache(pCtx context.Context, pUserID persist.DBID) error {
	_, err := n.getByUserIDSkipCache(pCtx, pUserID)
	if err != nil {
		return err
	}
	return nil
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
