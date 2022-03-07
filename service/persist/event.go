package persist

// Represents an event. The first 6 bits specify the category and
// rhe remaining 10 bits encode the particular event name.
type EventType int16

const (
	UserEventType = (1 << 6) + iota
	UserCreatedEvent
	UserDeletedEvent
)
const (
	TokenEventType = (2 << 6) + iota
	TokenCollectorsNoteAddedEvent
)
const (
	CollectionEventType = (3 << 6) + iota
	CollectionCreatedEvent
	CollectionCollectorsNoteAdded
	CollectionTokensAdded
)

func CategoryFromEventType(eventTypeID EventType) int {
	return int(eventTypeID) >> 6
}

func NameFromEventType(eventTypeID EventType) int {
	mask := (1 << 6) - 1
	return int(eventTypeID) & mask
}
