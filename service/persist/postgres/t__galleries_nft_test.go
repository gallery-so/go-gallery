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
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	userID, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	nfts := []persist.NFTDB{
		{
			OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
			Name:         "name",
		},
		{
			OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d1",
			Name:         "blah blah",
		},
	}

	ids, err := nftRepo.CreateBulk(context.Background(), nfts)
	a.NoError(err)

	collection := persist.CollectionDB{
		Name:        "name",
		OwnerUserID: userID,
		NFTs:        ids,
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
	a.Equal(1, len(galleries[0].Collections))
	a.Equal(2, len(galleries[0].Collections[0].NFTs))

}
