package mediaprocessing

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, queue chan<- ProcessMediaInput, throttler *throttle.Locker) *gin.Engine {
	router.POST("/process", processIPFSMetadata(queue, throttler))
	return router
}
