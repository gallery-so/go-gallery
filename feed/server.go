package feed

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

func handleEvent(queries *db.Queries, taskClient *task.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		message := task.FeedMessage{}

		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		builder := NewEventBuilder(queries)
		event, err := builder.NewFeedEventFromTask(c.Request.Context(), message)

		if err != nil {
			logger.For(c).WithFields(logrus.Fields{"eventID": message.ID}).Debugf("failed to handle event: %s", err)

			if errors.Is(err, errUnhandledSingleEvent) || errors.Is(err, errUnhandledGroupedEvent) {
				c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s is unhandable", message.ID)})
				return
			}

			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if event == nil {
			logger.For(c).WithFields(logrus.Fields{"eventID": message.ID}).Debug("event had no matches")
			c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s had no matches", message.ID)})
			return
		}

		// Send event to feedbot
		err = taskClient.CreateTaskForFeedbot(c.Request.Context(), task.FeedbotMessage{FeedEventID: event.ID, Action: event.Action})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		logger.For(c).WithFields(logrus.Fields{"eventID": message.ID}).Debug("event processed")
		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s processed", message.ID)})
	}
}
