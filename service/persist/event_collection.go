package persist

import (
	"context"
	"time"
)

type CollectionEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	CollectionID DBID            `json:"collection_id"`
	Version      NullInt32       `json:"version"`
	Type         int             `json:"event_type"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Event        CollectionEvent `json:"event"`
}

type CollectionEvent struct {
	Username string `json:"username"`
	NFTs     []DBID `json:"nfts"`
}

type CollectionEventRepository interface {
	Add(context.Context, CollectionEventRecord) (DBID, error)
	Get(context.Context, DBID) (CollectionEventRecord, error)
	GetEventsSince(context.Context, CollectionEventRecord, time.Time) ([]CollectionEventRecord, error)
}
