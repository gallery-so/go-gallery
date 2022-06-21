package feedbot

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

func handleMessage(gql *graphql.Client) gin.HandlerFunc {
	discordHandler := PostRenderSender{PostRenderer: PostRenderer{gql}}
	return func(c *gin.Context) {
		message := task.FeedbotMessage{}

		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		err := discordHandler.RenderAndSend(c.Request.Context(), message)
		if err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("event=%s processed", message.FeedEventID)})
	}
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}
