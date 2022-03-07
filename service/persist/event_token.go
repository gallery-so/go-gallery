package persist

import (
	"context"
	"time"
)

type TokenEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	TokenID      TokenID         `json:"token_id"`
	Version      NullInt32       `json:"version"`
	Type         int             `json:"event_type"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Event        TokenEvent      `json:"event"`
}

type TokenEvent struct {
	Username     string `json:"username"`
	CollectionID DBID   `json:"collection_id"`
}

type TokenEventRepository interface {
	Add(context.Context, TokenEventRecord) (DBID, error)
	Get(context.Context, DBID) (TokenEventRecord, error)
	GetEventsSince(context.Context, TokenEventRecord, time.Time) ([]TokenEventRecord, error)
}
