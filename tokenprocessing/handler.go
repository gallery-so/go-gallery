package tokenprocessing

import (
	"context"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/highlight"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
)

func handlersInitServer(ctx context.Context, router *gin.Engine, tp *tokenProcessor, mc *multichain.Provider, repos *postgres.Repositories, throttler *throttle.Locker, taskClient *task.Client, tokenManageCache *redis.Cache) *gin.Engine {
	// Handles retries and token state
	fastRetry := limiters.NewKeyRateLimiter(ctx, tokenManageCache, "tickFast", 1, 30*time.Second)
	slowRetry := limiters.NewKeyRateLimiter(ctx, tokenManageCache, "tickSlow", 1, 5*time.Minute)
	mintRetry := limiters.NewKeyRateLimiter(ctx, tokenManageCache, "tickMint", 1, 10*time.Second)
	refreshManager := tokenmanage.New(ctx, taskClient, tokenManageCache, tickTokenSync(ctx, fastRetry, slowRetry))
	syncManager := tokenmanage.NewWithRetries(ctx, taskClient, tokenManageCache, maxRetriesSync, tickTokenSync(ctx, fastRetry, slowRetry))
	highlightProvider := highlight.NewProvider(http.DefaultClient)
	mintManager := tokenmanage.New(ctx, taskClient, tokenManageCache, tickToken(ctx, mintRetry))

	mediaGroup := router.Group("/media")
	mediaGroup.POST("/process", func(c *gin.Context) {
		if hub := sentryutil.SentryHubFromContext(c); hub != nil {
			hub.Scope().AddEventProcessor(sentryutil.SpanFilterEventProcessor(c, 1000, 1*time.Millisecond, 8, true))
		}
		processBatch(tp, mc.Queries, taskClient, syncManager)(c)
	})
	mediaGroup.POST("/process/token", processMediaForTokenIdentifiers(tp, mc.Queries, refreshManager))
	mediaGroup.POST("/tokenmanage/process/token", processMediaForTokenManaged(tp, mc.Queries, taskClient, syncManager))
	mediaGroup.POST("/process/post-preflight", processPostPreflight(tp, mc, repos.UserRepository, taskClient, syncManager))
	mediaGroup.POST("/process/highlight-mint-claim", processHighlightMintClaim(mc, highlightProvider, tp, mintManager, taskClient, 10))

	authOpts := middleware.BasicAuthOptionBuilder{}

	ownersGroup := router.Group("/owners")
	// Return 200 on auth failures to prevent task/job retries
	ownersGroup.POST("/process/opensea", middleware.BasicHeaderAuthRequired(env.GetString("OPENSEA_WEBHOOK_SECRET"), authOpts.WithFailureStatus(http.StatusOK)), processOwnersForOpenseaTokens(mc, mc.Queries))
	ownersGroup.POST("/process/wallet-removal", processWalletRemoval(mc.Queries))

	contractsGroup := router.Group("/contracts")
	contractsGroup.POST("/detect-spam", detectSpamContracts(mc.Queries))

	return router
}

func tickToken(ctx context.Context, l *limiters.KeyRateLimiter) tokenmanage.TickToken {
	return func(td db.TokenDefinition) (time.Duration, error) {
		_, delay, err := l.ForKey(ctx, td.ID.String())
		return delay, err
	}
}

func tickTokenSync(ctx context.Context, fastRetry, slowRetry *limiters.KeyRateLimiter) tokenmanage.TickToken {
	return func(td db.TokenDefinition) (time.Duration, error) {
		if shareToGalleryEnabled(td) {
			_, delay, err := fastRetry.ForKey(ctx, td.ID.String())
			return delay, err
		}
		_, delay, err := slowRetry.ForKey(ctx, td.ID.String())
		return delay, err
	}
}

func maxRetriesSync(td db.TokenDefinition) int {
	if shareToGalleryEnabled(td) {
		return 24
	}
	return 2
}

func shareToGalleryEnabled(td db.TokenDefinition) bool {
	return platform.IsProhibition(td.Chain, td.ContractAddress) || td.IsFxhash
}
