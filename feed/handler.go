package feed

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// TaskRequired checks that the request came from Cloud Tasks.
// Returns a 200 status in order to remove bad messages from the task queue.
func taskRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskName := c.Request.Header.Get("X-Appengine-Taskname")
		if taskName == "" {
			c.AbortWithError(http.StatusOK, errors.New("invalid task"))
			return
		}

		queueName := c.Request.Header.Get("X-Appengine-Queuename")
		if queueName == "" {
			c.AbortWithError(http.StatusOK, errors.New("invalid queue"))
			return
		}

		creds := c.Request.Header.Get("Authorization")
		if creds != "Basic "+viper.GetString("FEED_SECRET") {
			c.AbortWithError(http.StatusOK, errors.New("unauthorized request"))
			return
		}
	}
}

func handlersInit(router *gin.Engine) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-event", taskRequired(), handleEvent())
	return router
}
