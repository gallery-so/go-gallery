package feedbot

import (
	"errors"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/util"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/shurcooL/graphql"
)

// TODO: Use middleware.BasicHeaderAuthRequired. Requires reworking the sender to use
// the appropriate header/encoding.
func authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		creds := c.Request.Header.Get("Authorization")
		if creds != "Basic "+env.GetString("FEEDBOT_SECRET") {
			// Return 200 on auth failures to prevent task retries
			c.AbortWithError(http.StatusOK, errors.New("unauthorized request"))
			return
		}
	}
}

func handlersInit(router *gin.Engine, gql *graphql.Client) *gin.Engine {
	router.GET("/ping", util.HealthCheckHandler())
	router.POST("/tasks/feed-event", middleware.TaskRequired(), authRequired(), handleMessage(gql))
	return router
}
