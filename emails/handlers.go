package emails

import (
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client) *gin.Engine {

	sendGroup := router.Group("/send")
	sendGroup.POST("/notifications", sendNotificationEmails(queries, s))
	sendGroup.POST("/verification", sendVerificationEmail(loaders, queries, s))

	sendGroup.POST("/unsubscribe", unsubscribeFromEmailType(queries))
	sendGroup.POST("/resubscribe", resubscribeFromEmailType(queries))

	router.POST("/verify", verifyEmail(queries))
	return router
}
