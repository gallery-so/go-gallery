package postgres

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestCollectionGetByUserID_Success(t *testing.T) {
	a, db := setupTest(t)

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

	id, err := userRepo.Create(context.Background(), user)
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
		OwnerUserID: id,
		Nfts:        ids,
	}

	_, err = collectionRepo.Create(context.Background(), collection)
	a.NoError(err)

	collections, err := collectionRepo.GetByUserID(context.Background(), id, true)
	a.NoError(err)

	a.Equal(1, len(collections))

	a.Greater(len(collections[0].NFTs), 0)
}

func TestCollectionGetByID_Success(t *testing.T) {
	a, db := setupTest(t)

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

	id, err := userRepo.Create(context.Background(), user)
	a.NoError(err)
	a.NotEmpty(id)

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
	a.NotEmpty(ids)

	collection := persist.CollectionDB{
		Name:        "name",
		OwnerUserID: id,
		Nfts:        ids,
	}

	collID, err := collectionRepo.Create(context.Background(), collection)
	a.NoError(err)
	a.NotEmpty(collID)

	coll, err := collectionRepo.GetByID(context.Background(), collID, true)
	a.NoError(err)

	a.Equal(collection.Name, coll.Name)

	a.Greater(len(coll.NFTs), 0)

}
