package persist

import (
	"context"
)

// Backup represents a backup of a gallery in the database.
type Backup struct {
	Version      int64           `bson:"version" json:"version"` // schema version for this model
	ID           DBID            `bson:"_id" json:"id"`
	CreationTime CreationTime    `bson:"created_at" json:"created_at"`
	Deleted      bool            `bson:"deleted" json:"-"`
	LastUpdated  LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	GalleryID DBID    `bson:"gallery_id" json:"gallery_id" `
	Gallery   Gallery `bson:"gallery" json:"gallery"`
}

// BackupRepository is the interface for interacting with backed up versions of galleries
type BackupRepository interface {
	Insert(context.Context, Gallery) error
}
