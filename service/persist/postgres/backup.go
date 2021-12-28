package postgres

import (
	"context"
	"database/sql"

	"github.com/mikeydub/go-gallery/service/persist"
)

// BackupRepository is the postgres implementation for interacting with backed up versions of galleries
type BackupRepository struct {
	db *sql.DB
}

// NewBackupRepository creates a new postgres repository for interacting with backed up versions of galleries
func NewBackupRepository(db *sql.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

// Insert inserts a new backup of a gallery into the database and ensures that old backups are removed
func (b *BackupRepository) Insert(pCtx context.Context, pGallery persist.Gallery) error {
	getCurrentBackups := `SELECT ID FROM backups WHERE GALLERY_ID = $1 AND DELETED = false ORDER BY CREATED_AT ASC`
	res, err := b.db.QueryContext(pCtx, getCurrentBackups, pGallery.ID)
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
		deleteBackup := `DELETE FROM backups WHERE ID = $1`
		_, err = b.db.ExecContext(pCtx, deleteBackup, currentBackups[0])
		if err != nil {
			return err
		}
	}

	insertBackup := `INSERT INTO backups (ID, GALLERY_ID, VERSION, GALLERY) VALUES ($1, $2, $3, $4)`
	_, err = b.db.ExecContext(pCtx, insertBackup, persist.GenerateID(), pGallery.ID, pGallery.Version, pGallery)
	if err != nil {
		return err
	}

	return nil

}
