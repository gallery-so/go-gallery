package persist

import (
	"context"
	"time"
)

type UserEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	Version      NullInt32       `json:"version"`
	Code         int             `json:"event_code"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Data         UserEvent       `json:"data"`
}

type UserEvent struct{}

type UserEventRepository interface {
	Add(context.Context, UserEventRecord) (DBID, error)
	Get(context.Context, DBID) (UserEventRecord, error)
	GetEventsSince(context.Context, UserEventRecord, time.Time) ([]UserEventRecord, error)
}
