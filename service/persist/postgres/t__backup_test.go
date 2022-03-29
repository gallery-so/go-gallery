package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

func TestBackupRestore_Success(t *testing.T) {
	a, db := setupTest(t)
	userID, collID, nftIDs, g, backupRepo, galleryRepo, collectionRepo, nftRepo, _ := createMockGallery(t, a, db)

	err := backupRepo.Insert(context.Background(), g)
	a.NoError(err)

	backups, err := backupRepo.Get(context.Background(), userID)
	a.NoError(err)
	a.Len(backups, 1)
	a.Len(backups[0].Gallery.Collections, 1)

	collection2 := persist.CollectionDB{
		Name:        "name2",
		OwnerUserID: userID,
		NFTs:        nftIDs,
	}

	collID2, err := collectionRepo.Create(context.Background(), collection2)
	a.NoError(err)

	err = galleryRepo.Update(context.Background(), g.ID, userID, persist.GalleryUpdateInput{
		Collections: []persist.DBID{collID, collID2},
	})
	a.NoError(err)

	err = nftRepo.UpdateByID(context.Background(), nftIDs[0], userID, persist.NFTUpdateOwnerAddressInput{
		OwnerAddress: "0x8914496dc01efcc49a2fa340331fb90969b6f1d3",
	})
	a.NoError(err)

	err = backupRepo.Restore(context.Background(), backups[0].ID, userID)
	a.NoError(err)

	galleries, err := galleryRepo.GetByUserID(context.Background(), userID)
	a.NoError(err)

	a.Len(galleries, 1)

	a.Len(galleries[0].Collections, 1)
}

// backups made within 5 mins of each other should be de-duped
func TestThrottle_Success(t *testing.T) {
	a, db := setupTest(t)
	userID, collID, nftIDs, g, backupRepo, galleryRepo, collectionRepo, _, _ := createMockGallery(t, a, db)

	err := backupRepo.Insert(context.Background(), g)
	a.NoError(err)

	backups, err := backupRepo.Get(context.Background(), userID)
	a.NoError(err)
	a.Len(backups, 1)
	a.Len(backups[0].Gallery.Collections, 1)

	collection2 := persist.CollectionDB{
		Name:        "name2",
		OwnerUserID: userID,
		NFTs:        nftIDs,
	}

	collID2, err := collectionRepo.Create(context.Background(), collection2)
	a.NoError(err)

	err = galleryRepo.Update(context.Background(), g.ID, userID, persist.GalleryUpdateInput{
		Collections: []persist.DBID{collID, collID2},
	})
	a.NoError(err)

	err = backupRepo.Insert(context.Background(), g)
	a.NoError(err)

	backups, err = backupRepo.Get(context.Background(), userID)
	a.NoError(err)
	a.Len(backups, 1)
	a.Len(backups[0].Gallery.Collections, 1)
}

// backups made over 5 mins of each other should be added
func TestNoThrottle_Success(t *testing.T) {
	a, db := setupTest(t)
	userID, collID, nftIDs, g, backupRepo, galleryRepo, collectionRepo, _, _ := createMockGallery(t, a, db)

	err := backupRepo.Insert(context.Background(), g)
	a.NoError(err)

	backups, err := backupRepo.Get(context.Background(), userID)
	a.NoError(err)
	a.Len(backups, 1)
	a.Len(backups[0].Gallery.Collections, 1)

	collection2 := persist.CollectionDB{
		Name:        "name2",
		OwnerUserID: userID,
		NFTs:        nftIDs,
	}

	collID2, err := collectionRepo.Create(context.Background(), collection2)
	a.NoError(err)

	err = galleryRepo.Update(context.Background(), g.ID, userID, persist.GalleryUpdateInput{
		Collections: []persist.DBID{collID, collID2},
	})
	a.NoError(err)

	backupRepo.insertBackupStmt.ExecContext(context.Background(), persist.GenerateID(), g.ID, g.Version, g, persist.CreationTime(time.Now().Local().Add(time.Hour)))

	backups, err = backupRepo.Get(context.Background(), userID)
	a.NoError(err)
	a.Len(backups, 2)
	a.Len(backups[0].Gallery.Collections, 1)
}
