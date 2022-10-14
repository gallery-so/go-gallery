package persist

import (
	"context"
	"fmt"
)

type Admire struct {
	ID          DBID            `json:"id"`
	CreatedAt   CreationTime    `json:"created_at"`
	LastUpdated LastUpdatedTime `json:"last_updated"`
	FeedEventID DBID            `json:"feed_event_id"`
	ActorID     DBID            `json:"actor_id"`
	Deleted     bool            `json:"deleted"`
}

type AdmireRepository interface {
	CreateAdmire(ctx context.Context, feedEventID DBID, actorID DBID) (DBID, error)
	RemoveAdmire(ctx context.Context, admireID DBID) error
}

type ErrAdmireNotFound struct {
	AdmireID    DBID
	ActorID     DBID
	FeedEventID DBID
}

func (e ErrAdmireNotFound) Error() string {
	return fmt.Sprintf("admire not found by AdmireID: %s, ActorID: %s, FeedEventID: %s", e.AdmireID, e.ActorID, e.FeedEventID)
}
