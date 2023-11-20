package persist

import (
	"database/sql/driver"
	"fmt"
	"io"
)

type FeedEntityType int

const (
	FeedEventTypeTag FeedEntityType = iota
	PostTypeTag
)

const (
	ReportPostReasonSpamAndOrBot         ReportPostReason = "SPAM_AND_OR_BOT"
	ReportPostReasonInappropriateContent                  = "INAPPROPRIATE_CONTENT"
	ReportPostReasonSomethingElse                         = "SOMETHING_ELSE"
)

type ReportPostReason string

func (r *ReportPostReason) UnmarshalGQL(v any) error {
	val, ok := v.(string)
	if !ok {
		return fmt.Errorf("ReportPostReason must be a string")
	}
	switch val {
	case "SPAM_AND_OR_BOT":
		*r = ReportPostReasonSpamAndOrBot
	case "INAPPROPRIATE_CONTENT":
		*r = ReportPostReasonInappropriateContent
	case "SOMETHING_ELSE":
		*r = ReportPostReasonSomethingElse
	}
	return nil
}

func (r ReportPostReason) MarshalGQL(w io.Writer) { w.Write([]byte(string(r))) }

func (r ReportPostReason) Value() (driver.Value, error) { return string(r), nil }

func (r *ReportPostReason) Scan(v any) error {
	*r = ReportPostReason(v.(byte))
	return nil
}
