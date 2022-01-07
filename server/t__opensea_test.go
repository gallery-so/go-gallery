package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestOpenseaSync_Success(t *testing.T) {
	assert := setupTest(t, 1)
	ctx := context.Background()

	mike := persist.User{UsernameIdempotent: "mikey", Username: "mikey", Addresses: []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}}
	robin := persist.User{Username: "robin", UsernameIdempotent: "robin", Addresses: []persist.Address{persist.Address(strings.ToLower("0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"))}}

	robinUserID, err := tc.repos.userRepository.Create(ctx, robin)
	assert.Nil(err)

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

	ids, err := tc.repos.nftRepository.CreateBulk(ctx, []persist.NFT{nft1, nft2, nft3})
	assert.Nil(err)

	nft1DB, err := tc.repos.nftRepository.GetByID(ctx, ids[0])
	assert.Nil(err)
	nft2DB, err := tc.repos.nftRepository.GetByID(ctx, ids[1])
	assert.Nil(err)
	nft3DB, err := tc.repos.nftRepository.GetByID(ctx, ids[2])
	assert.Nil(err)

	logrus.Infof("nft1: %+v", nft1DB)
	logrus.Infof("nft2: %+v", nft2DB)
	logrus.Infof("nft3: %+v", nft3DB)

	coll := persist.CollectionDB{OwnerUserID: mikeUserID, Name: "mikey-coll", NFTs: []persist.DBID{nft3DB.ID, nft1DB.ID}}
	collID, err := tc.repos.collectionRepository.Create(ctx, coll)
	assert.Nil(err)

	robinOpenseaNFTs, err := opensea.PipelineAssetsForAcc(ctx, robinUserID, []persist.Address{"0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"}, tc.repos.nftRepository, tc.repos.userRepository, tc.repos.collectionRepository, tc.repos.historyRepository)
	assert.Nil(err)

	mikeOpenseaNFTs, err := opensea.PipelineAssetsForAcc(ctx, mikeUserID, []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}, tc.repos.nftRepository, tc.repos.userRepository, tc.repos.collectionRepository, tc.repos.historyRepository)
	assert.Nil(err)

	time.Sleep(time.Second * 3)

	mikeColl, err := tc.repos.collectionRepository.GetByID(ctx, collID, true)
	assert.Nil(err)
	assert.Len(mikeColl.NFTs, 1)

	robinNFTs, err := tc.repos.nftRepository.GetByUserID(ctx, robinUserID)
	assert.Nil(err)

	mikeNFTs, err := tc.repos.nftRepository.GetByUserID(ctx, mikeUserID)
	assert.Nil(err)

	ids1 := make([]int, len(robinOpenseaNFTs))
	ids2 := make([]int, len(robinNFTs))
	for i, nft := range robinOpenseaNFTs {
		ids1[i] = int(nft.OpenseaID.Int64())
	}
	for i, nft := range robinNFTs {
		ids2[i] = int(nft.OpenseaID.Int64())
	}

	// a function that finds the difference between two arrays
	arrayDiff := func(a, b []int) []int {
		mb := map[int]bool{}
		for _, x := range b {
			mb[x] = true
		}
		ab := []int{}
		for _, x := range a {
			if _, ok := mb[x]; !ok {
				ab = append(ab, x)
			}
		}
		return ab
	}

	log.Println("DIF", arrayDiff(ids1, ids2))

	assert.Len(robinOpenseaNFTs, len(robinNFTs))

	assert.Len(mikeOpenseaNFTs, len(mikeNFTs))

	assert.Greater(len(mikeNFTs), 0)

}

func openseaSyncRequest(assert *assert.Assertions, address persist.Address, jwt string) *http.Response {
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/nfts/opensea/refresh?addresses=%s", tc.serverURL, address),
		nil)
	assert.Nil(err)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwt))
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.Nil(err)
	return resp
}
