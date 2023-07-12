package persist

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
// type FeedEntity struct {
// 	ID          DBID             `json:"id"`
// 	TokenIDs    DBIDList         `json:"token_ids"`
// 	Caption     sql.NullString   `json:"caption"`
// 	EventTime   time.Time        `json:"event_time"`
// 	Source      FeedEntitySource `json:"source"`
// 	Version     sql.NullInt32    `json:"version"`
// 	OwnerID     DBID             `json:"owner_id"`
// 	GroupID     DBID             `json:"group_id"`
// 	Action      Action           `json:"action"`
// 	Data        FeedEventData    `json:"data"`
// 	EventIDs    DBIDList         `json:"event_ids"`
// 	Deleted     sql.NullBool     `json:"deleted"`
// 	LastUpdated time.Time        `json:"last_updated"`
// 	CreatedAt   time.Time        `json:"created_at"`
// }

// type FeedEntitySource string

// const (
// 	FeedEntitySourceFeedEvent FeedEntitySource = "feed_event"
// 	FeedEntitySourcePost      FeedEntitySource = "post"
// )

// func (f *FeedEntitySource) Scan(src interface{}) error {
// 	if src == nil {
// 		return nil
// 	}
// 	source := FeedEntitySource(src.(string))
// 	switch source {
// 	case FeedEntitySourceFeedEvent:
// 		*f = FeedEntitySourceFeedEvent
// 	case FeedEntitySourcePost:
// 		*f = FeedEntitySourcePost
// 	default:
// 		return fmt.Errorf("invalid FeedEntitySource: %s", source)
// 	}
// 	return nil
// }

const (
	FeedEventTypeTag = iota
	PostTypeTag
)
