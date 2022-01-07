package mongodb

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var historyColName = "history"

// HistoryRepository is a repository that stores collections in a MongoDB database
type HistoryRepository struct {
	historiesStorage *storage
}

// NewHistoryRepository creates a new instance of the collection mongo repository
func NewHistoryRepository(mgoClient *mongo.Client) *HistoryRepository {
	return &HistoryRepository{
		historiesStorage: newStorage(mgoClient, 0, galleryDBName, historyColName),
	}
}

// Upsert caches a transfer in the memory storage
func (h *HistoryRepository) Upsert(pCtx context.Context, pNFTID persist.DBID, pHistory persist.OwnershipHistory) error {

	pHistory.NFTID = pNFTID

	if _, err := h.historiesStorage.upsert(pCtx, bson.M{"nft_id": pNFTID}, pHistory); err != nil {
		return err
	}
	return nil

}
