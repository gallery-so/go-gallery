package persist

import (
	"context"
	"encoding/json"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var historyColName = "history"

// OwnershipHistory represents a list of owners for an NFT.
// This struct does not need any mongo related fields becuase it will
// not be persisted past the memory storage.
type OwnershipHistory struct {
	Owners []*Owner `json:"owners"`
}

// Owner represents a single owner of an NFT.
type Owner struct {
	Address      string             `json:"address"`
	UserID       DBID               `json:"user_id"`
	Username     string             `json:"username"`
	TimeObtained primitive.DateTime `json:"time_obtained"`
}

// HistorySetCache caches a transfer in the memory storage
func HistorySetCache(pCtx context.Context, pHistory *OwnershipHistory, pRequestURL string, pRuntime *runtime.Runtime) error {
	history, err := json.Marshal(pHistory)
	if err != nil {
		return err
	}
	mp := newStorage(0, historyColName, pRuntime).withRedis(OpenseaTransfersRDB, pRuntime)
	return mp.cacheSet(pCtx, pRequestURL, string(history), openseaTransfersTTL)
}

// HistoryGetCached retrieves a transfer from the memory storage
func HistoryGetCached(pCtx context.Context, pRequestURL string, pRuntime *runtime.Runtime) (string, error) {
	mp := newStorage(0, historyColName, pRuntime).withRedis(OpenseaTransfersRDB, pRuntime)
	return mp.cacheGet(pCtx, pRequestURL)
}
