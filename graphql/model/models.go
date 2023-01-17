package model

import (
	"fmt"
	"io"
	"time"

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

func (r *Community) GetGqlIDField_ContractAddress() string {
	return r.ContractAddress.Address().String()
}

func (v *Viewer) GetGqlIDField_UserID() string {
	return string(v.UserId)
}

type HelperCollectionTokenData struct {
	TokenId      persist.DBID
	CollectionId persist.DBID
}

type HelperTokenHolderData struct {
	UserId    persist.DBID
	WalletIds []persist.DBID
}

type HelperViewerData struct {
	UserId persist.DBID
}

type HelperCommunityData struct {
	ForceRefresh *bool
}

type HelperTokensAddedToCollectionFeedEventDataData struct {
	TokenIDs     persist.DBIDList
	CollectionID persist.DBID
}

type HelperCollectionCreatedFeedEventDataData struct {
	TokenIDs     persist.DBIDList
	CollectionID persist.DBID
}

type HelperGroupNotificationUsersConnectionData struct {
	UserIDs persist.DBIDList
}

type HelperGalleryUserData struct {
	UserID            persist.DBID
	FeaturedGalleryID *persist.DBID
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
	UserId persist.DBID
}

type HelperCollectionUpdatedFeedEventDataData struct {
	TokenIDs     persist.DBIDList
	CollectionID persist.DBID
}

type HelperGalleryUpdatedFeedEventDataData struct {
	FeedEventID persist.DBID
}

type HelperGalleryCollectionUpdateData struct {
	CollectionID persist.DBID
}

type HelperGalleryTokenUpdateData struct {
	TokenID persist.DBID
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

type Window struct {
	time.Duration
	Name string
}

var (
	lastSevenDaysWindow = Window{7 * 24 * time.Hour, "LAST_7_DAYS"}
	allTimeWindow       = Window{1<<63 - 1, "ALL_TIME"}
)

func (w *Window) UnmarshalGQL(v interface{}) error {
	window, ok := v.(string)
	if !ok {
		return fmt.Errorf("Window must be a string")
	}
	switch window {
	case lastSevenDaysWindow.Name:
		*w = lastSevenDaysWindow
	case allTimeWindow.Name:
		*w = allTimeWindow
	default:
		panic(fmt.Sprintf("unknown window: %s", window))
	}
	return nil
}

func (w Window) MarshalGQL(wt io.Writer) {
	switch {
	case w == lastSevenDaysWindow:
		wt.Write([]byte(fmt.Sprintf(`"%s"`, lastSevenDaysWindow.Name)))
	case w == allTimeWindow:
		wt.Write([]byte(fmt.Sprintf(`"%s"`, allTimeWindow.Name)))
	default:
		panic(fmt.Sprintf("unknown window: %v", w))
	}
}
