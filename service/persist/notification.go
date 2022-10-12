package persist

type NotificationData struct {
	GalleryID   DBID   `json:"gallery_id"`
	ViewerIDs   []DBID `json:"viewer_ids"`
	FollowerIDs []DBID `json:"follower_ids"`
	FeedEventID DBID   `json:"feed_event_id"`
	CommentID   DBID   `json:"comment_id"`
	AdmirerIDs  []DBID `json:"admirer_ids"`
}

func (n NotificationData) Validate() NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = uniqueDBIDs(n.AdmirerIDs)
	result.FollowerIDs = uniqueDBIDs(n.FollowerIDs)
	result.ViewerIDs = uniqueDBIDs(n.ViewerIDs)

	return result
}

func (n NotificationData) Concat(other NotificationData) NotificationData {
	result := NotificationData{}
	result.AdmirerIDs = append(n.AdmirerIDs, other.AdmirerIDs...)
	result.FollowerIDs = append(n.FollowerIDs, other.FollowerIDs...)
	result.ViewerIDs = append(n.ViewerIDs, other.ViewerIDs...)
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
