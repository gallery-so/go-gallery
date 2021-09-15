package persist

import (
	"context"
	"errors"
	"time"

	"github.com/mikeydub/go-gallery/runtime"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	eventColName = "event"
	feedColName  = "feed"
)

const (
	// EventTypeUpdateInfoCollection is the event type for when a collection's collectors note is updated
	EventTypeUpdateInfoCollection = iota
	// EventTypeUpdateInfoNFT is the event type for when a NFT's collectors note  is updated
	EventTypeUpdateInfoNFT
	// EventTypeAddNFTsToCollection is the event type for when a NFT is added to a collection
	EventTypeAddNFTsToCollection
	// EventTypeCreateCollection is the event type for when a collection is created
	EventTypeCreateCollection
)

const (
	// EventItemTypeCollection is the event item type for when an item is a collection
	EventItemTypeCollection = iota
	// EventItemTypeNFT is the event item type for when an item is a NFT
	EventItemTypeNFT
	// EventItemTypeUser is the event item type for when an item is a user
	EventItemTypeUser
	// EventItemTypeGallery is the event item type for when an item is a gallery
	EventItemTypeGallery
)

// feedInactive represents how long until a feed is considered no longer
var feedInactive = time.Hour * 24 * 30
var eventRecencyThreshold = time.Hour

// Feed is a feed of events for a user
type Feed struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`
	LastUpdated  primitive.DateTime `bson:"last_updated" json:"last_updated"`

	UserID DBID   `bson:"user_id" json:"user_id"`
	Events []DBID `bson:"events" json:"events"`
}

// Event represents an event in a user's feed
type Event struct {
	Version      int64              `bson:"version"              json:"version"` // schema version for this model
	ID           DBID               `bson:"_id"                  json:"id"`
	CreationTime primitive.DateTime `bson:"created_at"        json:"created_at"`
	Deleted      bool               `bson:"deleted" json:"-"`

	Type int         `bson:"type" json:"type"`
	Data []EventItem `bson:"data" json:"data"`
}

// EventItem represents the item for an event such as the collection being updated or an
// NFT being added to a collection
type EventItem struct {
	Type  int  `bson:"type" json:"type"`
	Value DBID `bson:"value" json:"value"`
}

// eventCreate creates an event in the database and broadcasts it to feeds
func eventCreate(pCtx context.Context, pEvent *Event, pRuntime *runtime.Runtime) (DBID, error) {
	mp := newStorage(0, eventColName, pRuntime)

	id, err := mp.insert(pCtx, pEvent)
	if err != nil {
		return "", err
	}

	// broadcast event in the background
	// TODO make sure to log errors
	go eventBroadcast(pCtx, id, pRuntime)

	return id, nil
}

// eventBroadcast broadcasts an event to all feeds that are considered "active"
func eventBroadcast(pCtx context.Context, pEventID DBID, pRuntime *runtime.Runtime) error {
	mp := newStorage(0, eventColName, pRuntime)
	query := bson.M{"last_updated": bson.M{"$lt": time.Now().Add(-feedInactive)}}
	return mp.push(pCtx, query, "events", pEventID)
}

func feedCreate(pCtx context.Context, pUserID DBID, pRuntime *runtime.Runtime) (DBID, error) {
	mp := newStorage(0, feedColName, pRuntime)
	feed := &Feed{UserID: pUserID}
	return mp.insert(pCtx, feed)
}

// FeedGetByUserID gets a feed from the DB by user ID
func FeedGetByUserID(pCtx context.Context, pUserID DBID, pRuntime *runtime.Runtime) (*Feed, error) {
	opts := options.Aggregate()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}
	mp := newStorage(0, feedColName, pRuntime)

	query := bson.M{"user_id": pUserID}

	feeds := []*Feed{}
	err := mp.aggregate(pCtx, newFeedPipeline(query), feeds, opts)
	if err != nil {
		return nil, err
	}
	if len(feeds) == 0 {
		return nil, errors.New("no feed found")
	}
	if len(feeds) > 1 {
		return nil, errors.New("more than one feed found")
	}
	return feeds[0], nil
}

func newFeedPipeline(matchFilter bson.M) mongo.Pipeline {

	return mongo.Pipeline{
		{{Key: "$match", Value: matchFilter}},
		{{Key: "$lookup", Value: bson.M{
			"from": "events",
			"let":  bson.M{"array": "$events"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{
						"$and": []bson.M{
							{"$in": []string{"$_id", "$$array"}},
							{"$eq": []interface{}{"$deleted", false}},
						},
					},
				}}},
				{{Key: "$addFields", Value: bson.M{
					"sort": bson.M{
						"$indexOfArray": []string{"$$array", "$_id"},
					}},
				}},
				{{Key: "$sort", Value: bson.M{"sort": 1}}},
				{{Key: "$unset", Value: []string{"sort"}}},
			},
			"as": "events",
		}}},
	}
}
