package tokenprocessing

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
)

const defaultSyncMaxRetries = 4

var (
	prohibitionContract = persist.NewContractIdentifiers("0x47a91457a3a1f700097199fd63c039c4784384ab", persist.ChainArbitrum)
	ensContract         = persist.NewContractIdentifiers(eth.EnsAddress, persist.ChainETH)
)

var contractSpecificRetries = map[persist.ContractIdentifiers]int{prohibitionContract: 24}

func handlersInitServer(ctx context.Context, router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker, taskClient *task.Client, tokenManageCache *redis.Cache) *gin.Engine {
	// Retry tokens that failed during syncs, but don't retry tokens that failed during manual refreshes
	noRetryManager := tokenmanage.New(ctx, taskClient, tokenManageCache)
	retryManager := tokenmanage.NewWithRetries(ctx, taskClient, tokenManageCache, syncMaxRetries)

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processBatch(tp, mc.Queries, retryManager)(c)
	})
	mediaGroup.POST("/process/token", processMediaForTokenIdentifiers(tp, mc.Queries, noRetryManager))
	mediaGroup.POST("/tokenmanage/process/token", processMediaForTokenManaged(tp, mc.Queries, retryManager))
	mediaGroup.POST("/process/post-preflight", processPostPreflight(tp, retryManager, mc, repos.UserRepository))
	ownersGroup := router.Group("/owners")
	ownersGroup.POST("/process/user", processOwnersForUserTokens(mc, mc.Queries))
	ownersGroup.POST("/process/alchemy", processOwnersForAlchemyTokens(mc, mc.Queries))
	ownersGroup.POST("/process/goldsky", processOwnersForGoldskyTokens(mc, mc.Queries))
	ownersGroup.POST("/process/wallet-removal", processWalletRemoval(mc.Queries))
	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))

	return router
}

func syncMaxRetries(tID persist.TokenIdentifiers) int {
	cID := persist.NewContractIdentifiers(tID.ContractAddress, tID.Chain)
	if retries, ok := contractSpecificRetries[cID]; ok {
		return retries
	}
	return defaultSyncMaxRetries
}
