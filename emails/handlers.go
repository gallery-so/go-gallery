package emails

import (
	"html/template"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client, t *template.Template) *gin.Engine {

	sendGroup := router.Group("/send")
	sendGroup.POST("/notifications", sendNotificationEmails(loaders, queries, s))
	return router
}
