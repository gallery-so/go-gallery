package postgres

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestGetCommunity_Success(t *testing.T) {
	a, db := setupTest(t)

	galleryRepo := NewGalleryRepository(db, redis.NewCache(0))
	collectionRepo := NewCollectionRepository(db, galleryRepo)
	nftRepo := NewNFTRepository(db, galleryRepo)
	userRepo := NewUserRepository(db)
	communityRepo := NewCommunityRepository(db, redis.NewCache(1))

	user := persist.User{

		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Wallets: []persist.EthereumAddress{
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	userID, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	nfts := []persist.NFT{
		{
			OwnerAddress:   "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
			Name:           "name",
			OpenseaID:      1,
			OpenseaTokenID: "0x2",
			Contract: persist.NFTContract{
				ContractAddress:     "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
				ContractDescription: "description",
				ContractName:        "name",
				ContractImage:       "image",
			},
		},
		{
			OwnerAddress:   "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
			Name:           "blah blah",
			OpenseaID:      10,
			OpenseaTokenID: "0x1",
			Contract: persist.NFTContract{
				ContractAddress:     "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
				ContractDescription: "description",
				ContractName:        "name",
				ContractImage:       "image",
			},
		},
	}

	ids, err := nftRepo.CreateBulk(context.Background(), nfts)
	a.NoError(err)

	gallery := persist.GalleryDB{
		OwnerUserID: userID,
		Collections: []persist.DBID{},
	}

	id, err := galleryRepo.Create(context.Background(), gallery)
	a.NoError(err)

	collection := persist.CollectionDB{
		Name:        "name",
		OwnerUserID: userID,
		NFTs:        ids,
	}

	collID, err := collectionRepo.Create(context.Background(), collection)
	a.NoError(err)

	err = galleryRepo.AddCollections(context.Background(), id, userID, []persist.DBID{collID})
	a.NoError(err)

	galleries, err := galleryRepo.GetByUserID(context.Background(), userID)
	a.NoError(err)

	a.Len(galleries, 1)

	a.Equal(userID, galleries[0].OwnerUserID)
	a.Len(galleries[0].Collections, 1)
	a.Len(galleries[0].Collections[0].NFTs, 2)

	newNFTs, err := nftRepo.GetByContractData(context.Background(), "0x1", nfts[0].Contract.ContractAddress)
	a.NoError(err)
	a.Len(newNFTs, 1)

	comm, err := communityRepo.GetByAddress(context.Background(), nfts[0].Contract.ContractAddress, true)
	a.NoError(err)

	a.Equal(nfts[0].Contract.ContractName, comm.Name)
	a.Greater(len(comm.Owners), 0)
}
