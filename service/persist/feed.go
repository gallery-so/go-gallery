package persist

type FeedEntityType int

const (
	FeedEventTypeTag FeedEntityType = iota
	PostTypeTag
)

type IDsInStruct struct {
	IDs []DBID `json:"ids"`
}
