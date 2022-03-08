package persist

import (
	"context"
	"time"
)

type NftEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	NftID        TokenID         `json:"nft_id"`
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

type NftEventRepository interface {
	Add(context.Context, NftEventRecord) (DBID, error)
	Get(context.Context, DBID) (NftEventRecord, error)
	GetEventsSince(context.Context, NftEventRecord, time.Time) ([]NftEventRecord, error)
}
