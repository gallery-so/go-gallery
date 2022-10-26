package persist

import (
	"database/sql/driver"
	"encoding/json"
)

type EmailType string

type EmailVerificationStatus int

const (
	EmailTypeNotifications EmailType = "notifications"
	EmailTypeAdmin                   = "admin"
)

const (
	EmailVerificationStatusUnverified EmailVerificationStatus = iota
	EmailVerificationStatusVerified
	EmailVerificationStatusAdmin
)

type EmailUnsubscriptions struct {
	All           NullBool `json:"all"`
	Notifications NullBool `json:"notifications"`
}

func (e EmailUnsubscriptions) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e *EmailUnsubscriptions) Scan(src interface{}) error {
	return json.Unmarshal(src.([]byte), e)
}

func (e EmailType) Value() (driver.Value, error) {
	return string(e), nil
}

func (e *EmailType) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*e = EmailType(src.(int32))
	return nil
}

func (e EmailType) String() string {
	return string(e)
}
