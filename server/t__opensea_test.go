package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/stretchr/testify/assert"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//---------------------------------------------------
func TestFetchAssertsForAcc(pTest *testing.T) {

	ctx := context.Background()

	user := &persist.User{AddressesLst: []string{"0x485b8ac36535fae56b2910780245dd69dda270bc"}}

	userID, err := persist.UserCreate(user, ctx, r)
	assert.Nil(pTest, err)

	nft := &persist.Nft{
		OwnerUserIdStr:  userID,
		OwnerAddressStr: "0x485b8ac36535fae56b2910780245dd69dda270bc",
		NameStr:         "test",
		OpenSeaIDint:    0,
	}

	nft2 := &persist.Nft{
		OwnerUserIdStr:  userID,
		OwnerAddressStr: "0x485b8ac36535fae56b2910780245dd69dda270bc",
		NameStr:         "test2",
		OpenSeaIDint:    32087758,
	}

	_, err = persist.NftCreateBulk([]*persist.Nft{nft, nft2}, ctx, r)
	assert.Nil(pTest, err)

	nfts, err := OpenSeaPipelineAssetsForAcc("0x485b8ac36535fae56b2910780245dd69dda270bc", ctx, r)
	assert.Nil(pTest, err)

	nftsByUser, err := persist.NftGetByUserId(userID, ctx, r)
	assert.Nil(pTest, err)

	for _, nft := range nfts {
		fmt.Println(nft.NameStr)
	}

	fmt.Println("----------------------------------------------- BY USER")

	for _, nft := range nftsByUser {
		fmt.Println(nft.NameStr)
	}

	assert.Equal(pTest, len(nfts), len(nftsByUser))

}
