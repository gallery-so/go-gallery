package feedbot

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

func handleMessage(repos persist.Repositories, gql *graphql.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		msg := task.EventMessage{}
		if err := c.ShouldBindJSON(&msg); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		builder := QueryBuilder{repos, gql}
		query, err := builder.NewQuery(c.Request.Context(), msg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if handled, err := feedPosts.SearchFor(c.Request.Context(), query); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		} else if !handled {
			c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s matched no rules", msg.ID)})
			return
		}

		if err := markSent(c, repos, msg); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event=%s processed", msg.ID)})
	}
}

func markSent(ctx context.Context, repos persist.Repositories, msg task.EventMessage) error {
	switch persist.CategoryFromEventCode(msg.EventCode) {
	case persist.UserEventCode:
		return repos.UserEventRepository.MarkSent(ctx, msg.ID)
	case persist.NftEventCode:
		return repos.NftEventRepository.MarkSent(ctx, msg.ID)
	case persist.CollectionEventCode:
		return repos.CollectionEventRepository.MarkSent(ctx, msg.ID)
	default:
		return fmt.Errorf("failed to mark event as sent, got unknown event: %v", msg.EventCode)
	}
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}
