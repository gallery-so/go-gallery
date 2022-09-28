package mediaprocessing

import (
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, queue chan<- ProcessMediaInput, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) *gin.Engine {
	router.GET("/keepalive", keepAlive())

	router.POST("/process", processMediaForUsersTokensOfChain(queue, tokenRepo, contractRepo, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	return router
}
