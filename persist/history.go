package persist

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	history, err := json.Marshal(pHistory)
	if err != nil {
		return err
	}
	mp := newStorage(0, historyColName, pRuntime).withRedis(OpenseaTransfersRDB, pRuntime)
	err = mp.cacheSet(pCtx, string(pNFTID), string(history), openseaTransfersTTL)
	if err != nil {
		return err
	}

	return mp.upsert(pCtx, bson.M{"nft_id": pNFTID}, pHistory)
}

// HistoryGet retrieves a transfer from the memory storage
func HistoryGet(pCtx context.Context, pNFTID DBID, skipCache bool, pRuntime *runtime.Runtime) (*OwnershipHistory, error) {
	mp := newStorage(0, historyColName, pRuntime).withRedis(OpenseaTransfersRDB, pRuntime)
	if !skipCache {
		history := &OwnershipHistory{}
		val, err := mp.cacheGet(pCtx, string(pNFTID))
		if err == nil {
			err = json.Unmarshal([]byte(val), history)
			if err == nil {
				return history, nil
			}
		}
	}

	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	histories := []*OwnershipHistory{}
	err := mp.find(pCtx, bson.M{"nft_id": pNFTID}, histories, opts)
	if err != nil {
		return nil, err
	}
	if len(histories) == 0 {
		return nil, errors.New("no history found")
	}

	toCache, err := json.Marshal(histories[0])
	if err != nil {
		return nil, err
	}
	err = mp.cacheSet(pCtx, string(pNFTID), string(toCache), openseaTransfersTTL)
	if err != nil {
		return nil, err
	}

	return histories[0], nil
}
