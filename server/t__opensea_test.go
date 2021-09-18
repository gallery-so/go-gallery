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

	robin := &persist.User{Addresses: []string{"0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"}}
	gianna := &persist.User{Addresses: []string{"0xdd33e6fd03983c970ae5e647df07314435d69f6b"}}
	robinUserID, err := persist.UserCreate(ctx, robin, tc.r)
	assert.Nil(t, err)
	giannaUserID, err := persist.UserCreate(ctx, gianna, tc.r)
	assert.Nil(t, err)

	nft := &persist.Nft{
		OwnerUserID:  giannaUserID,
		OwnerAddress: "0xdd33e6fd03983c970ae5e647df07314435d69f6b",
		Name:         "kks",
		OpenSeaID:    34147626,
	}

	nft2 := &persist.Nft{
		OwnerUserID:  robinUserID,
		OwnerAddress: "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
		Name:         "malsjdlaksjd",
		OpenSeaID:    46062326,
	}

	_, err = persist.NftCreateBulk(ctx, []*persist.Nft{nft, nft2}, tc.r)
	assert.Nil(t, err)

	now, err := persist.NftGetByUserID(ctx, giannaUserID, tc.r)
	assert.Nil(t, err)
	assert.Len(t, now, 1)

	nfts, err := openSeaPipelineAssetsForAcc(ctx, robinUserID, "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15", true, tc.r)
	assert.Nil(t, err)

	nftsByUser, err := persist.NftGetByUserID(ctx, robinUserID, tc.r)
	assert.Nil(t, err)

	nftsByUserTwo, err := persist.NftGetByUserID(ctx, giannaUserID, tc.r)
	assert.Nil(t, err)

	assert.Len(t, nftsByUserTwo, 0)

	assert.Len(t, nfts, len(nftsByUser))

	// process is async so this may not work
	assert.NotNil(t, nftsByUser[0].OwnershipHistory)

}
