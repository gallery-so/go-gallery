package persist

import (
	"database/sql/driver"
	"encoding/json"
)

type EmailNotificationSettings struct {
	UnsubscribedFromAll NullBool `json:"unsubscribed_from_all"`
}

func (e EmailNotificationSettings) Value() (driver.Value, error) {
	return json.Marshal(e)
}

func (e *EmailNotificationSettings) Scan(src interface{}) error {
	return json.Unmarshal(src.([]byte), e)
}
