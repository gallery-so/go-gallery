package indexer

import (
	"net/http"

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

func handlersInitServer(router *gin.Engine, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, ethClient *ethclient.Client, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, idxer *indexer) *gin.Engine {

	nftsGroup := router.Group("/nfts")
	nftsGroup.GET("/get/metadata", getTokenMetadata(tokenRepository, ipfsClient, ethClient, arweaveClient))

	contractsGroup := router.Group("/contracts")
	contractsGroup.GET("/get", getContract(contractRepository))
	contractsGroup.POST("/refresh", updateContractMetadata(contractRepository, ethClient, httpClient))

	return router
}
