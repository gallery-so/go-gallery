package tokenprocessing

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
)

func handlersInitServer(router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker, validator *validator.Validate, tm *tokenmanage.Manager) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processMediaForUsersTokens(tp, repos.TokenRepository, repos.ContractRepository, tm)(c)
	})
	mediaGroup.POST("/process/token", processMediaForTokenIdentifiers(tp, repos.TokenRepository, repos.ContractRepository, repos.UserRepository, repos.WalletRepository, tm))
	mediaGroup.POST("/process/token-id", processMediaForTokenInstance(tp, repos.TokenRepository, repos.ContractRepository, tm))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	ownersGroup.POST("/process/user", processOwnersForUserTokens(mc, mc.Queries, validator))
	ownersGroup.POST("/process/wallet-removal", processWalletRemoval(mc.Queries))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))

	return router
}
