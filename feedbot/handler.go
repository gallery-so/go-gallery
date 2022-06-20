package feedbot

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shurcooL/graphql"
	"github.com/spf13/viper"
)

type errBadTaskRequest struct {
	msg string
}

func (e errBadTaskRequest) Error() string {
	return fmt.Sprintf("bad task request: %s", e.msg)
}

// TaskRequired checks that the request comes from Cloud Tasks and does a basic auth check.
// Returns a 200 status to remove the message from the queue if it is a bad request.
func TaskRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskName := c.Request.Header.Get("X-Appengine-Taskname")
		if taskName == "" {
			c.AbortWithError(http.StatusOK, errBadTaskRequest{"invalid task"})
			return
		}

		queueName := c.Request.Header.Get("X-Appengine-Queuename")
		if queueName == "" {
			c.AbortWithError(http.StatusOK, errBadTaskRequest{"invalid queue"})
			return
		}

		creds := c.Request.Header.Get("Authorization")
		if creds != "Basic "+viper.GetString("FEEDBOT_SECRET") {
			c.AbortWithError(http.StatusOK, errors.New("unauthorized request"))
			return
		}
	}
}

func handlersInit(router *gin.Engine, gql *graphql.Client) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-event", TaskRequired(), handleMessage(gql))
	return router
}
