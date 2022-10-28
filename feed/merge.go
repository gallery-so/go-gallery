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
	return combined.merge(eventsAsc...)
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

func (c *combinedCollectionEvent) merge(events ...db.Event) *combinedCollectionEvent {
	for _, other := range events {
		action := c.event.Action
		if c.event.Action == "" {
			action = other.Action
		} else if c.event.Action != other.Action {
			action = persist.ActionCollectionUpdated
		}

		collectionTokenIDs := c.event.Data.CollectionTokenIDs
		if other.Action == persist.ActionCollectionCreated || other.Action == persist.ActionTokensAddedToCollection {
			collectionTokenIDs = other.Data.CollectionTokenIDs
		}

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
