package persist

import (
	"database/sql"
	"fmt"
)

type ResourceType int
type Action string
type ActionList []Action

const (
	ResourceTypeUser ResourceType = iota
	ResourceTypeToken
	ResourceTypeCollection
	ResourceTypeGallery
	ResourceTypeAdmire
	ResourceTypeComment
	ActionUserCreated                     Action = "UserCreated"
	ActionUserFollowedUsers               Action = "UserFollowedUsers"
	ActionCollectorsNoteAddedToToken      Action = "CollectorsNoteAddedToToken"
	ActionCollectionCreated               Action = "CollectionCreated"
	ActionCollectorsNoteAddedToCollection Action = "CollectorsNoteAddedToCollection"
	ActionTokensAddedToCollection         Action = "TokensAddedToCollection"
	ActionAdmiredFeedEvent                Action = "AdmiredFeedEvent"
	ActionCommentedOnFeedEvent            Action = "CommentedOnFeedEvent"
	ActionViewedGallery                   Action = "ViewedGallery"
	ActionCollectionUpdated               Action = "CollectionUpdated"
	ActionGalleryUpdated                  Action = "GalleryUpdated"
	ActionGalleryInfoUpdated              Action = "GalleryInfoUpdated"
)

type EventData struct {
	UserBio                  string   `json:"user_bio"`
	UserFollowedBack         bool     `json:"user_followed_back"`
	UserRefollowed           bool     `json:"user_refollowed"`
	TokenCollectorsNote      string   `json:"token_collectors_note"`
	TokenCollectionID        DBID     `json:"token_collection_id"`
	CollectionTokenIDs       DBIDList `json:"collection_token_ids"`
	CollectionCollectorsNote string   `json:"collection_collectors_note"`
	GalleryName              string   `json:"gallery_name"`
	GalleryDescription       string   `json:"gallery_description"`
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
	CollectionIsNew             bool     `json:"collection_is_new"`
	GalleryID                   DBID     `json:"gallery_id"`
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

func StrToNullStr(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: *s}
}

func NullStrToStr(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

func DBIDToNullStr(id DBID) sql.NullString {
	s := id.String()
	return StrToNullStr(&s)
}

func NullStrToDBID(s sql.NullString) DBID {
	return DBID(NullStrToStr(s))
}
