package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB) {
	viper.Set("POSTGRES_HOST", "0.0.0.0")
	viper.Set("POSTGRES_PORT", 5432)
	viper.Set("POSTGRES_USER", "postgres")
	viper.Set("POSTGRES_PASSWORD", "")
	viper.Set("POSTGRES_DB", "postgres")
	viper.Set("ENV", "local")

	db := NewClient()

	t.Cleanup(func() {
		defer db.Close()
		dropSQL := `TRUNCATE users, nfts, collections, galleries;`
		_, err := db.Exec(dropSQL)
		if err != nil {
			t.Logf("error dropping tables: %v", err)
		}
	})

	return assert.New(t), db
}

func createMockGallery(t *testing.T, a *assert.Assertions, db *sql.DB) (persist.DBID, persist.DBID, []persist.DBID, persist.Gallery, *BackupRepository, *GalleryRepository, *CollectionRepository, *NFTRepository, *UserRepository) {
	galleryRepo := NewGalleryRepository(db, redis.NewCache(0))
	collectionRepo := NewCollectionRepository(db, galleryRepo)
	nftRepo := NewNFTRepository(db, galleryRepo)
	userRepo := NewUserRepository(db)
	backupRepo := NewBackupRepository(db)

	user := persist.User{
		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses: []persist.Address{
			"0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
		},
	}

	userID, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	nfts := []persist.NFT{
		{
			OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
			Name:         "name",
			OpenseaID:    1,
		},
		{
			OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d2",
			Name:         "blah blah",
			OpenseaID:    10,
		},
	}

	nftIDs, err := nftRepo.CreateBulk(context.Background(), nfts)
	a.NoError(err)

	collection := persist.CollectionDB{
		Name:        "name",
		OwnerUserID: userID,
		NFTs:        nftIDs,
	}

	collID, err := collectionRepo.Create(context.Background(), collection)
	a.NoError(err)

	gallery := persist.GalleryDB{
		OwnerUserID: userID,
		Collections: []persist.DBID{collID},
	}

	id, err := galleryRepo.Create(context.Background(), gallery)
	a.NoError(err)

	g, err := galleryRepo.GetByID(context.Background(), id)
	a.NoError(err)

	return userID, collID, nftIDs, g, backupRepo, galleryRepo, collectionRepo, nftRepo, userRepo
}
