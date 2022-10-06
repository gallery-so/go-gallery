package persist

type NotificationData struct {
	GalleryID   DBID   `json:"gallery_id"`
	ViewerIDs   []DBID `json:"viewer_ids"`
	FollowerIDs []DBID `json:"follower_ids"`
	FeedEventID DBID   `json:"feed_event_id"`
	CommentID   DBID   `json:"comment_id"`
	AdmirerIDs  []DBID `json:"admirer_ids"`
}
