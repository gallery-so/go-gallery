package persist

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/util"
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
	ID          DBID             `json:"id"`
	TokenIDs    DBIDList         `json:"token_ids"`
	Caption     sql.NullString   `json:"caption"`
	EventTime   sql.NullTime     `json:"event_time"`
	Source      FeedEntitySource `json:"source"`
	Version     sql.NullInt32    `json:"version"`
	OwnerID     DBID             `json:"owner_id"`
	GroupID     DBID             `json:"group_id"`
	Action      sql.NullString   `json:"action"`
	Data        pgtype.JSONB     `json:"data"`
	EventIDs    DBIDList         `json:"event_ids"`
	Deleted     sql.NullBool     `json:"deleted"`
	LastUpdated sql.NullTime     `json:"last_updated"`
	CreatedAt   sql.NullTime     `json:"created_at"`
}

func (f *FeedEntity) Scan(src interface{}) error {
	if src == nil {
		return nil
	}

	source, ok := src.(string)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed for type %T", src)
	}

	source = strings.TrimLeft(source, "(")
	source = strings.TrimRight(source, ")")
	// Create a new CSV reader reading from the source string
	reader := csv.NewReader(strings.NewReader(source))

	// Assuming that the CSV file has only a single row, we can call reader.Read() to get the first row
	fields, err := reader.Read()
	if err != nil {
		return err
	}

	/*
			field order:
			id varchar(255),
		    token_ids varchar(255)[],
		    caption varchar,
		    event_time timestamptz,
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
	*/

	dataPresentStatus := pgtype.Present
	if fields[9] == "" {
		dataPresentStatus = pgtype.Null
	}
	// Parse the fields into the struct values
	tokenIDsStrings := strings.Split(fields[1], ",")
	tokenIDsDBIDs, _ := util.Map(tokenIDsStrings, func(i string) (DBID, error) {
		return DBID(i), nil
	})
	eventIDsStrings := strings.Split(fields[10], ",")
	eventIDsDBIDs, _ := util.Map(eventIDsStrings, func(i string) (DBID, error) {
		return DBID(i), nil
	})
	new := FeedEntity{
		ID:          DBID(fields[0]),
		TokenIDs:    DBIDList(tokenIDsDBIDs),
		Caption:     sql.NullString{String: fields[2], Valid: fields[2] != ""},
		EventTime:   sql.NullTime{Time: parseTime(fields[3]), Valid: fields[3] != ""},
		Source:      FeedEntitySource(fields[4]),
		Version:     sql.NullInt32{Int32: parseInt32(fields[5]), Valid: fields[5] != ""},
		OwnerID:     DBID(fields[6]),
		GroupID:     DBID(fields[7]),
		Action:      sql.NullString{String: fields[8], Valid: fields[8] != ""},
		Data:        pgtype.JSONB{Bytes: []byte(fields[9]), Status: dataPresentStatus},
		EventIDs:    DBIDList(eventIDsDBIDs),
		Deleted:     sql.NullBool{Bool: fields[11] == "t", Valid: fields[11] != ""},
		LastUpdated: sql.NullTime{Time: parseTime(fields[12]), Valid: fields[12] != ""},
		CreatedAt:   sql.NullTime{Time: parseTime(fields[13]), Valid: fields[13] != ""},
	}

	*f = new

	return nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.999999-07", s)
	if err != nil {
		panic(err)
	}
	return t
}

func parseInt32(s string) int32 {
	i, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return int32(i)
}

type FeedEntitySource string

const (
	FeedEntitySourceFeedEvent FeedEntitySource = "feed_event"
	FeedEntitySourcePost      FeedEntitySource = "post"
)

func (f *FeedEntitySource) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	source := FeedEntitySource(src.(string))
	switch source {
	case FeedEntitySourceFeedEvent:
		*f = FeedEntitySourceFeedEvent
	case FeedEntitySourcePost:
		*f = FeedEntitySourcePost
	default:
		return fmt.Errorf("invalid FeedEntitySource: %s", source)
	}
	return nil
}
