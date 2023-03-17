package feed

import (
	"context"
	"errors"
	"net/http"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
)

// TaskRequired checks that the request came from Cloud Tasks.
// Returns a 200 status in order to remove bad messages from the task queue.
func taskRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskName := c.Request.Header.Get("X-CloudTasks-TaskName")
		if taskName == "" {
			c.AbortWithError(http.StatusOK, errors.New("invalid task"))
			return
		}

		queueName := c.Request.Header.Get("X-CloudTasks-QueueName")
		if queueName == "" {
			c.AbortWithError(http.StatusOK, errors.New("invalid queue"))
			return
		}

		creds := c.Request.Header.Get("Authorization")
		if creds != "Basic "+env.Get[string](context.Background(), "FEED_SECRET") {
			c.AbortWithError(http.StatusOK, errors.New("unauthorized request"))
			return
		}
	}
}

func handlersInit(router *gin.Engine, queries *db.Queries, taskClient *cloudtasks.Client) *gin.Engine {
	router.GET("/ping", ping())
	router.POST("/tasks/feed-event", taskRequired(), handleEvent(queries, taskClient))
	return router
}
