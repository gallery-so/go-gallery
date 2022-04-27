package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"
)

type CollectionEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	CollectionID DBID            `json:"collection_id"`
	Version      NullInt32       `json:"version"`
	Code         EventCode       `json:"event_code"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Data         CollectionEvent `json:"data"`
}

type CollectionEvent struct {
	NFTs           []DBID     `json:"nfts"`
	CollectorsNote NullString `json:"collectors_note"`
}

func (c CollectionEvent) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *CollectionEvent) Scan(value interface{}) error {
	if value == nil {
		*c = CollectionEvent{}
		return nil
	}
	return json.Unmarshal(value.([]uint8), c)
}

type CollectionEventRepository interface {
	Add(context.Context, CollectionEventRecord) (*CollectionEventRecord, error)
	Get(context.Context, DBID) (CollectionEventRecord, error)
	GetEventsSince(context.Context, CollectionEventRecord, time.Time) ([]CollectionEventRecord, error)
	GetEventBefore(context.Context, CollectionEventRecord) (*CollectionEventRecord, error)
}
