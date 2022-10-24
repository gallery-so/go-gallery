package feed

import (
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

// mergedFollowEvent
type mergedFollowEvent struct {
	evt          db.Event
	eventIDs     []persist.DBID
	followedIDs  []persist.DBID
	followedBack []bool
}

func (m mergedFollowEvent) merge(events ...db.Event) mergedFollowEvent {
	for _, event := range events {
		if !event.Data.UserRefollowed {
			m.add(event)
		}
	}
	return m
}

func (m mergedFollowEvent) asFeedEvent() db.FeedEvent {
	return db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   m.evt.ActorID,
		Action:    m.evt.Action,
		EventTime: m.evt.CreatedAt,
		EventIds:  m.eventIDs,
		Data: persist.FeedEventData{
			UserFollowedIDs:  m.followedIDs,
			UserFollowedBack: m.followedBack,
		},
	}
}

func (m *mergedFollowEvent) add(other db.Event) {
	gt := compare(m.evt, other)
	m = &mergedFollowEvent{
		evt: db.Event{
			ID:        gt.ID,
			ActorID:   gt.ActorID,
			Action:    gt.Action,
			CreatedAt: gt.CreatedAt,
		},
		eventIDs:     append(m.eventIDs, other.ID),
		followedIDs:  append(m.followedIDs, other.SubjectID),
		followedBack: append(m.followedBack, other.Data.UserFollowedBack),
	}
}

func (m mergedFollowEvent) hasNewFollows() bool {
	return len(m.followedIDs) > 1
}

// mergedCollectionUpdatedEvent
type mergedCollectionUpdatedEvent struct {
	evt             db.Event
	addedTokens     []persist.DBID
	eventIDs        []persist.DBID
	isNewCollection bool
}

func (m mergedCollectionUpdatedEvent) merge(events ...db.Event) mergedCollectionUpdatedEvent {
	for _, event := range events {
		m.add(event)
	}
	return m
}

func (m *mergedCollectionUpdatedEvent) add(other db.Event) {
	gt := compare(m.evt, other)
	m = &mergedCollectionUpdatedEvent{
		evt: db.Event{
			ID:           gt.ID,
			ActorID:      gt.ActorID,
			SubjectID:    gt.SubjectID,
			CollectionID: gt.CollectionID,
			Action:       gt.Action,
			CreatedAt:    gt.CreatedAt,
			Data: persist.EventData{
				CollectionTokenIDs:       gt.Data.CollectionTokenIDs,
				CollectionCollectorsNote: gt.Data.CollectionCollectorsNote,
			},
		},
		eventIDs: append(m.eventIDs, other.ID),
	}
	if other.Action == persist.ActionCollectionCreated {
		m.isNewCollection = true
	}
}

func compare(a, b db.Event) db.Event {
	if a.CreatedAt.After(b.CreatedAt) {
		return a
	}
	if a.CreatedAt.Before(b.CreatedAt) {
		return b
	}
	if a.ID > b.ID {
		return a
	}
	return b
}
