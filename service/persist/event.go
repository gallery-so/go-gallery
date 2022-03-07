package persist

// Represents an event. The first 6 bits specify the category and
// rhe remaining 10 bits encode the particular event name.
type EventID int16

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

func CategoryFromEventID(eventTypeID EventID) int {
	return int(eventTypeID) >> 6
}

func NameFromEventID(eventTypeID EventID) int {
	mask := (1 << 6) - 1
	return int(eventTypeID) & mask
}
