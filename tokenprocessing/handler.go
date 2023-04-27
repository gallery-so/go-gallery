package tokenprocessing

import (
	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, mc *multichain.Provider, repos *postgres.Repositories, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", processMediaForUsersTokensOfChain(mc, repos.TokenRepository, repos.ContractRepository, repos.WalletRepository, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	mediaGroup.POST("/process/token", processMediaForToken(mc, repos.TokenRepository, repos.ContractRepository, repos.UserRepository, repos.WalletRepository, ethClient, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))
	return router
}
