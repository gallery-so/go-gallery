package persist

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const accountCollName = "accounts"

// Account represents an ethereum account in the database
type Account struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	Address         string `bson:"address" json:"address"`
	LastSyncedBlock string `bson:"last_synced_block" json:"last_synced_block"`
}

// AccountUpsertByAddress updates a user by address
// pUpdate represents a struct with bson tags to specify which fields to update
func AccountUpsertByAddress(pCtx context.Context, pAddress string, pUpsert *Account,
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, accountCollName, pRuntime)

	_, err := mp.upsert(pCtx, bson.M{
		"address": pAddress,
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// AccountGetByAddress returns an account by a given address
func AccountGetByAddress(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*Account, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, runtime.GalleryDBName, usersCollName, pRuntime)

	result := []*Account{}
	err := mp.find(pCtx, bson.M{"address": pAddress}, &result, opts)

	if err != nil {
		return nil, err
	}

	return result, nil
}
