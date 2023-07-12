package persist

const (
	FeedEventTypeTag = iota
	PostTypeTag
)

type IDsInStruct struct {
	IDs []DBID `json:"ids"`
}
