// TODO: Remove when the feedbot uses the feed API instead of creating its own posts.
// Everything below can be removed.
package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"
)

type NftEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	NftID        DBID            `json:"nft_id"`
	Version      NullInt32       `json:"version"`
	Code         EventCode       `json:"event_code"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Data         NftEvent        `json:"data"`
}

type NftEvent struct {
	CollectionID   DBID       `json:"collection_id"`
	CollectorsNote NullString `json:"collectors_note"`
}

func (n NftEvent) Value() (driver.Value, error) {
	return json.Marshal(n)
}

func (n *NftEvent) Scan(value interface{}) error {
	if value == nil {
		*n = NftEvent{}
		return nil
	}
	return json.Unmarshal(value.([]uint8), n)
}

type NftEventRepository interface {
	Add(context.Context, NftEventRecord) (*NftEventRecord, error)
	Get(context.Context, DBID) (NftEventRecord, error)
	GetEventsSince(context.Context, NftEventRecord, time.Time) ([]NftEventRecord, error)
	GetEventBefore(context.Context, NftEventRecord) (*NftEventRecord, error)
	MarkSent(context.Context, DBID) error
}
