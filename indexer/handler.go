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

	nftsGroup := router.Group("/nfts")
	nftsGroup.POST("/refresh", updateTokens(tokenRepository, ethClient, ipfsClient, arweaveClient, storageClient))
	nftsGroup.POST("/validate", validateWalletsNFTs(tokenRepository, contractRepository, ethClient, ipfsClient, arweaveClient, storageClient))
	nftsGroup.GET("/get", getTokens(tokenRepository, contractRepository, ipfsClient, ethClient, arweaveClient, storageClient))

	contractsGroup := router.Group("/contracts")
	contractsGroup.GET("/get", getContract(contractRepository))
	contractsGroup.POST("/refresh", updateContractMedia(contractRepository, ethClient))

	return router
}
