package persist

import (
	"fmt"
	"time"
)

type Comment struct {
	ID          DBID      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	LastUpdated time.Time `json:"last_updated"`
	FeedEventID DBID      `json:"feed_event_id"`
	ActorID     DBID      `json:"actor_id"`
	ReplyTo     DBID      `json:"reply_to"`
	Comment     string    `json:"comment"`
	Deleted     bool      `json:"deleted"`
}

type MentionType string

const (
	MentionTypeUser      MentionType = "user"
	MentionTypeCommunity MentionType = "community"
)

var errCommentNotFound ErrCommentNotFound

type ErrCommentNotFound struct{}

func (e ErrCommentNotFound) Unwrap() error { return notFoundError }
func (e ErrCommentNotFound) Error() string { return "comment not found" }

type ErrCommentNotFoundByID struct{ ID DBID }

func (e ErrCommentNotFoundByID) Unwrap() error { return errCommentNotFound }
func (e ErrCommentNotFoundByID) Error() string {
	return fmt.Sprintf("comment not found by id=%s", e.ID)
}
