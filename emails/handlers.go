package emails

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client, r *redis.Client) *gin.Engine {
	sendGroup := router.Group("/send")

	sendGroup.POST("/notifications", middleware.AdminRequired(), adminSendNotificationEmail(queries, s))

	verificationLimiter := middleware.RateLimited(middleware.NewKeyRateLimiter(1, time.Second*5, r))
	sendGroup.POST("/verification", verificationLimiter, sendVerificationEmail(loaders, queries, s))

	router.POST("/subscriptions", updateSubscriptions(queries))
	router.POST("/unsubscribe", unsubscribe(queries))
	router.POST("/resubscribe", resubscribe(queries))

	router.POST("/verify", verifyEmail(queries))
	preVerifyLimiter := middleware.RateLimited(middleware.NewKeyRateLimiter(1, time.Millisecond*500, r))
	router.GET("/preverify", preVerifyLimiter, preverifyEmail())
	return router
}
