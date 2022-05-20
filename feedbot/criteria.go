package feedbot

import "github.com/mikeydub/go-gallery/service/persist"

type FeedCriteria struct{}

func (FeedCriteria) EventIsValid(q Query) bool {
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

func (FeedCriteria) EventNoMoreRecentEvents(q Query) bool {
	return q.EventsSince > 0
}

func (FeedCriteria) IsUserCreatedEvent(q Query) bool {
	return q.EventCode == persist.UserCreatedEvent
}

func (FeedCriteria) IsUserFollowedEvent(q Query) bool {
	return q.EventCode == persist.UserFollowedEvent
}

func (FeedCriteria) IsNftCollectorsNoteAddedEvent(q Query) bool {
	return q.EventCode == persist.NftCollectorsNoteAddedEvent
}

func (FeedCriteria) IsCollectionCreatedEvent(q Query) bool {
	return q.EventCode == persist.CollectionCreatedEvent
}

func (FeedCriteria) IsCollectionCollectorsNoteAddedEvent(q Query) bool {
	return q.EventCode == persist.CollectionCollectorsNoteAdded
}

func (FeedCriteria) IsCollectionTokensAddedEvent(q Query) bool {
	return q.EventCode == persist.CollectionTokensAdded
}

func (FeedCriteria) UserHasUsername(q Query) bool {
	return q.Username != ""
}

func (FeedCriteria) UserNoEventsBefore(q Query) bool {
	return q.LastUserEvent == nil
}

func (FeedCriteria) FollowedUserHasUsername(q Query) bool {
	return q.FollowedUsername != ""
}

func (FeedCriteria) NftHasCollectorsNote(q Query) bool {
	if q.NftCollectorsNote == "" {
		return false
	}

	if q.LastNftEvent != nil {
		return q.NftCollectorsNote != q.LastNftEvent.Data.CollectorsNote.String()
	}

	return true
}

func (FeedCriteria) NftBelongsToCollection(q Query) bool {
	return q.NftID != "" && q.CollectionID != ""
}

func (FeedCriteria) CollectionHasNfts(q Query) bool {
	return len(q.CollectionNfts) > 0
}

func (FeedCriteria) CollectionHasCollectorsNote(q Query) bool {
	if q.CollectionCollectorsNote == "" {
		return false
	}

	if q.LastNftEvent != nil {
		return q.CollectionCollectorsNote != q.LastCollectionEvent.Data.CollectorsNote.String()
	}

	return true
}

func (FeedCriteria) CollectionHasNewTokensAdded(q Query) bool {
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
