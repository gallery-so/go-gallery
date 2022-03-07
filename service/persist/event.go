package persist

// Represents an event. The first 8 bits specify the category and
// the remaining 8 bits encode the particular event name.
type EventCode int16

const (
	UserEventCode = (1 << 8) + iota
	UserCreatedEvent
	UserDeletedEvent
)
const (
	TokenEventCode = (2 << 8) + iota
	TokenCollectorsNoteAddedEvent
)
const (
	CollectionEventCode = (3 << 8) + iota
	CollectionCreatedEvent
	CollectionCollectorsNoteAdded
	CollectionTokensAdded
)

func CategoryFromEventCode(eventCode EventCode) int {
	return int(eventCode) >> 8
}

func NameFromEventCode(eventCode EventCode) int {
	mask := (1 << 8) - 1
	return int(eventCode) & mask
}
