package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type EmailType string

type EmailVerificationStatus int

var emailVerificationStatuses = []string{"Unverified", "Verified", "Failed", "Admin"}

const (
	EmailTypeNotifications EmailType = "notifications"
	EmailTypeAdmin                   = "admin"
)

const (
	EmailVerificationStatusUnverified EmailVerificationStatus = iota
	EmailVerificationStatusVerified
	EmailVerificationStatusFailed
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

func (e EmailVerificationStatus) Value() (driver.Value, error) {
	return int32(e), nil
}

func (e *EmailVerificationStatus) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	*e = EmailVerificationStatus(src.(int64))
	return nil
}

func (e EmailVerificationStatus) String() string {
	return emailVerificationStatuses[e]
}

func (e EmailVerificationStatus) IsVerified() bool {
	return e == EmailVerificationStatusVerified || e == EmailVerificationStatusAdmin
}

func (e EmailVerificationStatus) MarshalGQL(w io.Writer) {
	w.Write([]byte(fmt.Sprintf(`"%s"`, e.String())))
}

func (e *EmailVerificationStatus) UnmarshalGQL(v interface{}) error {
	switch v := v.(type) {
	case string:
		lower := strings.ToLower(v)
		for i, s := range emailVerificationStatuses {
			if strings.EqualFold(s, lower) {
				*e = EmailVerificationStatus(i)
				return nil
			}
		}
		return nil
	default:
		return nil
	}
}
