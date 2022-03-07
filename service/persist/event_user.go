package persist

import (
	"context"
	"time"
)

type UserEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	Version      NullInt32       `json:"version"`
	Type         int             `json:"event_type"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Event        UserEvent       `json:"event"`
}

type UserEvent struct {
	Username string `json:"username"`
}

type UserEventRepository interface {
	Add(context.Context, UserEventRecord) (DBID, error)
	Get(context.Context, DBID) (UserEventRecord, error)
	GetEventsSince(context.Context, UserEventRecord, time.Time) ([]UserEventRecord, error)
}
