package tokenprocessing

import (
	"context"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
)

func handlersInitServer(ctx context.Context, router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker, validator *validator.Validate, taskClient *cloudtasks.Client) *gin.Engine {
	// Retry tokens that failed during syncs, but don't retry tokens that failed during manual refreshes
	refreshManager := tokenmanage.New(ctx, taskClient)
	syncManager := tokenmanage.NewWithRetries(ctx, taskClient, 12)

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processMediaForUsersTokens(tp, repos.TokenRepository, repos.ContractRepository, syncManager)(c)
	})
	mediaGroup.POST("/process/token", processMediaForTokenIdentifiers(tp, repos.TokenRepository, repos.ContractRepository, repos.UserRepository, repos.WalletRepository, refreshManager))
	mediaGroup.POST("/process/token-id", processMediaForTokenInstance(tp, repos.TokenRepository, repos.ContractRepository, syncManager))
	mediaGroup.POST("/process/post-preflight", processPostPreflight(tp, syncManager, mc.Queries, mc, repos.ContractRepository, repos.UserRepository, repos.TokenRepository))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/contract", processOwnersForContractTokens(mc, repos.ContractRepository, throttler))
	ownersGroup.POST("/process/user", processOwnersForUserTokens(mc, mc.Queries, validator))
	ownersGroup.POST("/process/wallet-removal", processWalletRemoval(mc.Queries))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))

	return router
}
