package feedbot

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/event/cloudtask"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

type UserQueryForQuery struct {
	UserOrError struct {
		User struct {
			Name string
		} `graphql:"...on GalleryUser"`
	} `grappql:"userById(id: $id}"`
}

type NftQueryForQuery struct {
	NftOrError struct {
		Nft struct {
			Name  string
			Owner struct {
				User struct {
					Username string
				} `graphql:"...on GalleryUser"`
			}
		} `graphql:"...on Nft"`
	} `graphql:"nftById(id: $id)"`
}

type CollectionQueryForQuery struct {
	CollectionOrError struct {
		Collection struct {
			Name    string
			Gallery struct {
				Owner struct {
					Username string
				}
			}
			Nfts []struct {
				Id   string
				Name string
			}
		} `graphql:":...on Collection"`
	} `graphql:"collectionById(id: $id)"`
}

func handleMessage(repos persist.Repositories, gql *graphql.Client) gin.HandlerFunc {
	rules := newFeedRules()

	return func(c *gin.Context) {
		msg := cloudtask.EventMessage{}
		if err := c.ShouldBindJSON(&msg); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		query, err := newQueryFromMessage(ctx, repos, gql, msg)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		matches := rules.matchOn(query)
		if len(matches) == 0 {
			c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event(%s) matched no rules", msg.ID)})
			return
		}

		// Only using the first match
		if err := matches[1].Handle(ctx, query); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := markSent(c, repos, msg); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("event(%s) processed", msg.ID)})
	}
}

func newQueryFromMessage(ctx context.Context, repos persist.Repositories, gql *graphql.Client, msg cloudtask.EventMessage) (Query, error) {
	switch persist.CategoryFromEventCode(msg.EventCode) {
	case persist.UserEventCode:
		return fromUserEvent(ctx, repos, gql, msg)
	case persist.NftEventCode:
		return fromNftEvent(ctx, repos, gql, msg)
	case persist.CollectionEventCode:
		return fromCollectionEvent(ctx, repos, gql, msg)
	default:
		return Query{}, fmt.Errorf("failed to make query, got unknown event: %v", msg.EventCode)
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

func fromUserEvent(ctx context.Context, repos persist.Repositories, gql *graphql.Client, msg cloudtask.EventMessage) (Query, error) {
	event, err := repos.UserEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := repos.UserEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := repos.UserEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var (
		userQuery     UserQueryForQuery
		followedQuery UserQueryForQuery
	)

	if err := gql.Query(ctx, userQuery, map[string]interface{}{"id": event.UserID}); err != nil {
		logger.For(ctx).Errorf("failed to fetch additional user context: %v", err)
	}

	if event.Data.FollowedUserID != "" {
		if err := gql.Query(ctx, followedQuery, map[string]interface{}{"id": event.Data.FollowedUserID}); err != nil {
			logger.For(ctx).Errorf("failed to fetch additional followee context: %v", err)
		}
	}

	return Query{
		EventID:          msg.ID,
		EventCode:        msg.EventCode,
		EventsSince:      len(eventsSince),
		UserID:           event.UserID,
		Username:         userQuery.UserOrError.User.Name,
		FollowedUserID:   event.Data.FollowedUserID,
		FollowedUsername: followedQuery.UserOrError.User.Name,
		LastUserEvent:    lastEvent,
	}, nil
}

func fromNftEvent(ctx context.Context, repos persist.Repositories, gql *graphql.Client, msg cloudtask.EventMessage) (Query, error) {
	event, err := repos.NftEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := repos.NftEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := repos.NftEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var q NftQueryForQuery

	if err := gql.Query(ctx, q, map[string]interface{}{"id": event.NftID}); err == nil {
		logger.For(ctx).Errorf("failed to fetch additional nft context: %v", err)
	}

	return Query{
		EventID:           msg.ID,
		EventCode:         msg.EventCode,
		EventsSince:       len(eventsSince),
		UserID:            event.UserID,
		Username:          q.NftOrError.Nft.Owner.User.Username,
		NftID:             event.NftID,
		NftName:           q.NftOrError.Nft.Name,
		NftCollectorsNote: event.Data.CollectorsNote.String(),
		CollectionID:      event.Data.CollectionID,
		LastNftEvent:      lastEvent,
	}, nil
}

func fromCollectionEvent(ctx context.Context, repos persist.Repositories, gql *graphql.Client, msg cloudtask.EventMessage) (Query, error) {
	event, err := repos.CollectionEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := repos.CollectionEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := repos.CollectionEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var q CollectionQueryForQuery

	if err := gql.Query(ctx, q, map[string]interface{}{"id": event.CollectionID}); err != nil {
		logger.For(ctx).Errorf("failed to fetch additional nft context: %v", err)
	}

	return Query{
		EventID:                  msg.ID,
		EventCode:                msg.EventCode,
		EventsSince:              len(eventsSince),
		UserID:                   event.UserID,
		Username:                 q.CollectionOrError.Collection.Gallery.Owner.Username,
		CollectionID:             event.CollectionID,
		CollectionName:           q.CollectionOrError.Collection.Name,
		CollectionNfts:           q.CollectionOrError.Collection.Nfts,
		CollectionCollectorsNote: event.Data.CollectorsNote.String(),
		LastCollectionEvent:      lastEvent,
	}, nil
}

func ping() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ping": "pong"})
	}
}
