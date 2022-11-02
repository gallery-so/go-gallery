package feed

import (
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func mergeFollowEvents(eventsAsc []db.Event) *combinedFollowEvent {
	var combined combinedFollowEvent
	return combined.merge(eventsAsc)
}

func mergeCollectionEvents(eventsAsc []db.Event) *combinedCollectionEvent {
	var combined combinedCollectionEvent
	return combined.merge(eventsAsc)
}

type combinedFollowEvent struct {
	event        db.Event
	eventIDs     []persist.DBID
	followedIDs  []persist.DBID
	followedBack []bool
}

func (c *combinedFollowEvent) merge(eventsAsc []db.Event) *combinedFollowEvent {
	for _, other := range eventsAsc {
		if !other.Data.UserRefollowed {
			c.event = db.Event{
				ID:        other.ID,
				ActorID:   other.ActorID,
				Action:    other.Action,
				CreatedAt: other.CreatedAt,
			}
			c.eventIDs = append(c.eventIDs, other.ID)
			c.followedIDs = append(c.followedIDs, other.SubjectID)
			c.followedBack = append(c.followedBack, other.Data.UserFollowedBack)
		}
	}
	return c
}

type combinedCollectionEvent struct {
	event           db.Event
	eventIDs        []persist.DBID
	isNewCollection bool
}

func (c *combinedCollectionEvent) merge(eventsAsc []db.Event) *combinedCollectionEvent {
	for _, other := range eventsAsc {
		action := c.event.Action

		// If the collection is new, then categorize the event as a new collection event. Otherwise,
		// if there are two or more unique actions, the resulting event is categorized as
		// a generic update.
		if c.event.Action == "" {
			action = other.Action
		} else if action != persist.ActionCollectionCreated && c.event.Action != other.Action {
			action = persist.ActionCollectionUpdated
		}

		// Not every event has tokens attached to it, so we check that the event is relevant first.
		collectionTokenIDs := c.event.Data.CollectionTokenIDs
		if other.Action == persist.ActionCollectionCreated || other.Action == persist.ActionTokensAddedToCollection {
			collectionTokenIDs = other.Data.CollectionTokenIDs
		}

		// Not every event has a collector's note attached to it, so we check that the event is relevant first.
		collectorsNote := c.event.Data.CollectionCollectorsNote
		if other.Action == persist.ActionCollectionCreated || other.Action == persist.ActionCollectorsNoteAddedToCollection {
			collectorsNote = other.Data.CollectionCollectorsNote
		}

		c.event = db.Event{
			ID:           other.ID,
			ActorID:      other.ActorID,
			SubjectID:    other.SubjectID,
			CollectionID: other.CollectionID,
			Action:       action,
			CreatedAt:    other.CreatedAt,
			Data: persist.EventData{
				CollectionTokenIDs:       collectionTokenIDs,
				CollectionCollectorsNote: collectorsNote,
			},
		}
		c.eventIDs = append(c.eventIDs, other.ID)
		c.isNewCollection = c.isNewCollection || other.Action == persist.ActionCollectionCreated
	}
	return c
}
