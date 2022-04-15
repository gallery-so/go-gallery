package indexer

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
)

func getStatus(i *Indexer, tokenRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 10*time.Second)
		defer cancel()

		mostRecent, _ := tokenRepository.MostRecentBlock(ctx)

		c.JSON(http.StatusOK, gin.H{
			"most_recent_blockchain": i.mostRecentBlock,
			"most_recent_db":         mostRecent,
			"last_synced_block":      i.lastSyncedBlock,
			"is_listening":           i.isListening,
		})
	}
}
