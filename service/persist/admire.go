package persist

import (
	"context"
	"fmt"
	"time"
)

type Admire struct {
	ID          DBID      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	LastUpdated time.Time `json:"last_updated"`
	FeedEventID DBID      `json:"feed_event_id"`
	ActorID     DBID      `json:"actor_id"`
	Deleted     bool      `json:"deleted"`
}

type AdmireRepository interface {
	CreateAdmire(ctx context.Context, feedEventID DBID, actorID DBID) (DBID, error)
	CreateTokenAdmire(ctx context.Context, tokenID DBID) (DBID, error)
	RemoveAdmire(ctx context.Context, admireID DBID) error
}

type ErrAdmireNotFound struct {
	AdmireID    DBID
	ActorID     DBID
	FeedEventID DBID
	PostID      DBID
}

func (e ErrAdmireNotFound) Error() string {
	return fmt.Sprintf("admire not found | AdmireID: %s, ActorID: %s, FeedEventID: %s, PostID: %s", e.AdmireID, e.ActorID, e.FeedEventID, e.PostID)
}

type ErrAdmireAlreadyExists struct {
	AdmireID    DBID
	ActorID     DBID
	FeedEventID DBID
	PostID      DBID
}

type ErrAdmireTokenAlreadyExists struct {
	AdmireID    DBID
	ActorID     DBID
	TokenID 	DBID
}

func (e ErrAdmireAlreadyExists) Error() string {
	return fmt.Sprintf("admire already exists | AdmireID: %s, ActorID: %s, FeedEventID: %s, PostID: %s", e.AdmireID, e.ActorID, e.FeedEventID, e.PostID)
}

func (e ErrAdmireTokenAlreadyExists) Error() string {
	return fmt.Sprintf("admire token already exists | AdmireID: %s, ActorID: %s, TokenID: %s", e.AdmireID, e.ActorID, e.TokenID)
}
