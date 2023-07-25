package persist

import (
	"github.com/mikeydub/go-gallery/util"
)

type NotificationData struct {
	AuthedViewerIDs   []DBID    `json:"viewer_ids,omitempty"`
	UnauthedViewerIDs []string  `json:"unauthed_viewer_ids,omitempty"`
	FollowerIDs       []DBID    `json:"follower_ids,omitempty"`
	AdmirerIDs        []DBID    `json:"admirer_ids,omitempty"`
	FollowedBack      NullBool  `json:"followed_back,omitempty"`
	Refollowed        NullBool  `json:"refollowed,omitempty"`
	NewTokenID        DBID      `json:"new_token_ids,omitempty"`
	NewTokenQuantity  HexString `json:"new_token_quantity,omitempty"`
}

func (n NotificationData) Validate() NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = uniqueDBIDs(n.AdmirerIDs)
	result.FollowerIDs = uniqueDBIDs(n.FollowerIDs)
	result.AuthedViewerIDs = uniqueDBIDs(n.AuthedViewerIDs)
	result.UnauthedViewerIDs = uniqueStrings(n.UnauthedViewerIDs)
	result.NewTokenID = n.NewTokenID
	result.NewTokenQuantity = n.NewTokenQuantity

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
