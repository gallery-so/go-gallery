package mongodb

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const backupsCollName = "backups"

// BackupMongoRepository is the repository for interacting with gallery backups in a Mongo DB
type BackupMongoRepository struct {
	backupsStorage *storage
}

// NewBackupMongoRepository creates a new instance of the BackupMongoRepository
func NewBackupMongoRepository(mgoClient *mongo.Client) *BackupMongoRepository {
	return &BackupMongoRepository{
		backupsStorage: newStorage(mgoClient, 0, galleryDBName, backupsCollName),
	}
}

// Insert inserts a backed up gallery into the mongo db while also ensuring there are
// no more than three backups per gallery at any given time
func (b *BackupMongoRepository) Insert(pCtx context.Context, pGallery persist.Gallery) error {

	currentlyBackedUp := []*persist.Backup{}
	err := b.backupsStorage.find(pCtx, bson.M{"gallery_id": pGallery.ID}, &currentlyBackedUp, options.Find().SetSort(bson.M{"last_updated": -1}))
	if err != nil {
		return err
	}

	if len(currentlyBackedUp) > 2 {
		// delete the oldest backup(s)
		for _, backup := range currentlyBackedUp[2:] {
			err = b.backupsStorage.delete(pCtx, bson.M{"_id": backup.ID})
			if err != nil {
				return err
			}
		}
	}

	backup := &persist.Backup{
		GalleryID: pGallery.ID,
		Gallery:   pGallery,
	}
	_, err = b.backupsStorage.insert(pCtx, backup)
	return err

}
