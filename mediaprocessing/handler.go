package mediaprocessing

import (
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, repos *persist.Repositories, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, tokenBucket string, throttler *throttle.Locker) *gin.Engine {
	router.POST("/process", processMediaForUsersTokensOfChain(repos.TokenRepository, repos.ContractRepository, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	router.POST("/process/token", processMediaForToken(repos.TokenRepository, repos.UserRepository, repos.WalletRepository, ipfsClient, arweaveClient, stg, tokenBucket, throttler))
	return router
}
