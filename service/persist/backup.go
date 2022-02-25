package persist

import (
	"context"
)

// Backup represents a backup of a gallery in the database.
type Backup struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	GalleryID DBID    `json:"gallery_id" `
	Gallery   Gallery `json:"gallery"`
}

// BackupRepository is the interface for interacting with backed up versions of galleries
type BackupRepository interface {
	Insert(context.Context, Gallery) error
}
