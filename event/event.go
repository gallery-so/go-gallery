package event

import (
	"database/sql"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
)

const (
	UserEventType = iota + 1
	TokenEventType
	CollectionEventType
)

const (
	UserCreatedEvent = (UserEventType << 8) + iota + 1
	UserDeletedEvent
)

const (
	TokenCollectorsNoteAddedEvent = (TokenEventType << 8) + iota + 1
)

const (
	CollectionCreatedEvent = (CollectionEventType << 8) + iota + 1
	CollectionCollectorsNoteAdded
	CollectionTokensAdded
)

// First 8 bits is the event category, last 8 bits is the sub-type.
type EventTypeID int16

type EventMessage struct {
	ID          persist.DBID
	EventTypeID EventTypeID
}

type EventRepositories struct {
	UserEventRepository       persist.UserEventRepository
	TokenEventRepository      persist.TokenEventRepository
	CollectionEventRepository persist.CollectionEventRepository
}

func NewEventRepos(db *sql.DB) *EventRepositories {
	return &EventRepositories{
		UserEventRepository:       postgres.NewUserEventRepository(db),
		TokenEventRepository:      postgres.NewTokenEventRepository(db),
		CollectionEventRepository: postgres.NewCollectionEventRepository(db),
	}
}

func GetCategoryFromEventTypeID(eventTypeID EventTypeID) int {
	return int(eventTypeID) >> 8
}

func GetSubtypeFromEventTypeID(eventTypeID EventTypeID) int {
	return int(eventTypeID)&(1<<8) - 1
}
