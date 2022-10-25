package persist

import (
	"fmt"
)

type ResourceType int
type Action string

const (
	ResourceTypeUser ResourceType = iota
	ResourceTypeToken
	ResourceTypeCollection
	ResourceTypeGallery
	ResourceTypeAdmire
	ResourceTypeComment
	ResourceTypeFeedEvent
	ActionUserCreated                     Action = "UserCreated"
	ActionUserFollowedUsers               Action = "UserFollowedUsers"
	ActionCollectorsNoteAddedToToken      Action = "CollectorsNoteAddedToToken"
	ActionCollectionCreated               Action = "CollectionCreated"
	ActionCollectorsNoteAddedToCollection Action = "CollectorsNoteAddedToCollection"
	ActionTokensAddedToCollection         Action = "TokensAddedToCollection"
	ActionAdmiredFeedEvent                Action = "AdmiredFeedEvent"
	ActionCommentedOnFeedEvent            Action = "CommentedOnFeedEvent"
	ActionViewedGallery                   Action = "ViewedGallery"
)

type EventData struct {
	UserBio                  string   `json:"user_bio"`
	UserFollowedBack         bool     `json:"user_followed_back"`
	UserRefollowed           bool     `json:"user_refollowed"`
	TokenCollectorsNote      string   `json:"token_collectors_note"`
	TokenCollectionID        DBID     `json:"token_collection_id"`
	CollectionTokenIDs       DBIDList `json:"collection_token_ids"`
	CollectionCollectorsNote string   `json:"collection_collectors_note"`
}

type FeedEventData struct {
	UserBio                     string   `json:"user_bio"`
	UserFollowedIDs             DBIDList `json:"user_followed_ids"`
	UserFollowedBack            []bool   `json:"user_followed_back"`
	TokenID                     DBID     `json:"token_id"`
	TokenCollectionID           DBID     `json:"token_collection_id"`
	TokenNewCollectorsNote      string   `json:"token_new_collectors_note"`
	CollectionID                DBID     `json:"collection_id"`
	CollectionTokenIDs          DBIDList `json:"collection_token_ids"`
	CollectionNewTokenIDs       DBIDList `json:"collection_new_token_ids"`
	CollectionNewCollectorsNote string   `json:"collection_new_collectors_note"`
	CollectionIsPreFeed         bool     `json:"collection_is_pre_feed"`
}

type ErrFeedEventNotFoundByID struct {
	ID DBID
}

func (e ErrFeedEventNotFoundByID) Error() string {
	return fmt.Sprintf("event not found by id: %s", e.ID)
}

type ErrUnknownAction struct {
	Action Action
}

func (e ErrUnknownAction) Error() string {
	return fmt.Sprintf("unknown action: %s", e.Action)
}

type ErrUnknownResourceType struct {
	ResourceType ResourceType
}

func (e ErrUnknownResourceType) Error() string {
	return fmt.Sprintf("unknown resource type: %v", e.ResourceType)
}
