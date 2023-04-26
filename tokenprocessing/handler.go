package tokenprocessing

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", processMediaForUsersTokensOfChain(tp, repos.TokenRepository, repos.ContractRepository, throttler))
	mediaGroup.POST("/process/token", processMediaForToken(tp, repos.TokenRepository, repos.ContractRepository, repos.UserRepository, repos.WalletRepository, throttler))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))
	return router
}
