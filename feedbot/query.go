package feedbot

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/shurcooL/graphql"
)

type Query struct {
	EventID           persist.DBID
	EventCode         persist.EventCode
	EventsSince       int
	UserID            persist.DBID
	Username          string
	NftID             persist.DBID
	NftName           string
	NftCollectorsNote string
	CollectionID      persist.DBID
	CollectionName    string
	CollectionNfts    []struct {
		Nft struct {
			Dbid persist.DBID
			Name string
		}
	}
	CollectionCollectorsNote string
	FollowedUserID           persist.DBID
	FollowedUsername         string
	LastUserEvent            *persist.UserEventRecord
	LastNftEvent             *persist.NftEventRecord
	LastCollectionEvent      *persist.CollectionEventRecord
}

type QueryBuilder struct {
	repos persist.Repositories
	gql   *graphql.Client
}

func (qb *QueryBuilder) NewQuery(ctx context.Context, msg task.EventMessage) (Query, error) {
	switch persist.CategoryFromEventCode(msg.EventCode) {
	case persist.UserEventCode:
		return qb.fromUserEvent(ctx, msg)
	case persist.NftEventCode:
		return qb.fromNftEvent(ctx, msg)
	case persist.CollectionEventCode:
		return qb.fromCollectionEvent(ctx, msg)
	default:
		return Query{}, fmt.Errorf("failed to make query, got unknown event: %v", msg.EventCode)
	}
}

func (q *QueryBuilder) fromUserEvent(ctx context.Context, msg task.EventMessage) (Query, error) {
	event, err := q.repos.UserEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := q.repos.UserEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := q.repos.UserEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var userQuery UserQueryForQuery
	var followedQuery UserQueryForQuery

	if err := q.gql.Query(ctx, &userQuery, map[string]interface{}{"id": event.UserID}); err != nil {
		logger.For(ctx).Errorf("failed to fetch additional user context: %v", err)
	}

	if event.Data.FollowedUserID != "" {
		if err := q.gql.Query(ctx, &followedQuery, map[string]interface{}{"id": event.Data.FollowedUserID}); err != nil {
			logger.For(ctx).Errorf("failed to fetch additional followee context: %v", err)
		}
	}

	return Query{
		EventID:          msg.ID,
		EventCode:        msg.EventCode,
		EventsSince:      len(eventsSince),
		UserID:           event.UserID,
		Username:         userQuery.UserOrError.User.Username,
		FollowedUserID:   event.Data.FollowedUserID,
		FollowedUsername: followedQuery.UserOrError.User.Username,
		LastUserEvent:    lastEvent,
	}, nil
}

func (qb *QueryBuilder) fromNftEvent(ctx context.Context, msg task.EventMessage) (Query, error) {
	event, err := qb.repos.NftEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := qb.repos.NftEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := qb.repos.NftEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var q NftQueryForQuery

	if err := qb.gql.Query(ctx, &q, map[string]interface{}{"id": event.NftID}); err != nil {
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

func (qb *QueryBuilder) fromCollectionEvent(ctx context.Context, msg task.EventMessage) (Query, error) {
	event, err := qb.repos.CollectionEventRepository.Get(ctx, msg.ID)
	if err != nil {
		return Query{}, err
	}

	eventsSince, err := qb.repos.CollectionEventRepository.GetEventsSince(ctx, event, time.Now())
	if err != nil {
		return Query{}, err
	}

	lastEvent, err := qb.repos.CollectionEventRepository.GetEventBefore(ctx, event)
	if err != nil {
		return Query{}, err
	}

	var q CollectionQueryForQuery

	if err := qb.gql.Query(ctx, &q, map[string]interface{}{"id": event.CollectionID}); err != nil {
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

type UserQueryForQuery struct {
	UserOrError struct {
		User struct {
			Username string
		} `graphql:"...on GalleryUser"`
	} `graphql:"userById(id: $id)"`
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
				Nft struct {
					Dbid persist.DBID
					Name string
				}
			}
		} `graphql:"...on Collection"`
	} `graphql:"collectionById(id: $id)"`
}
