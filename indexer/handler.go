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

func handlersInitServer(router *gin.Engine, queueChan chan processTokensInput, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, idxer *indexer) *gin.Engine {

	nftsGroup := router.Group("/nfts")
	nftsGroup.POST("/refresh", updateTokens(tokenRepository, ethClient, ipfsClient, arweaveClient))
	nftsGroup.POST("/validate", validateWalletsNFTs(tokenRepository, contractRepository, ethClient, ipfsClient, arweaveClient))
	nftsGroup.GET("/get", getTokens(queueChan, tokenRepository, contractRepository, ipfsClient, ethClient, arweaveClient))
	nftsGroup.GET("/get/metadata", getTokenMetadata(tokenRepository, ipfsClient, ethClient, arweaveClient))

	contractsGroup := router.Group("/contracts")
	contractsGroup.GET("/get", getContract(contractRepository))
	contractsGroup.POST("/refresh", updateContractMetadata(contractRepository, ethClient))

	tasksGroup := router.Group("/tasks")
	tasksGroup.POST("refresh", processRefreshes(idxer, storageClient))

	return router
}
