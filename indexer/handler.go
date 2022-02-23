package indexer

import (
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
)

func handlersInit(router *gin.Engine, i *Indexer, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, tq *task.Queue, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) *gin.Engine {
	router.GET("/status", getStatus(i, tokenRepository))

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/update", updateMedia(tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient))

	nftsGroup := router.Group("/nfts")
	nftsGroup.POST("/validate", validateUsersNFTs(tokenRepository, contractRepository, userRepository, ethClient, ipfsClient, arweaveClient, storageClient))

	return router
}
