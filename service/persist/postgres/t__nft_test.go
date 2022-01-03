package postgres

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestNFTCreate_Success(t *testing.T) {
	a, db := setupTest(t)

	nftRepo := NewNFTRepository(db, redis.NewCache(0), redis.NewCache(1))

	nft := persist.NFT{
		Deleted:      false,
		Version:      1,
		OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		Name:         "name",
	}

	_, err := nftRepo.Create(context.Background(), nft)
	a.NoError(err)

}

func TestNFTGetByID_Success(t *testing.T) {
	a, db := setupTest(t)

	nftRepo := NewNFTRepository(db, redis.NewCache(0), redis.NewCache(1))

	nft := persist.NFT{
		Deleted:      false,
		Version:      1,
		OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		Name:         "name",
	}

	id, err := nftRepo.Create(context.Background(), nft)
	a.NoError(err)

	nft2, err := nftRepo.GetByID(context.Background(), id)
	a.NoError(err)
	a.Equal(id, nft2.ID)
	a.Equal(nft.OwnerAddress, nft2.OwnerAddress)
	a.Equal(nft.Name, nft2.Name)
}
