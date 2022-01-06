package indexer

import (
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
)

func handlersInit(router *gin.Engine, i *Indexer, tokenRepository persist.TokenRepository, userRepository persist.UserRepository, tq *task.Queue, ethClient *ethclient.Client, ipfsClient *shell.Shell, storageClient *storage.Client) *gin.Engine {
	router.GET("/status", getStatus(i, tokenRepository))

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/update", updateMedia(tq, tokenRepository, ethClient, ipfsClient, storageClient))

	nftsGroup := router.Group("/nfts")
	nftsGroup.POST("/validate", validateUsersNFTs(tokenRepository, userRepository))

	return router
}
