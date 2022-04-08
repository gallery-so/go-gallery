package postgres

import (
	"context"
	"database/sql"
	"testing"

	migrate "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/docker"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (*assert.Assertions, *sql.DB) {
	pg, pgUnpatch := docker.InitPostgres()
	rd, rdUnpatch := docker.InitRedis()

	db := NewClient()
	err := migrate.RunMigration(db)
	if err != nil {
		t.Fatalf("failed to seed db: %s", err)
	}

	t.Cleanup(func() {
		defer db.Close()
		defer pgUnpatch()
		defer rdUnpatch()
		for _, r := range []*dockertest.Resource{pg, rd} {
			if err := r.Close(); err != nil {
				t.Fatalf("could not purge resource: %s", err)
			}
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

	address := util.RandEthAddress()

	user := persist.User{
		Username:           "username",
		UsernameIdempotent: "username-idempotent",
		Addresses:          []persist.Address{persist.Address(address)},
	}

	userID, err := userRepo.Create(context.Background(), user)
	a.NoError(err)

	nfts := []persist.NFT{
		{
			OwnerAddress: persist.Address(address),
			Name:         "name",
			OpenseaID:    1,
		},
		{
			OwnerAddress: persist.Address(address),
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
