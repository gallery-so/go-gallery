package persist

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jackc/pgtype"
)

/*
create type feed_entity AS (

	id varchar(255),
	token_ids varchar(255)[],
	caption varchar,
	event_time timestampz,
	source varchar,
	version int,
	owner_id varchar(255),
	group_id varchar(255),
	action varchar(255),
	data jsonb,
	event_ids varchar(255)[],
	deleted boolean,
	last_updated timestamptz,
	created_at timestamptz

);
*/
type FeedEntity struct {
	ID          DBID           `json:"id"`
	TokenIDs    DBIDList       `json:"token_ids"`
	Caption     sql.NullString `json:"caption"`
	EventTime   sql.NullTime   `json:"event_time"`
	Source      sql.NullString `json:"source"`
	Version     sql.NullInt32  `json:"version"`
	OwnerID     DBID           `json:"owner_id"`
	GroupID     DBID           `json:"group_id"`
	Action      sql.NullString `json:"action"`
	Data        pgtype.JSONB   `json:"data"`
	EventIDs    DBIDList       `json:"event_ids"`
	Deleted     sql.NullBool   `json:"deleted"`
	LastUpdated sql.NullTime   `json:"last_updated"`
	CreatedAt   sql.NullTime   `json:"created_at"`
}

func (f *FeedEntity) Scan(src interface{}) error {
	// Convert the interface{} type to a byte array
	source, ok := src.([]byte)
	if !ok {
		return errors.New("Type assertion .([]byte) failed.")
	}

	// Unmarshal the byte array into the FeedEntity struct
	err := json.Unmarshal(source, f)
	if err != nil {
		return err
	}

	return nil
}
