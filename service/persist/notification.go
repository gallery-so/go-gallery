package persist

type NotificationData struct {
	GalleryID   DBID     `json:"gallery_id"`
	ViewerIDs   []DBID   `json:"viewer_ids"`
	ViewerIPs   []string `json:"viewer_ips"`
	FollowerIDs []DBID   `json:"follower_ids"`
	FeedEventID DBID     `json:"feed_event_id"`
	CommentID   DBID     `json:"comment_id"`
	AdmirerIDs  []DBID   `json:"admirer_ids"`
}

func (n NotificationData) Validate() NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = uniqueDBIDs(n.AdmirerIDs)
	result.FollowerIDs = uniqueDBIDs(n.FollowerIDs)
	result.ViewerIDs = uniqueDBIDs(n.ViewerIDs)
	result.ViewerIPs = uniqueStrings(n.ViewerIPs)

	return result
}

func (n NotificationData) Concat(other NotificationData) NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = append(other.AdmirerIDs, n.AdmirerIDs...)
	result.FollowerIDs = append(other.FollowerIDs, n.FollowerIDs...)
	result.ViewerIDs = append(other.ViewerIDs, n.ViewerIDs...)
	result.ViewerIPs = append(other.ViewerIPs, n.ViewerIPs...)
	result.FeedEventID = other.FeedEventID
	result.CommentID = other.CommentID
	result.GalleryID = other.GalleryID

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

	for _, id := range strs {
		if _, ok := seen[id]; !ok {
			seen[id] = true
			result = append(result, id)
		}
	}

	return result
}
