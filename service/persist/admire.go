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
}

type ErrAdmireFeedEventNotFound struct {
	AdmireID    DBID
	ActorID     DBID
	FeedEventID DBID
}

type ErrAdmirePostNotFound struct {
	AdmireID    DBID
	ActorID     DBID
	PostID		DBID
}


type ErrAdmireTokenNotFound struct {
	AdmireID    DBID
	ActorID     DBID
	TokenID		DBID
}

func (e ErrAdmireNotFound) Error() string {
	return fmt.Sprintf("admire not found | AdmireID: %s, ActorID: %s", e.AdmireID, e.ActorID)
}

func (e ErrAdmireFeedEventNotFound) Error() string {
	return fmt.Sprintf("admire feed event not found | AdmireID: %s, ActorID: %s, FeedEventID: %s", e.AdmireID, e.ActorID, e.FeedEventID)
}

func (e ErrAdmirePostNotFound) Error() string {
	return fmt.Sprintf("admire post not found | AdmireID: %s, ActorID: %s, PostID: %s", e.AdmireID, e.ActorID, e.PostID)
}

func (e ErrAdmireTokenNotFound) Error() string {
	return fmt.Sprintf("admire token not found | AdmireID: %s, ActorID: %s, TokenID: %s", e.AdmireID, e.ActorID, e.TokenID)
}

type ErrAdmireAlreadyExists struct {
	AdmireID    DBID
	ActorID     DBID
	FeedEventID DBID
	PostID      DBID
}

func (e ErrAdmireAlreadyExists) Error() string {
	return fmt.Sprintf("admire already exists | AdmireID: %s, ActorID: %s, FeedEventID: %s, PostID: %s", e.AdmireID, e.ActorID, e.FeedEventID, e.PostID)
}
