package feed

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
)

func handleEvent() gin.HandlerFunc {
	return func(c *gin.Context) {
		message := task.FeedMessage{}

		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		builder := NewEventBuilder()
		event, err := builder.NewEvent(c.Request.Context(), message)

		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if event == nil {
			c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s had no matches", message.ID)})
		}

		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s processed", message.ID)})
	}
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}
