package feedbot

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/event/cloudtask"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

func handleMessage(repos persist.Repositories, gql *graphql.Client) gin.HandlerFunc {
	builder := NewQueryBuilder(repos, gql)

	return func(c *gin.Context) {
		msg := cloudtask.EventMessage{}
		if err := c.ShouldBindJSON(&msg); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		query, err := builder.NewQuery(c, msg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		handled, err := feedPosts.SearchFor(c, query)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if !handled {
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

func markSent(ctx context.Context, repos persist.Repositories, msg cloudtask.EventMessage) error {
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
