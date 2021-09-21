package persist

import (
	"context"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const contractsCollName = "contracts"

// Contract represents an ethereum contract in the database
type Contract struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id" binding:"required"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	Address         string `bson:"address" json:"address"`
	LastSyncedBlock string `bson:"last_synced_block" json:"last_synced_block"`
	Symbol          string `bson:"symbol" json:"symbol"`
	TokenName       string `bson:"token_name" json:"token_name"`
}

// ContractUpsertByAddress upserts an contract by a given address
// pUpdate represents a struct with bson tags to specify which fields to update
func ContractUpsertByAddress(pCtx context.Context, pAddress string, pUpsert *Contract,
	pRuntime *runtime.Runtime) error {

	mp := newStorage(0, runtime.GalleryDBName, contractsCollName, pRuntime)

	_, err := mp.upsert(pCtx, bson.M{
		"address": strings.ToLower(pAddress),
	}, pUpsert)
	if err != nil {
		return err
	}

	return nil
}

// ContractGetByAddress returns an contract by a given address
func ContractGetByAddress(pCtx context.Context, pAddress string,
	pRuntime *runtime.Runtime) ([]*Contract, error) {

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	mp := newStorage(0, runtime.GalleryDBName, usersCollName, pRuntime)

	result := []*Contract{}
	err := mp.find(pCtx, bson.M{"address": strings.ToLower(pAddress)}, &result, opts)

	if err != nil {
		return nil, err
	}

	return result, nil
}
