package emails

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) *gin.Engine {

	sendGroup := router.Group("/send")
	if isDevEnv() {
		sendGroup.POST("/notifications", middleware.RateLimited(), sendNotificationEmails(queries, s))
	}
	sendGroup.POST("/verification", middleware.RateLimited(), sendVerificationEmail(loaders, queries, s))

	router.POST("/subscriptions", updateSubscriptions(queries))
	router.POST("/unsubscribe", unsubscribe(queries))
	router.POST("/resubscribe", resubscribe(queries))

	router.POST("/verify", verifyEmail(queries))
	router.GET("/preverify", preverifyEmail())
	return router
}
