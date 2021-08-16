package server

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func TestFetchAssertsForAcc(pTest *testing.T) {

	ctx := context.Background()

	user := &persist.User{Addresses: []string{"0x485b8ac36535fae56b2910780245dd69dda270bc"}}

	userID, err := persist.UserCreate(ctx, user, tc.r)
	assert.Nil(pTest, err)

	nft := &persist.Nft{
		OwnerUserID:  userID,
		OwnerAddress: "0x485b8ac36535fae56b2910780245dd69dda270bc",
		Name:         "test",
		OpenSeaID:    0,
	}

	nft2 := &persist.Nft{
		OwnerUserID:  userID,
		OwnerAddress: "0x485b8ac36535fae56b2910780245dd69dda270bc",
		Name:         "test2",
		OpenSeaID:    32087758,
	}

	_, err = persist.NftCreateBulk(ctx, []*persist.Nft{nft, nft2}, tc.r)
	assert.Nil(pTest, err)

	nfts, err := openSeaPipelineAssetsForAcc(ctx, "0x485b8ac36535fae56b2910780245dd69dda270bc", false, tc.r)

	assert.Nil(pTest, err)

	nftsByUser, err := persist.NftGetByUserID(ctx, userID, tc.r)
	assert.Nil(pTest, err)

	assert.Equal(pTest, len(nfts), len(nftsByUser))

}
