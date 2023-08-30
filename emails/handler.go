package emails

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sendgrid/sendgrid-go"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/redis"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) *gin.Engine {
	sendGroup := router.Group("/send")

	sendGroup.POST("/notifications", middleware.AdminRequired(), adminSendNotificationEmail(queries, s))

	// Return 200 on auth failures to prevent task/job retries
	authOpts := middleware.BasicAuthOptionBuilder{}
	basicAuthHandler := middleware.BasicHeaderAuthRequired(env.GetString("EMAIL_TASK_SECRET"), authOpts.WithFailureStatus(http.StatusOK))
	sendGroup.POST("/process/add-to-mailing-list", basicAuthHandler, middleware.TaskRequired(), processAddToMailingList(queries))

	limiterCtx := context.Background()
	limiterCache := redis.NewCache(redis.EmailRateLimitersCache)

	verificationLimiter := limiters.NewKeyRateLimiter(limiterCtx, limiterCache, "verification", 1, time.Second*5)
	sendGroup.POST("/verification", middleware.IPRateLimited(verificationLimiter), sendVerificationEmail(loaders, queries, s))

	router.POST("/subscriptions", updateSubscriptions(queries))
	router.POST("/unsubscribe", unsubscribe(queries))
	router.POST("/resubscribe", resubscribe(queries))

	router.POST("/verify", verifyEmail(queries))
	preverifyLimiter := limiters.NewKeyRateLimiter(limiterCtx, limiterCache, "preverify", 1, time.Millisecond*500)
	router.GET("/preverify", middleware.IPRateLimited(preverifyLimiter), preverifyEmail())
	return router
}
