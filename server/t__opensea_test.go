package server

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/stretchr/testify/assert"
)

//---------------------------------------------------
func TestFetchAssertsForAcc(t *testing.T) {
	setupTest(t)
	ctx := context.Background()

	user := &persist.User{Addresses: []string{"0x485b8ac36535fae56b2910780245dd69dda270bc"}}

	userID, err := persist.UserCreate(ctx, user, tc.r)
	assert.Nil(t, err)

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
	assert.Nil(t, err)

	nfts, err := openSeaPipelineAssetsForAcc(ctx, "0x485b8ac36535fae56b2910780245dd69dda270bc", true, tc.r)

	assert.Nil(t, err)

	nftsByUser, err := persist.NftGetByUserID(ctx, userID, tc.r)
	assert.Nil(t, err)

	assert.Equal(t, len(nfts), len(nftsByUser))

	// process is async so this may not work
	assert.NotNil(t, nftsByUser[0].OwnershipHistory)

}
