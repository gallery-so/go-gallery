package mongodb

import (
	"context"

	"github.com/mikeydub/go-gallery/persist"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var historyColName = "history"

// HistoryMongoRepository is a repository that stores collections in a MongoDB database
type HistoryMongoRepository struct {
	mp *storage
}

// NewHistoryMongoRepository creates a new instance of the collection mongo repository
func NewHistoryMongoRepository(mgoClient *mongo.Client) *HistoryMongoRepository {
	return &HistoryMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, historyColName),
	}
}

// Upsert caches a transfer in the memory storage
func (h *HistoryMongoRepository) Upsert(pCtx context.Context, pNFTID persist.DBID, pHistory *persist.OwnershipHistory) error {

	pHistory.NFTID = pNFTID

	if _, err := h.mp.upsert(pCtx, bson.M{"nft_id": pNFTID}, pHistory); err != nil {
		return err
	}
	return nil

}
