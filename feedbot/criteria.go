package feedbot

import "github.com/mikeydub/go-gallery/service/persist"

type Criteria struct {
	EventIsValid                         Criterion
	EventNoMoreRecentEvents              Criterion
	IsUserCreatedEvent                   Criterion
	IsUserFollowedEvent                  Criterion
	IsNftCollectorsNoteAddedEvent        Criterion
	IsCollectionCreatedEvent             Criterion
	IsCollectionCollectorsNoteAddedEvent Criterion
	IsCollectionTokensAddedEvent         Criterion
	UserHasUsername                      Criterion
	UserNoEventsBefore                   Criterion
	FollowedUserHasUsername              Criterion
	NftHasCollectorsNote                 Criterion
	NftBelongsToCollection               Criterion
	CollectionHasNfts                    Criterion
	CollectionHasCollectorsNote          Criterion
	CollectionHasNewTokensAdded          Criterion
}

type Criterion struct {
	name string
	eval func(q Query) bool
}

func newCriteria() Criteria {
	return Criteria{
		EventIsValid: Criterion{
			name: "EventIsValid",
			eval: eventIsValid,
		},
		EventNoMoreRecentEvents: Criterion{
			name: "EventNoMoreRecentEvents",
			eval: eventNoMoreRecentEvents,
		},
		IsUserCreatedEvent: Criterion{
			name: "IsUserCreatedEvent",
			eval: isUserCreatedEvent,
		},
		IsUserFollowedEvent: Criterion{
			name: "IsUserFollowedEvent",
			eval: isUserFollowedEvent,
		},
		IsNftCollectorsNoteAddedEvent: Criterion{
			name: "IsNftCollectorsNoteAddedEvent",
			eval: isNftCollectorsNoteAddedEvent,
		},
		IsCollectionCreatedEvent: Criterion{
			name: "IsCollectionCreatedEvent",
			eval: isCollectionCreatedEvent,
		},
		IsCollectionCollectorsNoteAddedEvent: Criterion{
			name: "IsCollectionCollectorsNoteAddedEvent",
			eval: isCollectionCollectorsNoteAddedEvent,
		},
		IsCollectionTokensAddedEvent: Criterion{
			name: "IsCollectionTokensAddedEvent",
			eval: isCollectionTokensAddedEvent,
		},
		UserHasUsername: Criterion{
			name: "UserHasUsername",
			eval: userHasUsername,
		},
		UserNoEventsBefore: Criterion{
			name: "UserNoEventsBefore",
			eval: userNoEventsBefore,
		},
		FollowedUserHasUsername: Criterion{
			name: "FollowedUserHasUsername",
			eval: followedUserHasUsername,
		},
		NftHasCollectorsNote: Criterion{
			name: "NftHasCollectorsNote",
			eval: nftHasCollectorsNote,
		},
		NftBelongsToCollection: Criterion{
			name: "NftBelongsToCollection",
			eval: nftBelongsToCollection,
		},
		CollectionHasNfts: Criterion{
			name: "CollectionHasNfts",
			eval: collectionHasNfts,
		},
		CollectionHasCollectorsNote: Criterion{
			name: "CollectionHasCollectorsNote",
			eval: collectionHasCollectorsNote,
		},
		CollectionHasNewTokensAdded: Criterion{
			name: "CollectionHasNewTokensAdded",
			eval: collectionHasNewTokensAdded,
		},
	}
}

func eventIsValid(q Query) bool {
	if q.EventID == "" {
		return false
	}

	if q.EventCode == 0 {
		return false
	}

	switch persist.CategoryFromEventCode(q.EventCode) {
	case persist.UserEventCode:
		return q.UserID != ""
	case persist.NftEventCode:
		return q.NftID != ""
	case persist.CollectionEventCode:
		return q.CollectionID != ""
	default:
		return false
	}
}

func eventNoMoreRecentEvents(q Query) bool {
	return q.EventsSince == 0
}

func isUserCreatedEvent(q Query) bool {
	return q.EventCode == persist.UserCreatedEvent
}

func isUserFollowedEvent(q Query) bool {
	return q.EventCode == persist.UserFollowedEvent
}

func isNftCollectorsNoteAddedEvent(q Query) bool {
	return q.EventCode == persist.NftCollectorsNoteAddedEvent
}

func isCollectionCreatedEvent(q Query) bool {
	return q.EventCode == persist.CollectionCreatedEvent
}

func isCollectionCollectorsNoteAddedEvent(q Query) bool {
	return q.EventCode == persist.CollectionCollectorsNoteAdded
}

func isCollectionTokensAddedEvent(q Query) bool {
	return q.EventCode == persist.CollectionTokensAdded
}

func userHasUsername(q Query) bool {
	return q.Username != ""
}

func userNoEventsBefore(q Query) bool {
	return q.LastUserEvent == nil
}

func followedUserHasUsername(q Query) bool {
	return q.FollowedUsername != ""
}

func nftHasCollectorsNote(q Query) bool {
	if q.NftCollectorsNote == "" {
		return false
	}

	if q.LastNftEvent != nil {
		return q.NftCollectorsNote != q.LastNftEvent.Data.CollectorsNote.String()
	}

	return true
}

func nftBelongsToCollection(q Query) bool {
	return q.NftID != "" && q.CollectionID != ""
}

func collectionHasNfts(q Query) bool {
	return len(q.CollectionNfts) > 0
}

func collectionHasCollectorsNote(q Query) bool {
	if q.CollectionCollectorsNote == "" {
		return false
	}

	if q.LastNftEvent != nil {
		return q.CollectionCollectorsNote != q.LastCollectionEvent.Data.CollectorsNote.String()
	}

	return true
}

func collectionHasNewTokensAdded(q Query) bool {
	if q.LastCollectionEvent == nil {
		return true
	}

	var newTokens bool
	for _, nft := range q.CollectionNfts {
		contains := false
		for _, otherId := range q.LastCollectionEvent.Data.NFTs {
			if nft.Id == otherId.String() {
				contains = true
				break
			}
		}
		if !contains {
			newTokens = true
			break
		}
	}

	return newTokens
}
