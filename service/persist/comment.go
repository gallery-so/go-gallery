package persist

import (
	"context"
	"fmt"
)

type Comment struct {
	ID          DBID            `json:"id"`
	CreatedAt   CreationTime    `json:"created_at"`
	LastUpdated LastUpdatedTime `json:"last_updated"`
	FeedEventID DBID            `json:"feed_event_id"`
	ActorID     DBID            `json:"actor_id"`
	ReplyTo     DBID            `json:"reply_to"`
	Comment     string          `json:"comment"`
	Deleted     bool            `json:"deleted"`
}

type CommentRepository interface {
	// replyToID is optional
	CreateComment(ctx context.Context, feedEventID DBID, actorID DBID, replyToID *DBID, comment string) (DBID, error)
	RemoveComment(ctx context.Context, commentID DBID) error
}

type ErrCommentNotFound struct {
	ID DBID
}

func (e ErrCommentNotFound) Error() string {
	return fmt.Sprintf("comment not found by id: %s", e.ID)
}
