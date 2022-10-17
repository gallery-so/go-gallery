package tokenprocessing

import (
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, mc *multichain.Provider, repos *persist.Repositories, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", processMediaForUsersTokensOfChain(repos.TokenRepository, repos.ContractRepository, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	mediaGroup.POST("/process/token", processMediaForToken(repos.TokenRepository, repos.UserRepository, repos.WalletRepository, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	return router
}
