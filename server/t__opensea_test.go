package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestOpenseaSync_Success(t *testing.T) {
	assert := setupTest(t, 1)
	ctx := context.Background()

	mike := persist.User{UsernameIdempotent: "mikey", Username: "mikey", Addresses: []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}}

	mikeUserID, err := tc.repos.userRepository.Create(ctx, mike)
	assert.Nil(err)

	nft1 := persist.NFT{
		OwnerAddress: "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
		Name:         "poop",
		OpenseaID:    46062326,
	}
	nft2 := persist.NFT{
		OwnerAddress: "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
		Name:         "baby",
		OpenseaID:    46062320,
	}

	nft3 := persist.NFT{
		OwnerAddress: persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA")),
		Name:         "wow",
		OpenseaID:    46062322,
	}
	nft4 := persist.NFT{
		OwnerAddress: persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA")),
		Name:         "baby",
		OpenseaID:    61355517,
	}

	ids, err := tc.repos.nftRepository.CreateBulk(ctx, []persist.NFT{nft1, nft2, nft3, nft4})
	assert.Nil(err)

	nft1DB, err := tc.repos.nftRepository.GetByID(ctx, ids[0])
	assert.Nil(err)
	nft2DB, err := tc.repos.nftRepository.GetByID(ctx, ids[1])
	assert.Nil(err)
	nft3DB, err := tc.repos.nftRepository.GetByID(ctx, ids[2])
	assert.Nil(err)
	nft4DB, err := tc.repos.nftRepository.GetByID(ctx, ids[3])
	assert.Nil(err)

	mikeCollNFTs := []persist.DBID{}
	if nft1DB.OwnerAddress == mike.Addresses[0] {
		mikeCollNFTs = append(mikeCollNFTs, ids[0])
	}
	if nft2DB.OwnerAddress == mike.Addresses[0] {
		mikeCollNFTs = append(mikeCollNFTs, ids[1])
	}
	if nft3DB.OwnerAddress == mike.Addresses[0] {
		mikeCollNFTs = append(mikeCollNFTs, ids[2])
	}
	if nft4DB.OwnerAddress == mike.Addresses[0] {
		mikeCollNFTs = append(mikeCollNFTs, ids[3])
	}

	coll := persist.CollectionDB{OwnerUserID: mikeUserID, Name: "mikey-coll", NFTs: mikeCollNFTs}
	collID, err := tc.repos.collectionRepository.Create(ctx, coll)
	assert.Nil(err)

	err = opensea.UpdateAssetsForAcc(ctx, mikeUserID, []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}, tc.repos.nftRepository, tc.repos.userRepository, tc.repos.collectionRepository)
	assert.Nil(err)

	time.Sleep(time.Second * 3)

	mikeColl, err := tc.repos.collectionRepository.GetByID(ctx, collID, true)
	assert.Nil(err)
	assert.Len(mikeColl.NFTs, 1)

	mikeNFTs, err := tc.repos.nftRepository.GetByUserID(ctx, mikeUserID)
	assert.Nil(err)

	assert.Greater(len(mikeNFTs), 0)

}
