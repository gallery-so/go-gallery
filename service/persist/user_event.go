package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"
)

type UserEventRecord struct {
	ID           DBID            `json:"id"`
	UserID       DBID            `json:"user_id"`
	Version      NullInt32       `json:"version"`
	Code         EventCode       `json:"event_code"`
	CreationTime CreationTime    `json:"created_at"`
	LastUpdated  LastUpdatedTime `json:"last_updated"`
	Data         UserEvent       `json:"data"`
}

type UserEvent struct {
	Username string     `json:"username"`
	Bio      NullString `json:"bio"`
}

func (u UserEvent) Value() (driver.Value, error) {
	return json.Marshal(u)
}

func (u *UserEvent) Scan(value interface{}) error {
	if value == nil {
		*u = UserEvent{}
		return nil
	}
	return json.Unmarshal(value.([]uint8), u)
}

type UserEventRepository interface {
	Add(context.Context, UserEventRecord) (DBID, error)
	Get(context.Context, DBID) (UserEventRecord, error)
	GetEventsSince(context.Context, UserEventRecord, time.Time) ([]UserEventRecord, error)
}
