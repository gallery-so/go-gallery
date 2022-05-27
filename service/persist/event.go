package persist

type EventCode int16

const (
	UserEventCode = iota + 1
	NftEventCode
	CollectionEventCode
)
const (
	UserCreatedEvent = (UserEventCode << 8) + iota + 1
	UserFollowedEvent
)
const (
	NftCollectorsNoteAddedEvent = (NftEventCode << 8) + iota + 1
)
const (
	CollectionCreatedEvent = (CollectionEventCode << 8) + iota + 1
	CollectionCollectorsNoteAdded
	CollectionTokensAdded
)

func CategoryFromEventCode(eventCode EventCode) int {
	return int(eventCode) >> 8
}
