package indexer

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
)

func getStatus(i *indexer, tokenRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 10*time.Second)
		defer cancel()

		mostRecent, _ := tokenRepository.MostRecentBlock(ctx)

		readableSyncMap := make(map[contractOwnerMethod]int)

		i.contractOwnerStats.Range(func(key, value interface{}) bool {
			readableSyncMap[key.(contractOwnerMethod)] = value.(int)
			return true
		})

		c.JSON(http.StatusOK, gin.H{
			"most_recent_blockchain": i.mostRecentBlock,
			"most_recent_db":         mostRecent,
			"last_synced_chunk":      i.lastSyncedChunk,
			"is_listening":           i.isListening,
			"contract_owner_stats":   readableSyncMap,
		})
	}
}
