package indexer

import (
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
)

func handlersInit(router *gin.Engine, i *indexer, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) *gin.Engine {
	router.GET("/status", getStatus(i, tokenRepository))

	return router
}

func handlersInitServer(router *gin.Engine, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) *gin.Engine {

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/update", updateMedia(tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient))

	nftsGroup := router.Group("/nfts")
	nftsGroup.POST("/validate", validateWalletsNFTs(tokenRepository, contractRepository, ethClient, ipfsClient, arweaveClient, storageClient))
	nftsGroup.GET("/get", getTokens(tokenRepository, ipfsClient, ethClient))

	return router
}
