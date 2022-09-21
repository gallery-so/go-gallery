package tokenprocessing

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, mediaQueue chan<- ProcessMediaInput, collectionTokensQueue chan<- ProcessCollectionTokensRefreshInput, throttler *throttle.Locker) *gin.Engine {
	media := router.Group("/media")
	media.POST("/process", processIncomingMedia(mediaQueue, throttler))

	collection := router.Group("/collection")
	collectionTokens := collection.Group("/tokens")
	collectionTokens.POST("/refresh", processIncomingCollectionTokensRefresh(collectionTokensQueue, throttler))
	return router
}
