package persist

import (
	"github.com/mikeydub/go-gallery/service/fingerprints"
)

type NotificationData struct {
	AuthedViewerIDs            []DBID                     `json:"viewer_ids"`
	UnauthedViewerFingerprints []fingerprints.Fingerprint `json:"viewer_ips"`
	FollowerIDs                []DBID                     `json:"follower_ids"`
	AdmirerIDs                 []DBID                     `json:"admirer_ids"`
	FollowedBack               bool                       `json:"followed_back"`
	Refollowed                 bool                       `json:"refollowed"`
}

func (n NotificationData) Validate() NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = uniqueDBIDs(n.AdmirerIDs)
	result.FollowerIDs = uniqueDBIDs(n.FollowerIDs)
	result.AuthedViewerIDs = uniqueDBIDs(n.AuthedViewerIDs)
	result.UnauthedViewerFingerprints = uniqueFingerprints(n.UnauthedViewerFingerprints)

	return result
}

func (n NotificationData) Concat(other NotificationData) NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = append(other.AdmirerIDs, n.AdmirerIDs...)
	result.FollowerIDs = append(other.FollowerIDs, n.FollowerIDs...)
	result.AuthedViewerIDs = append(other.AuthedViewerIDs, n.AuthedViewerIDs...)
	result.UnauthedViewerFingerprints = append(other.UnauthedViewerFingerprints, n.UnauthedViewerFingerprints...)

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

func uniqueFingerprints(strs []fingerprints.Fingerprint) []fingerprints.Fingerprint {
	seen := make(map[fingerprints.Fingerprint]bool)
	result := []fingerprints.Fingerprint{}

	for _, fpt := range strs {
		if _, ok := seen[fpt]; !ok {
			seen[fpt] = true
			result = append(result, fpt)
		}
	}

	return result
}
