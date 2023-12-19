package persist

import (
	"github.com/mikeydub/go-gallery/util"
)

type AnnouncementDetails struct {
	Platform             string `json:"platform"`
	InternalID           string `json:"internal_id"`
	ImageURL             string `json:"image_url,omitempty"`
	Title                string `json:"title,omitempty"`
	Description          string `json:"description,omitempty"`
	CTAText              string `json:"cta_text,omitempty"`
	CTALink              string `json:"cta_link,omitempty"`
	PushNotificationText string `json:"push_notification_text,omitempty"`
}

type NotificationData struct {
	AuthedViewerIDs        []DBID               `json:"viewer_ids,omitempty"`
	UnauthedViewerIDs      []string             `json:"unauthed_viewer_ids,omitempty"`
	FollowerIDs            []DBID               `json:"follower_ids,omitempty"`
	AdmirerIDs             []DBID               `json:"admirer_ids,omitempty"`
	FollowedBack           NullBool             `json:"followed_back,omitempty"`
	Refollowed             NullBool             `json:"refollowed,omitempty"`
	NewTokenID             DBID                 `json:"new_token_id,omitempty"`
	NewTokenQuantity       HexString            `json:"new_token_quantity,omitempty"`
	OriginalCommentID      DBID                 `json:"original_comment_id,omitempty"`
	ActivityBadgeThreshold int                  `json:"activity_badge_threshold,omitempty"`
	NewTopActiveUser       bool                 `json:"new_top_active_user,omitempty"`
	AnnouncementDetails    *AnnouncementDetails `json:"announcement_details,omitempty"`
}

func (n NotificationData) Validate() NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = uniqueDBIDs(n.AdmirerIDs)
	result.FollowerIDs = uniqueDBIDs(n.FollowerIDs)
	result.AuthedViewerIDs = uniqueDBIDs(n.AuthedViewerIDs)
	result.UnauthedViewerIDs = uniqueStrings(n.UnauthedViewerIDs)
	result.NewTokenID = n.NewTokenID
	result.NewTokenQuantity = n.NewTokenQuantity
	result.OriginalCommentID = n.OriginalCommentID
	result.FollowedBack = n.FollowedBack
	result.Refollowed = n.Refollowed
	result.ActivityBadgeThreshold = n.ActivityBadgeThreshold
	result.NewTopActiveUser = n.NewTopActiveUser
	result.AnnouncementDetails = n.AnnouncementDetails

	return result
}

func (n NotificationData) Concat(other NotificationData) NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = append(other.AdmirerIDs, n.AdmirerIDs...)
	result.FollowerIDs = append(other.FollowerIDs, n.FollowerIDs...)
	result.AuthedViewerIDs = append(other.AuthedViewerIDs, n.AuthedViewerIDs...)
	result.UnauthedViewerIDs = append(other.UnauthedViewerIDs, n.UnauthedViewerIDs...)
	result.FollowedBack = other.FollowedBack || n.FollowedBack
	result.Refollowed = other.Refollowed || n.Refollowed
	result.NewTokenQuantity = other.NewTokenQuantity.Add(n.NewTokenQuantity)
	result.NewTokenID = DBID(util.FirstNonEmptyString(other.NewTokenID.String(), n.NewTokenID.String()))
	result.OriginalCommentID = DBID(util.FirstNonEmptyString(other.OriginalCommentID.String(), n.OriginalCommentID.String()))
	result.ActivityBadgeThreshold, _ = util.FindFirst([]int{other.ActivityBadgeThreshold, n.ActivityBadgeThreshold}, func(i int) bool {
		return i > 0
	})
	result.NewTopActiveUser = other.NewTopActiveUser || n.NewTopActiveUser
	if other.AnnouncementDetails != nil {
		result.AnnouncementDetails = other.AnnouncementDetails
	} else {
		result.AnnouncementDetails = n.AnnouncementDetails
	}

	return result.Validate()
}

func uniqueDBIDs(ids []DBID) []DBID {
	seen := make(map[DBID]bool)
	result := []DBID{}

	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = true
			result = append(result, id)
		}
	}

	return result
}

func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, str := range strs {
		if _, ok := seen[str]; !ok {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}
