package model

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

type GqlID string

func (r *CollectionToken) GetGqlIDField_TokenID() string {
	return r.HelperCollectionTokenData.TokenId.String()
}

func (r *CollectionToken) GetGqlIDField_CollectionID() string {
	return r.HelperCollectionTokenData.CollectionId.String()
}

func (r *Community) GetGqlIDField_Chain() string {
	return fmt.Sprint(r.ContractAddress.Chain())
}

func (r *NotificationSettings) GetGqlIDField_UserID() string {
	return fmt.Sprint(r.HelperNotificationSettingsData.UserId.String())
}

func (r *Community) GetGqlIDField_ContractAddress() string {
	return r.ContractAddress.Address().String()
}

type HelperCollectionTokenData struct {
	TokenId      persist.DBID
	CollectionId persist.DBID
}

type HelperTokenHolderData struct {
	UserId    persist.DBID
	WalletIds []persist.DBID
}

type HelperCommunityData struct {
	ForceRefresh *bool
}

type HelperTokensAddedToCollectionFeedEventDataData struct {
	FeedEventID persist.DBID
}

type HelperCollectionCreatedFeedEventDataData struct {
	FeedEventID persist.DBID
}

type HelperFeedConnectionData struct {
	UserId persist.DBID
}

type HelperGroupNotificationUsersConnectionData struct {
	UserIDs persist.DBIDList
}

type HelperNotificationSettingsData struct {
	UserId persist.DBID
}

type HelperSomeoneFollowedYouNotificationData struct {
	OwnerID          persist.DBID
	NotificationData persist.NotificationData
}
type HelperSomeoneViewedYourGalleryNotificationData struct {
	OwnerID          persist.DBID
	GalleryID        persist.DBID
	NotificationData persist.NotificationData
}
type HelperSomeoneFollowedYouBackNotificationData struct {
	OwnerID          persist.DBID
	NotificationData persist.NotificationData
}
type HelperSomeoneCommentedOnYourFeedEventNotificationData struct {
	OwnerID          persist.DBID
	FeedEventID      persist.DBID
	CommentID        persist.DBID
	NotificationData persist.NotificationData
}
type HelperSomeoneAdmiredYourFeedEventNotificationData struct {
	OwnerID          persist.DBID
	FeedEventID      persist.DBID
	NotificationData persist.NotificationData
}

type HelperNotificationsConnectionData struct {
	UserId  persist.DBID
	ByFirst bool
}

type HelperUserEmailData struct {
	UserId persist.DBID
}

type ErrInvalidIDFormat struct {
	message string
}

func (e ErrInvalidIDFormat) Error() string {
	return fmt.Sprintf("invalid ID format: %s", e.message)
}

type ErrInvalidIDType struct {
	typeName string
}

func (e ErrInvalidIDType) Error() string {
	return fmt.Sprintf("no fetch method found for ID type '%s'", e.typeName)
}
