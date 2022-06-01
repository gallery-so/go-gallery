package feedbot

import "github.com/mikeydub/go-gallery/service/persist"

var baseCriteria = []func(Query) bool{eventIsValid, eventNoMoreRecentEvents, userHasUsername}

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
			if nft.Nft.Dbid == otherId {
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
