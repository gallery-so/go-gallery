package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
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

type Mentions map[DBID]Mention

func (m Mentions) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *Mentions) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]uint8), m)
}

type MentionType string

const (
	MentionTypeUser      MentionType = "user"
	MentionTypeCommunity MentionType = "community"
)

type Mention struct {
	MentionType MentionType    `json:"mention_type"`
	Index       *CompleteIndex `json:"index,omitempty"`
}

func (m Mention) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *Mention) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]uint8), m)
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
