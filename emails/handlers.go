package emails

import (
	htmltemplate "html/template"
	plaintemplate "text/template"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/sendgrid/sendgrid-go"
)

func handlersInitServer(router *gin.Engine, loaders *dataloader.Loaders, queries *coredb.Queries, s *sendgrid.Client, htmltemplates *htmltemplate.Template, plaintemplates *plaintemplate.Template) *gin.Engine {

	sendGroup := router.Group("/send")
	sendGroup.POST("/notifications", sendNotificationEmails(queries, s, htmltemplates, plaintemplates))
	sendGroup.POST("/verification", sendVerificationEmail(loaders, queries, s, htmltemplates, plaintemplates))

	router.POST("/verify", verifyEmail(queries))
	return router
}
