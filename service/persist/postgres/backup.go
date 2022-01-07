package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// BackupRepository is the postgres implementation for interacting with backed up versions of galleries
type BackupRepository struct {
	db                    *sql.DB
	getCurrentBackupsStmt *sql.Stmt
	deleteBackupStmt      *sql.Stmt
	insertBackupStmt      *sql.Stmt
}

// NewBackupRepository creates a new postgres repository for interacting with backed up versions of galleries
func NewBackupRepository(db *sql.DB) *BackupRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	getCurrentBackupsStmt, err := db.PrepareContext(ctx, `SELECT ID FROM backups WHERE GALLERY_ID = $1 AND DELETED = false ORDER BY CREATED_AT ASC`)
	checkNoErr(err)

	deleteBackupStmt, err := db.PrepareContext(ctx, `DELETE FROM backups WHERE ID = $1`)
	checkNoErr(err)

	insertBackupStmt, err := db.PrepareContext(ctx, `INSERT INTO backups (ID, GALLERY_ID, VERSION, GALLERY) VALUES ($1, $2, $3, $4)`)
	checkNoErr(err)

	return &BackupRepository{db: db, getCurrentBackupsStmt: getCurrentBackupsStmt, deleteBackupStmt: deleteBackupStmt, insertBackupStmt: insertBackupStmt}
}

// Insert inserts a new backup of a gallery into the database and ensures that old backups are removed
func (b *BackupRepository) Insert(pCtx context.Context, pGallery persist.Gallery) error {
	res, err := b.getCurrentBackupsStmt.QueryContext(pCtx, pGallery.ID)
	if err != nil {
		return err
	}
	defer res.Close()

	var currentBackups []persist.DBID
	for res.Next() {
		var id persist.DBID
		err = res.Scan(&id)
		if err != nil {
			return err
		}
		currentBackups = append(currentBackups, id)
	}

	if err = res.Err(); err != nil {
		return err
	}

	if len(currentBackups) > 2 {
		// delete the oldest backup
		_, err = b.deleteBackupStmt.ExecContext(pCtx, currentBackups[0])
		if err != nil {
			return err
		}
	}

	_, err = b.insertBackupStmt.ExecContext(pCtx, persist.GenerateID(), pGallery.ID, pGallery.Version, pGallery)
	if err != nil {
		return err
	}

	return nil

}
