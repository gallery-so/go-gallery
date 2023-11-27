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
	ReportReasonSpamAndOrBot         ReportReason = "SPAM_AND_OR_BOT"
	ReportReasonInappropriateContent              = "INAPPROPRIATE_CONTENT"
	ReportReasonSomethingElse                     = "SOMETHING_ELSE"
)

type ReportReason string

func (r *ReportReason) UnmarshalGQL(v any) error {
	val, ok := v.(string)
	if !ok {
		return fmt.Errorf("ReportReason must be a string")
	}
	switch val {
	case "SPAM_AND_OR_BOT":
		*r = ReportReasonSpamAndOrBot
	case "INAPPROPRIATE_CONTENT":
		*r = ReportReasonInappropriateContent
	case "SOMETHING_ELSE":
		*r = ReportReasonSomethingElse
	}
	return nil
}

func (r ReportReason) MarshalGQL(w io.Writer) { w.Write([]byte(string(r))) }

func (r ReportReason) Value() (driver.Value, error) { return string(r), nil }

func (r *ReportReason) Scan(v any) error {
	*r = ReportReason(v.(byte))
	return nil
}
