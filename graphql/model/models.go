package model

import (
	"fmt"
	"io"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type GqlID string

func (r *CollectionToken) GetGqlIDField_TokenID() string {
	return r.HelperCollectionTokenData.TokenId.String()
}

func (r *CollectionToken) GetGqlIDField_CollectionID() string {
	return r.HelperCollectionTokenData.CollectionId.String()
}

func (v *Viewer) GetGqlIDField_UserID() string {
	return string(v.UserId)
}

type HelperCollectionTokenData struct {
	TokenId      persist.DBID
	CollectionId persist.DBID
}

type HelperTokenHolderData struct {
	UserId     persist.DBID
	WalletIds  []persist.DBID
	ContractId persist.DBID
}

type HelperViewerData struct {
	UserId persist.DBID
}

type HelperCommunityData struct {
	Community db.Community
}

type HelperContractCommunityData struct {
	Community db.Community
}

type HelperArtBlocksCommunityData struct {
	Community db.Community
}

type HelperTokensAddedToCollectionFeedEventDataData struct {
	TokenIDs     persist.DBIDList
	CollectionID persist.DBID
}

type HelperCollectionCreatedFeedEventDataData struct {
	TokenIDs     persist.DBIDList
	CollectionID persist.DBID
}

type HelperGalleryUserData struct {
	UserID            persist.DBID
	FeaturedGalleryID *persist.DBID
	Traits            persist.Traits
}

type HelperCommentData struct {
	PostID      *persist.DBID
	FeedEventID *persist.DBID
	ReplyToID   *persist.DBID
}

type HelperMentionData struct {
	UserID      *persist.DBID
	CommunityID *persist.DBID
}

type HelperAdmireData struct {
	PostID      *persist.DBID
	FeedEventID *persist.DBID
	CommentID   *persist.DBID
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

type HelperSomeoneCommentedOnYourPostNotificationData struct {
	OwnerID          persist.DBID
	PostID           persist.DBID
	CommentID        persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneAdmiredYourFeedEventNotificationData struct {
	OwnerID          persist.DBID
	FeedEventID      persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneAdmiredYourPostNotificationData struct {
	OwnerID          persist.DBID
	PostID           persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneAdmiredYourCommentNotificationData struct {
	CommentID        persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneAdmiredYourTokenNotificationData struct {
	OwnerID          persist.DBID
	TokenID          persist.DBID
	NotificationData persist.NotificationData
}

type HelperNewTokensNotificationData struct {
	OwnerID          persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneRepliedToYourCommentNotificationData struct {
	OwnerID          persist.DBID
	CommentID        persist.DBID
	NotificationData persist.NotificationData
}

type HelperSomeoneMentionedYouNotificationData struct {
	PostID    *persist.DBID
	CommentID *persist.DBID
}
type HelperSomeoneMentionedYourCommunityNotificationData struct {
	CommunityID persist.DBID
	PostID      *persist.DBID
	CommentID   *persist.DBID
}

type HelperSomeonePostedYourWorkNotificationData struct {
	CommunityID persist.DBID
	PostID      persist.DBID
}

type HelperSomeoneYouFollowPostedTheirFirstPostNotificationData struct {
	PostID persist.DBID
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

type HelperSocialConnectionData struct {
	UserID        persist.DBID
	UserCreatedAt time.Time
}

type HelperTokenData struct {
	Token        db.Token
	CollectionID *persist.DBID
}

type HelperTokenDefinitionData struct {
	Definition db.TokenDefinition
}

type HelperEnsProfileImageData struct {
	UserID    persist.DBID
	WalletID  persist.DBID
	EnsDomain string
}

type HelperPostData struct {
	TokenIDs persist.DBIDList
	AuthorID persist.DBID
}

type HelperPostComposerDraftDetailsPayloadData struct {
	Token           persist.TokenIdentifiers
	TokenDefinition db.TokenDefinition
	TokenMedia      db.TokenMedia
	ContractID      persist.DBID
}

type HelperSomeoneYouFollowOnFarcasterJoinedNotificationData struct {
	UserID persist.DBID
}

type HelperHighlightMintClaimStatusPayloadData struct {
	TokenID persist.DBID
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
	lastFiveDaysWindow  = Window{5 * 24 * time.Hour, "LAST_5_DAYS"}
	lastSevenDaysWindow = Window{7 * 24 * time.Hour, "LAST_7_DAYS"}
	allTimeWindow       = Window{1<<63 - 1, "ALL_TIME"}
)

func (w *Window) UnmarshalGQL(v interface{}) error {
	window, ok := v.(string)
	if !ok {
		return fmt.Errorf("Window must be a string")
	}
	switch window {
	case lastFiveDaysWindow.Name:
		*w = lastFiveDaysWindow
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
	case w == lastFiveDaysWindow:
		wt.Write([]byte(fmt.Sprintf(`"%s"`, lastFiveDaysWindow.Name)))
	case w == lastSevenDaysWindow:
		wt.Write([]byte(fmt.Sprintf(`"%s"`, lastSevenDaysWindow.Name)))
	case w == allTimeWindow:
		wt.Write([]byte(fmt.Sprintf(`"%s"`, allTimeWindow.Name)))
	default:
		panic(fmt.Sprintf("unknown window: %v", w))
	}
}
