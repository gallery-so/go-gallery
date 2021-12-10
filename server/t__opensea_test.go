package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestOpenseaSync_Success(t *testing.T) {
	assert := setupTest(t, 1)
	ctx := context.Background()

	mike := persist.User{UserNameIdempotent: "mikey", UserName: "mikey", Addresses: []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}}
	robin := persist.User{UserName: "robin", UserNameIdempotent: "robin", Addresses: []persist.Address{persist.Address(strings.ToLower("0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"))}}
	gianna := persist.User{UserName: "gianna", UserNameIdempotent: "gianna", Addresses: []persist.Address{persist.Address(strings.ToLower("0xdd33e6fd03983c970ae5e647df07314435d69f6b"))}}

	robinUserID, err := tc.repos.userRepository.Create(ctx, robin)
	assert.Nil(err)
	giannaUserID, err := tc.repos.userRepository.Create(ctx, gianna)
	assert.Nil(err)
	mikeUserID, err := tc.repos.userRepository.Create(ctx, mike)
	assert.Nil(err)

	nft := persist.NFTDB{
		OwnerAddress: "0xdd33e6fd03983c970ae5e647df07314435d69f6b",
		Name:         "kks",
		OpenseaID:    34147626,
	}

	nft2 := persist.NFTDB{
		OwnerAddress: "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
		Name:         "malsjdlaksjd",
		OpenseaID:    46062326,
	}
	nft3 := persist.NFTDB{
		OwnerAddress: "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
		Name:         "asdjasdasd",
		OpenseaID:    46062320,
	}

	nft4 := persist.NFTDB{
		OwnerAddress: persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA")),
		Name:         "asdasdasd",
		OpenseaID:    46062322,
	}

	ids, err := tc.repos.nftRepository.CreateBulk(ctx, []persist.NFTDB{nft, nft2, nft3, nft4})
	assert.Nil(err)

	coll := persist.CollectionDB{OwnerUserID: mikeUserID, Name: "mikey-coll", Nfts: []persist.DBID{ids[3]}}
	collID, err := tc.repos.collectionRepository.Create(ctx, coll)
	assert.Nil(err)

	now, err := tc.repos.nftRepository.GetByUserID(ctx, giannaUserID)
	assert.Nil(err)
	assert.Len(now, 1)

	robinOpenseaNFTs, err := openSeaPipelineAssetsForAcc(ctx, robinUserID, []persist.Address{"0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"}, tc.repos.nftRepository, tc.repos.userRepository, tc.repos.collectionRepository, tc.repos.historyRepository)
	assert.Nil(err)

	mikeOpenseaNFTs, err := openSeaPipelineAssetsForAcc(ctx, mikeUserID, []persist.Address{persist.Address(strings.ToLower("0x27B0f73721DA882fAAe00B6e43512BD9eC74ECFA"))}, tc.repos.nftRepository, tc.repos.userRepository, tc.repos.collectionRepository, tc.repos.historyRepository)
	assert.Nil(err)

	mikeColl, err := tc.repos.collectionRepository.GetByID(ctx, collID, true)
	assert.Nil(err)
	assert.Len(mikeColl.Nfts, 0)

	nftsByUser, err := tc.repos.nftRepository.GetByUserID(ctx, robinUserID)
	assert.Nil(err)

	nftsByUserThree, err := tc.repos.nftRepository.GetByUserID(ctx, mikeUserID)
	assert.Nil(err)

	ids1 := make([]int, len(robinOpenseaNFTs))
	ids2 := make([]int, len(nftsByUser))
	for i, nft := range robinOpenseaNFTs {
		ids1[i] = nft.OpenSeaID
	}
	for i, nft := range nftsByUser {
		ids2[i] = nft.OpenSeaID
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

	assert.Len(robinOpenseaNFTs, len(nftsByUser))

	assert.Len(mikeOpenseaNFTs, len(nftsByUserThree))

	assert.Greater(len(nftsByUserThree), 0)

}

func TestOpenseaRateLimit_Failure(t *testing.T) {
	assert := setupTest(t, 1)
	var resp *http.Response
	for i := 0; i < 100; i++ {
		resp = openseaSyncRequest(assert, tc.user1.address, tc.user1.jwt)
	}
	assertErrorResponse(assert, resp)
	type OpenseaSyncResp struct {
		getNftsOutput
		Error string `json:"error"`
	}
	output := &OpenseaSyncResp{}
	err := util.UnmarshallBody(output, resp.Body)
	assert.Nil(err)
	assert.NotEmpty(output.Error)
	assert.Equal(output.Error, "rate limited")
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
