package persist

import (
	"database/sql/driver"
	"encoding/json"
)

type EmailType int

const (
	EmailTypeNotifications EmailType = iota
	EmailTypeAdmin
)

type EmailNotificationSettings struct {
	UnsubscribedFromAll       NullBool    `json:"unsubscribed_from_all"`
	IndividualUnsubscriptions []EmailType `json:"individual_unsubscriptions"`
}

func (e EmailNotificationSettings) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e *EmailNotificationSettings) Scan(src interface{}) error {
	return json.Unmarshal(src.([]byte), e)
}

func (e EmailType) Value() (driver.Value, error) {
	return int(e), nil
}

func (e *EmailType) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*e = EmailType(src.(int32))
	return nil
}

func (e *EmailType) String() string {
	switch *e {
	case EmailTypeNotifications:
		return "notifications"
	case EmailTypeAdmin:
		return "admin"
	default:
		return "unknown"
	}
}
