package emails

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/Khan/genqlient/graphql"
	"github.com/bsm/redislock"
	"github.com/gin-gonic/gin"
	"github.com/sendgrid/sendgrid-go"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/notifications"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/store"
	"github.com/mikeydub/go-gallery/service/task"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, q *coredb.Queries, s *sendgrid.Client, r *redis.Cache, b *store.BucketStorer, psub *pubsub.Client, t *task.Client, notifLock *redislock.Client, gql *graphql.Client) *gin.Engine {
	sendGroup := router.Group("/send")

	sendGroup.POST("/notifications", middleware.AdminRequired(), adminSendNotificationEmail(q, s))

	// Return 200 on auth failures to prevent task/job retries
	authOpts := middleware.BasicAuthOptionBuilder{}
	basicAuthHandler := middleware.BasicHeaderAuthRequired(env.GetString("EMAILS_TASK_SECRET"), authOpts.WithFailureStatus(http.StatusOK))
	sendGroup.POST("/process/add-to-mailing-list", basicAuthHandler, middleware.TaskRequired(), processAddToMailingList(q))

	limiterCtx := context.Background()
	limiterCache := redis.NewCache(redis.EmailRateLimitersCache)

	verificationLimiter := limiters.NewKeyRateLimiter(limiterCtx, limiterCache, "verification", 1, time.Second*5)
	sendGroup.POST("/verification", middleware.IPRateLimited(verificationLimiter), sendVerificationEmail(loaders, q, s))

	router.POST("/unsubscriptions", updateUnsubscriptions(q))
	router.GET("/unsubscriptions", getUnsubscriptions(q))
	router.POST("/unsubscribe", unsubscribe(q))
	router.POST("/resubscribe", resubscribe(q))

	router.POST("/verify", verifyEmail(q))
	preverifyLimiter := limiters.NewKeyRateLimiter(limiterCtx, limiterCache, "preverify", 1, time.Millisecond*500)
	router.GET("/preverify", middleware.IPRateLimited(preverifyLimiter), preverifyEmail())

	digestGroup := router.Group("/digest")
	digestGroup.GET("/values", middleware.RetoolAuthRequired, getDigestValues(q, b, gql))
	digestGroup.POST("/values", middleware.RetoolAuthRequired, updateDigestValues(b))
	digestGroup.POST("/send", middleware.CloudSchedulerMiddleware, sendDigestEmails(q, s, r, b, gql))
	digestGroup.POST("/send-test", middleware.RetoolAuthRequired, sendDigestTestEmail(q, s, b, gql))

	notificationsGroup := router.Group("/notifications")
	notificationsGroup.POST("/send", middleware.CloudSchedulerMiddleware, sendNotificationEmails(q, s, r))
	notificationsGroup.POST("/announcement", middleware.RetoolAuthRequired, useEventHandler(q, psub, t, notifLock), sendAnnouncementNotification(q))

	return router
}

func useEventHandler(q *coredb.Queries, p *pubsub.Client, t *task.Client, l *redislock.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		event.AddTo(c, false, notifications.New(q, p, t, l, false), q, t, nil)
		c.Next()
	}
}
