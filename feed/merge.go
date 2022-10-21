package feed

import (
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type mergedFollowEvent struct {
	evt          db.Event
	eventIDs     []persist.DBID
	followedIDs  []persist.DBID
	followedBack []bool
}

func (m *mergedFollowEvent) Merge(events ...db.Event) db.FeedEvent {
	for _, event := range events {
		if !event.Data.UserRefollowed {
			m.merge(event)
		}
	}
	return db.FeedEvent{
		ID:        persist.GenerateID(),
		OwnerID:   m.evt.ActorID,
		Action:    m.evt.Action,
		EventTime: m.evt.CreatedAt,
		EventIds:  m.eventIDs,
		Caption:   m.evt.Caption,
		Data: persist.FeedEventData{
			UserFollowedIDs:  m.followedIDs,
			UserFollowedBack: m.followedBack,
		},
	}
}

func (m *mergedFollowEvent) hasNewFollows() bool {
	return len(m.followedIDs) > 1
}

func (m *mergedFollowEvent) merge(other db.Event) {
	greater := compare(m.evt, other)
	m = &mergedFollowEvent{
		evt: db.Event{
			ID:        greater.ID,
			ActorID:   greater.ActorID,
			Action:    greater.Action,
			CreatedAt: greater.CreatedAt,
			Caption:   greater.Caption,
		},
		eventIDs:     append(m.eventIDs, other.ID),
		followedIDs:  append(m.followedIDs, other.SubjectID),
		followedBack: append(m.followedBack, other.Data.UserFollowedBack),
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
