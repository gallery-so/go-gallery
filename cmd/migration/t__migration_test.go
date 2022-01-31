package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/stretchr/testify/assert"
)

func TestMigration_Success(t *testing.T) {

	assert := assert.New(t)
	ctx := context.Background()
	userAddr := persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))
	contractAddr := persist.Address(strings.ToLower("0x8914496dC01Efcc49a2FA340331Fb90969B6F1d2"))
	pgClient := postgres.NewClient()

	t.Cleanup(func() {
		pgClient.Exec(`DROP TABLE collections, nfts, tokens, users;`)
	})

	collRepo := postgres.NewCollectionRepository(pgClient)
	nftRepo := postgres.NewNFTRepository(pgClient, nil, nil)
	tokenRepo := postgres.NewTokenRepository(pgClient)
	userRepo := postgres.NewUserRepository(pgClient)

	user := persist.User{
		Username:           "bob",
		UsernameIdempotent: "bob",
		Bio:                "im dope",
		Addresses:          []persist.Address{userAddr},
	}

	userID, err := userRepo.Create(ctx, user)
	assert.Nil(err)

	nft := persist.NFT{
		OpenseaTokenID: "1",
		OpenseaID:      1,
		OwnerAddress:   userAddr,
		Contract: persist.NFTContract{
			ContractAddress: contractAddr,
		},
	}

	nftID, err := nftRepo.Create(ctx, nft)
	assert.Nil(err)

	coll := persist.CollectionDB{
		OwnerUserID:    userID,
		CollectorsNote: "yay",
		Name:           "yes",
		NFTs:           []persist.DBID{nftID},
	}

	collID, err := collRepo.Create(ctx, coll)
	assert.Nil(err)

	tokenEqu := persist.Token{
		TokenID:         "1",
		ContractAddress: contractAddr,
		Name:            "silly token",
		OwnerAddress:    nft.OwnerAddress,
	}

	tokenID, err := tokenRepo.Create(ctx, tokenEqu)
	assert.Nil(err)

	run()

	updatedColl, err := collRepo.GetByID(ctx, collID, true)
	assert.Nil(err)

	assert.Len(updatedColl.NFTs, 1)
	assert.Equal(updatedColl.NFTs[0].ID, tokenID)
}
