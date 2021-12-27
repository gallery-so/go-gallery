package postgres

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestGalleriesGetByUserID_Success(t *testing.T) {
	a, db := setupTest(t)

	galleryRepo := NewGalleryRepository(db, redis.NewCache(3))
	collectionRepo := NewCollectionRepository(db)
	nftRepo := NewNFTRepository(db, redis.NewCache(0), redis.NewCache(1))
	userRepo := NewUserRepository(db)

	user := persist.User{

		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses: []persist.Address{
			"address-1",
		},
	}

	userID, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	nfts := []persist.NFTDB{
		{
			OwnerAddress: "owner",
			Name:         "name",
		},
		{
			OwnerAddress: "next owner",
			Name:         "blah blah",
		},
	}

	ids, err := nftRepo.CreateBulk(context.Background(), nfts)
	a.NoError(err)

	collection := persist.CollectionDB{
		Name:        "name",
		OwnerUserID: userID,
		Nfts:        ids,
	}

	collID, err := collectionRepo.Create(context.Background(), collection)
	a.NoError(err)

	gallery := persist.GalleryDB{
		OwnerUserID: userID,
		Collections: []persist.DBID{collID},
	}

	_, err = galleryRepo.Create(context.Background(), gallery)
	a.NoError(err)

	galleries, err := galleryRepo.GetByUserID(context.Background(), userID)
	a.NoError(err)

	a.Equal(1, len(galleries))

	a.Equal(userID, galleries[0].OwnerUserID)

}
