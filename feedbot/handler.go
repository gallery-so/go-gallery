package feedbot

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shurcooL/graphql"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/util"
)

func handlersInit(router *gin.Engine, gql *graphql.Client) *gin.Engine {
	authOpts := middleware.BasicAuthOptionBuilder{}
	basicAuthHandler := middleware.BasicHeaderAuthRequired(env.GetString("FEEDBOT_SECRET"), authOpts.WithFailureStatus(http.StatusOK))
	router.GET("/ping", util.HealthCheckHandler())
	router.POST("/tasks/feed-event", middleware.TaskRequired(), basicAuthHandler, postToDiscord(gql))
	router.POST("/tasks/slack-post", middleware.TaskRequired(), basicAuthHandler, postToSlack(gql))
	return router
}
