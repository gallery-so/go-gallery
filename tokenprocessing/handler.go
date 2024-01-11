package tokenprocessing

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
)

func handlersInitServer(ctx context.Context, router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker, taskClient *task.Client, tokenManageCache *redis.Cache) *gin.Engine {
	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processBatch(tp, mc.Queries, taskClient, tokenManageCache)(c)
	})
	mediaGroup.POST("/process/token", processMediaForTokenIdentifiers(tp, mc.Queries, tokenmanage.New(ctx, taskClient, tokenManageCache)))
	mediaGroup.POST("/tokenmanage/process/token", processMediaForTokenManaged(tp, mc.Queries, taskClient, tokenManageCache))
	mediaGroup.POST("/process/post-preflight", processPostPreflight(tp, mc, repos.UserRepository, taskClient, tokenManageCache))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/user", processOwnersForUserTokens(mc, mc.Queries))
	ownersGroup.POST("/process/alchemy", processOwnersForAlchemyTokens(mc, mc.Queries))
	ownersGroup.POST("/process/opensea", processOwnersForOpenseaTokens(mc, mc.Queries))
	ownersGroup.POST("/process/wallet-removal", processWalletRemoval(mc.Queries))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))

	return router
}
