package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

func TestBackup(t *testing.T) {

	t.Run("backup restores gallery", func(t *testing.T) {
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
	})

	// backups made within 5 mins of each other should be de-duped
	t.Run("backups are deduped", func(t *testing.T) {
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
	})

	// backups made over 5 mins of each other should be added
	t.Run("backups not throttled are stored", func(t *testing.T) {
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
	})

	// Backup pruning rules:
	// - backups in the past 24 hours should be stored no more often than every 5 minutes
	// - backups in the past 7 days should be stored no more often than every 1 hour
	// - backups beyond the past 7 days should be stored no more often than every 1 day
	t.Run("backups are pruned correctly", func(t *testing.T) {
		// timestamps within past 24h
		d1 := time.Now().Add(-6 * time.Minute)
		d2 := time.Now().Add(-7 * time.Minute)
		d3 := time.Now().Add(-15 * time.Minute)
		d4 := time.Now().Add(-25 * time.Minute)
		d5 := time.Now().Add(-1 * time.Hour)
		d6 := time.Now().Add(-2 * time.Hour)
		d7 := time.Now().Add(-2*time.Hour - 1*time.Minute)
		d8 := time.Now().Add(-15 * time.Hour)
		// timestamps within past 7d
		day := 24 * time.Hour
		w1 := time.Now().Add(-3 * day)
		w2 := time.Now().Add(-3*day - 20*time.Minute)
		w3 := time.Now().Add(-3*day - 30*time.Minute)
		w4 := time.Now().Add(-3*day - 40*time.Minute)
		w5 := time.Now().Add(-3*day - 120*time.Minute)
		w6 := time.Now().Add(-5 * day)
		w7 := time.Now().Add(-6 * day)
		// timestamps beyond 7d
		t1 := time.Now().Add(-10 * day)
		t2 := time.Now().Add(-10*day - 30*time.Minute)
		t3 := time.Now().Add(-11 * day)
		t4 := time.Now().Add(-12*day - 30*time.Minute)
		t5 := time.Now().Add(-13 * day)
		t6 := time.Now().Add(-30 * day)
		t7 := time.Now().Add(-60 * day)

		testCases := []struct {
			input    []time.Time
			expected []time.Time
		}{
			{input: []time.Time{d1, d2, d3, d4}, expected: []time.Time{d1, d3, d4}},
			{input: []time.Time{d1, d3, d6, d7}, expected: []time.Time{d1, d3, d6}},
			{input: []time.Time{d1, d2, d3, d4, d5, d6, d7, d8}, expected: []time.Time{d1, d3, d4, d5, d6, d8}},
			{input: []time.Time{w1, w2, w3, w4}, expected: []time.Time{w1}},
			{input: []time.Time{w2, w4, w5, w6, w7}, expected: []time.Time{w2, w5, w6, w7}},
			{input: []time.Time{t1, t2, t3, t4, t5}, expected: []time.Time{t1, t3, t4}},
			{input: []time.Time{t2, t3, t5, t6, t7}, expected: []time.Time{t2, t5, t6, t7}},
			{input: []time.Time{
				d1, d2, d3, d4, d5, d6, d7, d8,
				w1, w2, w3, w4, w5, w6, w7,
				t1, t2, t3, t4, t5, t6, t7,
			}, expected: []time.Time{
				d1, d3, d4, d5, d6, d8, w1, w5,
				w6, w7, t1, t3, t4, t6, t7,
			}},
		}

		for _, tc := range testCases {
			a, db := setupTest(t)
			userID, _, _, g, backupRepo, _, _, _, _ := createMockGallery(t, a, db)

			// manually seed all inputs except for the first one
			for _, inputTimestamp := range tc.input {
				backupRepo.insertBackupStmt.ExecContext(context.Background(), persist.GenerateID(), g.ID, g.Version, g, persist.CreationTime(inputTimestamp))
			}

			// officially insert a gallery via .Insert() to trigger underlying pruning
			backupRepo.Insert(context.Background(), g)

			backups, err := backupRepo.Get(context.Background(), userID)
			a.NoError(err)
			// should have a length of the expected seeded backups *plus* the officially inserted gallery via .Insert()
			a.Len(backups, len(tc.expected)+1)
			for i := 0; i < len(backups)-1; i++ {
				backup := backups[i]
				// queried results are returned in reverse of insert order
				actualCreationTime := time.Time(backup.CreationTime).Round(time.Second)
				expectedCreationTime := tc.expected[len(tc.expected)-1-i].Round(time.Second)
				a.True(actualCreationTime.Equal(expectedCreationTime))
			}
		}
	})
}
