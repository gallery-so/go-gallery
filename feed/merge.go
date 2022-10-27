package feed

import (
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

func mergeFollowEvents(events []db.Event) combinedFollowEvent {
	return combinedFollowEvent{}.merge(events...)
}

func mergeCollectionEvents(events []db.Event) combinedCollectionEvent {
	return combinedCollectionEvent{}.merge(events...)
}

type combinedFollowEvent struct {
	evt          db.Event
	eventIDs     []persist.DBID
	followedIDs  []persist.DBID
	followedBack []bool
}

func (c combinedFollowEvent) merge(events ...db.Event) combinedFollowEvent {
	for _, event := range events {
		if !event.Data.UserRefollowed {
			c.add(event)
		}
	}
	return c
}

func (c combinedFollowEvent) asFeedEvent() db.FeedEvent {
	return db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   c.evt.ActorID,
		Action:    c.evt.Action,
		EventTime: c.evt.CreatedAt,
		EventIds:  c.eventIDs,
		Data: persist.FeedEventData{
			UserFollowedIDs:  c.followedIDs,
			UserFollowedBack: c.followedBack,
		},
	}
}

func (c *combinedFollowEvent) add(other db.Event) {
	e := mostRecent(c.evt, other)
	c = &combinedFollowEvent{
		evt: db.Event{
			ID:        e.ID,
			ActorID:   e.ActorID,
			Action:    e.Action,
			CreatedAt: e.CreatedAt,
		},
		eventIDs:     append(c.eventIDs, other.ID),
		followedIDs:  append(c.followedIDs, other.SubjectID),
		followedBack: append(c.followedBack, other.Data.UserFollowedBack),
	}
}

type combinedCollectionEvent struct {
	evt             db.Event
	eventIDs        []persist.DBID
	isNewCollection bool
}

func (c combinedCollectionEvent) merge(events ...db.Event) combinedCollectionEvent {
	for _, event := range events {
		c.add(event)
	}
	return c
}

func (c *combinedCollectionEvent) add(other db.Event) {
	e := mostRecent(c.evt, other)

	// The combined event is marked as an update event if there
	// are two or more unique actions that make it up.
	action := c.evt.Action
	if c.evt.Action == "" {
		action = other.Action
	} else if c.evt.Action != other.Action {
		action = persist.ActionCollectionUpdated
	}

	// Not every collection action involves adding tokens, so we need
	// to check if the event is relevant.
	collectionTokenIDs := c.evt.Data.CollectionTokenIDs
	if (other.Action == persist.ActionCollectionCreated ||
		other.Action == persist.ActionTokensAddedToCollection) && isGreaterThan(other, c.evt) {
		collectionTokenIDs = other.Data.CollectionTokenIDs
	}

	// Not every collection action involves adding a collector's note, so we need
	// to check if the event is relevant.
	collectorsNote := c.evt.Data.CollectionCollectorsNote
	if (other.Action == persist.ActionCollectionCreated ||
		other.Action == persist.ActionCollectorsNoteAddedToCollection) && isGreaterThan(other, c.evt) {
		collectorsNote = other.Data.CollectionCollectorsNote
	}

	c = &combinedCollectionEvent{
		evt: db.Event{
			ID:           e.ID,
			ActorID:      e.ActorID,
			SubjectID:    e.SubjectID,
			CollectionID: e.CollectionID,
			Action:       action,
			CreatedAt:    e.CreatedAt,
			Data: persist.EventData{
				CollectionTokenIDs:       collectionTokenIDs,
				CollectionCollectorsNote: collectorsNote,
			},
		},
		eventIDs:        append(c.eventIDs, other.ID),
		isNewCollection: c.isNewCollection || other.Action == persist.ActionCollectionCreated,
	}
}

func (c combinedCollectionEvent) asFeedEvent(addedTokens []persist.DBID) db.FeedEvent {
	return db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   c.evt.ActorID,
		Action:    c.evt.Action,
		EventTime: c.evt.CreatedAt,
		EventIds:  c.eventIDs,
		Data: persist.FeedEventData{
			CollectionID:                c.evt.SubjectID,
			CollectionTokenIDs:          c.evt.Data.CollectionTokenIDs,
			CollectionNewCollectorsNote: c.evt.Data.CollectionCollectorsNote,
			CollectionNewTokenIDs:       addedTokens,
		},
	}
}

// mostRecent returns the more recent event
func mostRecent(a, b db.Event) db.Event {
	if isGreaterThan(a, b) {
		return a
	}
	return b
}

// isGreaterThan returns true if event a is greater than event b
func isGreaterThan(a, b db.Event) bool {
	if a.CreatedAt.After(b.CreatedAt) {
		return true
	}
	if a.CreatedAt.Before(b.CreatedAt) {
		return false
	}
	if a.ID > b.ID {
		return true
	}
	return false
}
