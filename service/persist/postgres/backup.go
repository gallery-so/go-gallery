package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// BackupRepository is the postgres implementation for interacting with backed up versions of galleries
type BackupRepository struct {
	db                    *sql.DB
	getCurrentBackupsStmt *sql.Stmt
	getGalleryIDStmt      *sql.Stmt
	getBackupsStmt        *sql.Stmt
	getBackupByIDStmt     *sql.Stmt
	deleteBackupStmt      *sql.Stmt
	insertBackupStmt      *sql.Stmt

	getUserAddressesStmt *sql.Stmt
	ownsNFTStmt          *sql.Stmt

	updateCollectionNFTsStmt *sql.Stmt
	updateGalleryStmt        *sql.Stmt
}

// NewBackupRepository creates a new postgres repository for interacting with backed up versions of galleries
func NewBackupRepository(db *sql.DB) *BackupRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getCurrentBackupsStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT FROM backups WHERE GALLERY_ID = $1 AND DELETED = false ORDER BY CREATED_AT ASC;`)
	checkNoErr(err)

	getGalleryIDStmt, err := db.PrepareContext(ctx, `SELECT ID FROM galleries WHERE OWNER_USER_ID = $1 AND DELETED = false LIMIT 1;`)
	checkNoErr(err)

	getBackupsStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT,VERSION,GALLERY_ID,GALLERY FROM backups WHERE GALLERY_ID = $1 AND DELETED = false ORDER BY CREATED_AT ASC;`)
	checkNoErr(err)

	getBackupByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,CREATED_AT,VERSION,GALLERY_ID,GALLERY FROM backups WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	deleteBackupStmt, err := db.PrepareContext(ctx, `DELETE FROM backups WHERE ID = $1;`)
	checkNoErr(err)

	insertBackupStmt, err := db.PrepareContext(ctx, `INSERT INTO backups (ID, GALLERY_ID, VERSION, GALLERY) VALUES ($1, $2, $3, $4);`)
	checkNoErr(err)

	getUserAddressesStmt, err := db.PrepareContext(ctx, `SELECT ADDRESSES FROM users WHERE ID = $1;`)
	checkNoErr(err)

	ownsNFTStmt, err := db.PrepareContext(ctx, `SELECT EXISTS(SELECT 1 FROM nfts WHERE OWNER_ADDRESS = ANY($1) AND ID = $2 AND DELETED = false);`)
	checkNoErr(err)

	updateCollectionNFTsStmt, err := db.PrepareContext(ctx, `UPDATE collections SET NFTS = $2 WHERE ID = $1;`)
	checkNoErr(err)

	updateGalleryStmt, err := db.PrepareContext(ctx, `UPDATE galleries SET COLLECTIONS = $2 WHERE ID = $1;`)
	checkNoErr(err)

	return &BackupRepository{
		db:                    db,
		getCurrentBackupsStmt: getCurrentBackupsStmt,
		deleteBackupStmt:      deleteBackupStmt,
		insertBackupStmt:      insertBackupStmt,
		getGalleryIDStmt:      getGalleryIDStmt,
		getBackupsStmt:        getBackupsStmt,
		getBackupByIDStmt:     getBackupByIDStmt,

		getUserAddressesStmt: getUserAddressesStmt,
		ownsNFTStmt:          ownsNFTStmt,

		updateCollectionNFTsStmt: updateCollectionNFTsStmt,
		updateGalleryStmt:        updateGalleryStmt,
	}
}

// Insert inserts a new backup of a gallery into the database and ensures that old backups are removed
func (b *BackupRepository) Insert(pCtx context.Context, pGallery persist.Gallery) error {
	res, err := b.getCurrentBackupsStmt.QueryContext(pCtx, pGallery.ID)
	if err != nil {
		return err
	}
	defer res.Close()

	currentBackups := map[persist.DBID]persist.CreationTime{}
	for res.Next() {
		var id persist.DBID
		var createdAt persist.CreationTime
		err = res.Scan(&id, &createdAt)
		if err != nil {
			return err
		}
		currentBackups[id] = createdAt
	}

	if err = res.Err(); err != nil {
		return err
	}

	doInsert := true
	for id, createdAt := range currentBackups {
		if time.Since(createdAt.Time()) > time.Hour*24*5 {
			_, err = b.deleteBackupStmt.ExecContext(pCtx, id)
			if err != nil {
				return err
			}
		}
		if time.Since(createdAt.Time()) < time.Hour*24*7 {
			doInsert = false
		}
	}

	if doInsert {
		_, err = b.insertBackupStmt.ExecContext(pCtx, persist.GenerateID(), pGallery.ID, pGallery.Version, pGallery)
		if err != nil {
			return err
		}
	}

	return nil

}

// Get returns the current backups of a gallery for a user
func (b *BackupRepository) Get(pCtx context.Context, pUserID persist.DBID) ([]persist.Backup, error) {
	var galleryID persist.DBID
	err := b.getGalleryIDStmt.QueryRowContext(pCtx, pUserID).Scan(&galleryID)
	if err != nil {
		return nil, err
	}

	res, err := b.getBackupsStmt.QueryContext(pCtx, galleryID)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	backups := []persist.Backup{}
	for res.Next() {
		var backup persist.Backup
		err = res.Scan(&backup.ID, &backup.CreationTime, &backup.Version, &backup.GalleryID, &backup.Gallery)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}

	if err = res.Err(); err != nil {
		return nil, err
	}

	return backups, nil
}

// Restore restores a backup of a gallery for a user
func (b *BackupRepository) Restore(pCtx context.Context, pBackupID, pUserID persist.DBID) error {

	var galleryID persist.DBID
	err := b.getGalleryIDStmt.QueryRowContext(pCtx, pUserID).Scan(&galleryID)
	if err != nil {
		return fmt.Errorf("could not get gallery id for user %s: %w", pUserID, err)
	}

	var backup persist.Backup
	err = b.getBackupByIDStmt.QueryRowContext(pCtx, pBackupID).Scan(&backup.ID, &backup.CreationTime, &backup.Version, &backup.GalleryID, &backup.Gallery)
	if err != nil {
		return fmt.Errorf("could not get backup %s: %w", pBackupID, err)
	}

	var addresses []persist.Address
	err = b.getUserAddressesStmt.QueryRowContext(pCtx, pUserID).Scan(pq.Array(&addresses))
	if err != nil {
		return fmt.Errorf("could not get user addresses for user %s: %w", pUserID, err)
	}

	tx, err := b.db.BeginTx(pCtx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updateNFTs := tx.StmtContext(pCtx, b.updateCollectionNFTsStmt)
	updateGallery := tx.StmtContext(pCtx, b.updateGalleryStmt)

	collIDs := make([]persist.DBID, 0, len(backup.Gallery.Collections))
	for _, coll := range backup.Gallery.Collections {
		collIDs = append(collIDs, coll.ID)
		result := make([]persist.DBID, 0, len(coll.NFTs))
		for _, nft := range coll.NFTs {
			var owns bool
			err = b.ownsNFTStmt.QueryRowContext(pCtx, pq.Array(addresses), nft.ID).Scan(&owns)
			if err != nil {
				return fmt.Errorf("could not check if user owns nft %s: %w", nft.ID, err)
			}
			if owns {
				result = append(result, nft.ID)
			}
		}
		_, err = updateNFTs.ExecContext(pCtx, coll.ID, pq.Array(result))
		if err != nil {
			return err
		}
	}

	_, err = updateGallery.ExecContext(pCtx, galleryID, pq.Array(collIDs))
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}
