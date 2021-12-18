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

	nft := persist.NFTDB{
		Deleted:      false,
		Version:      1,
		OwnerAddress: "owner",
		Name:         "name",
	}

	_, err := nftRepo.Create(context.Background(), nft)
	a.NoError(err)

}
