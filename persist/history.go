package persist

import (
	"context"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var historyColName = "history"

// OwnershipHistory represents a list of owners for an NFT.
type OwnershipHistory struct {
	Version      int64              `bson:"version"       json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"           json:"id"`
	CreationTime primitive.DateTime `bson:"created_at" json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	NFTID  DBID     `bson:"nft_id" json:"nft_id"`
	Owners []*Owner `bson:"owners" json:"owners"`
}

// Owner represents a single owner of an NFT.
type Owner struct {
	Address      string             `bson:"address" json:"address"`
	UserID       DBID               `bson:"user_id" json:"user_id"`
	Username     string             `bson:"username" json:"username"`
	TimeObtained primitive.DateTime `bson:"time_obtained" json:"time_obtained"`
}

// HistoryUpsert caches a transfer in the memory storage
func HistoryUpsert(pCtx context.Context, pNFTID DBID, pHistory *OwnershipHistory, pRuntime *runtime.Runtime) error {

	pHistory.NFTID = pNFTID
	mp := newStorage(0, historyColName, pRuntime)

	return mp.upsert(pCtx, bson.M{"nft_id": pNFTID}, pHistory)
}
