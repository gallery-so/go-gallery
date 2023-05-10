package tokenprocessing

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
)

func handlersInitServer(router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processMediaForUsersTokens(tp, repos.TokenRepository, repos.ContractRepository, throttler)(c)
	})
	mediaGroup.POST("/process/token", processMediaForToken(tp, repos.TokenRepository, repos.ContractRepository, repos.UserRepository, repos.WalletRepository, throttler))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))
	return router
}
