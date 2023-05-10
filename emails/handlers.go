package emails

import (
	"context"
	"github.com/mikeydub/go-gallery/service/redis"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) *gin.Engine {
	sendGroup := router.Group("/send")

	sendGroup.POST("/notifications", middleware.AdminRequired(), adminSendNotificationEmail(queries, s))

	limiterCtx := context.Background()
	limiterCache := redis.NewCache(redis.EmailRateLimitersCache)

	verificationLimiter := middleware.NewKeyRateLimiter(limiterCtx, limiterCache, "verification", 1, time.Second*5)
	sendGroup.POST("/verification", middleware.IPRateLimited(verificationLimiter), sendVerificationEmail(loaders, queries, s))

	router.POST("/subscriptions", updateSubscriptions(queries))
	router.POST("/unsubscribe", unsubscribe(queries))
	router.POST("/resubscribe", resubscribe(queries))

	router.POST("/verify", verifyEmail(queries))
	preverifyLimiter := middleware.NewKeyRateLimiter(limiterCtx, limiterCache, "preverify", 1, time.Millisecond*500)
	router.GET("/preverify", middleware.IPRateLimited(preverifyLimiter), preverifyEmail())
	return router
}
