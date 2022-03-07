package feedbot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/util"
)

type RetryableTasKError interface {
	Retryable() bool
	Error() string
}

var errInvalidEvent = errors.New("unknown event type")

func handleMessage(eventRepos *event.EventRepositories) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := event.EventMessage{}
		if retried := retryTask(c, c.ShouldBindJSON(&input)); retried {
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		switch event.GetCategoryFromEventTypeID(input.EventTypeID) {
		case event.UserEventType:
			err := handleUserEvents(ctx, eventRepos.UserEventRepository, input)
			if retried := retryTask(c, err); retried {
				return
			}
		case event.TokenEventType:
			err := handleTokenEvents(ctx, eventRepos.TokenEventRepository, input)
			if retried := retryTask(c, err); retried {
				return
			}
		case event.CollectionEventType:
			err := handleCollectionEvents(ctx, eventRepos.CollectionEventRepository, input)
			if retried := retryTask(c, err); retried {
				return
			}
		default:
			util.ErrResponse(c, http.StatusBadRequest, errInvalidEvent)
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event(%s) processed", input.ID)})
	}
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}

func retryTask(c *gin.Context, err error) bool {
	if err != nil {
		if re, ok := err.(RetryableTasKError); ok && re.Retryable() {
			// Statuses other than 2xx and 503 are retried.
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return true
		}
		c.JSON(http.StatusOK, err)
		return true
	}
	return false
}
