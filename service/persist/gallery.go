package persist

import (
	"fmt"
)

// GalleryDB represents a group of collections of NFTs in the database.
// Collections of NFTs will be represented as a list of collection IDs creating
// a join relationship in the database
// This struct will only be used in database operations
type GalleryDB struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	OwnerUserID DBID   `json:"owner_user_id"`
	Collections []DBID `json:"collections"`
}

// Gallery represents a group of collections of NFTS in the application.
// Collections are represented as structs instead of IDs
// This struct will be decoded from a find database operation and used throughout
// the application where GalleryDB is not used
type Gallery struct {
	Version      NullInt32       `json:"version"` // schema version for this model
	ID           DBID            `json:"id" binding:"required"`
	CreationTime CreationTime    `json:"created_at"`
	Deleted      NullBool        `json:"-"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`

	OwnerUserID DBID         `json:"owner_user_id"`
	Collections []Collection `json:"collections"`
}

// GalleryTokenUpdateInput represents a struct that is used to update a gallery's list of collections in the databse
type GalleryTokenUpdateInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	Collections []DBID `json:"collections"`
}

// ErrGalleryNotFound is returned when a gallery is not found by its ID
type ErrGalleryNotFound struct {
	ID           DBID
	CollectionID DBID
}

func (e ErrGalleryNotFound) Error() string {
	return fmt.Sprintf("gallery not found with ID: %v CollectionID: %v", e.ID, e.CollectionID)
}
