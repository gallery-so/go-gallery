package main

import (
	"context"
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMigration_Success(t *testing.T) {
	setDefaults()

	assert := assert.New(t)
	ctx := context.Background()
	userAddr := persist.Address(strings.ToLower("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))
	contractAddr := persist.Address(strings.ToLower("0x8914496dC01Efcc49a2FA340331Fb90969B6F1d2"))
	pgClient := postgres.NewClient()

	t.Cleanup(func() {
		pgClient.Exec(`TRUNCATE collections, nfts, tokens, users;`)
	})

	collRepo := postgres.NewCollectionRepository(pgClient)
	nftRepo := postgres.NewNFTRepository(pgClient, nil, nil)
	tokenRepo := postgres.NewTokenRepository(pgClient)
	userRepo := postgres.NewUserRepository(pgClient)
	collTokenRepo := postgres.NewCollectionTokenRepository(pgClient)

	user := persist.User{
		Username:           "bob",
		UsernameIdempotent: "bob",
		Bio:                "im dope",
		Addresses:          []persist.Address{userAddr},
	}

	userID, err := userRepo.Create(ctx, user)
	assert.Nil(err)

	userInDB, err := userRepo.GetByID(ctx, userID)
	assert.Nil(err)
	logrus.Infof("User: %+v %s", userInDB, userInDB.Addresses[0])

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

	before, err := collRepo.GetByID(ctx, collID, true)
	assert.Nil(err)
	assert.Len(before.NFTs, 1)

	var rawNFTs []persist.DBID
	pgClient.QueryRowContext(ctx, `SELECT NFTS FROM collections WHERE ID = $1`, collID).Scan(pq.Array(&rawNFTs))
	logrus.Info(rawNFTs)

	run()

	pgClient.QueryRowContext(ctx, `SELECT NFTS FROM collections WHERE ID = $1`, collID).Scan(pq.Array(&rawNFTs))
	logrus.Info(rawNFTs)

	updatedColl, err := collTokenRepo.GetByID(ctx, collID, true)
	assert.Nil(err)

	assert.Len(updatedColl.NFTs, 1)
	assert.Equal(updatedColl.NFTs[0].ID, tokenID)
}
