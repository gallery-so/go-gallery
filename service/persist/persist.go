package persist

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/segmentio/ksuid"
)

// DBID represents a database ID
type DBID string

// CreationTime represents the time a record was created
type CreationTime time.Time

// LastUpdatedTime represents the time a record was last updated
type LastUpdatedTime time.Time

// GenerateID generates a application-wide unique ID
func GenerateID() DBID {
	id, err := ksuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return DBID(id.String())
}

func (d DBID) String() string {
	return string(d)
}

// Scan implements the database/sql Scanner interface for the DBID type
func (d *DBID) Scan(i interface{}) error {
	*d = DBID(i.(string))
	return nil
}

// Value implements the database/sql driver Valuer interface for the DBID type
func (d DBID) Value() (driver.Value, error) {
	if d.String() == "" {
		return GenerateID().String, nil
	}
	return d.String(), nil
}

// Time returns the time.Time representation of the CreationTime
func (c CreationTime) Time() time.Time {
	return time.Time(c)
}

// MarshalJSON returns the JSON representation of the CreationTime
func (c CreationTime) MarshalJSON() ([]byte, error) {
	bs, err := c.Time().MarshalJSON()
	if err != nil {
		return nil, err
	}
	return bs, nil
}

// UnmarshalJSON sets the CreationTime from the JSON representation
func (c *CreationTime) UnmarshalJSON(b []byte) error {
	t := time.Time{}
	err := json.Unmarshal(b, &t)
	if err != nil {
		return err
	}

	*c = CreationTime(t)
	return nil
}

// Scan implements the database/sql Scanner interface for the CreationTime type
func (c *CreationTime) Scan(i interface{}) error {
	*c = CreationTime(i.(time.Time))
	return nil
}

// Value implements the database/sql driver Valuer interface for the CreationTime type
func (c CreationTime) Value() (driver.Value, error) {
	if c.Time().IsZero() {
		return time.Now(), nil
	}
	return c.Time(), nil
}

// Time returns the time.Time representation of the LastUpdatedTime
func (l LastUpdatedTime) Time() time.Time {
	return time.Time(l)
}

// Scan implements the database/sql Scanner interface for the LastUpdatedTime type
func (l *LastUpdatedTime) Scan(i interface{}) error {
	*l = LastUpdatedTime(i.(time.Time))
	return nil
}

// Value implements the database/sql driver Valuer interface for the LastUpdatedTime type
func (l LastUpdatedTime) Value() (driver.Value, error) {
	if l.Time().IsZero() {
		return time.Now(), nil
	}
	return l.Time(), nil
}

// MarshalJSON returns the JSON representation of the LastUpdatedTime
func (l LastUpdatedTime) MarshalJSON() ([]byte, error) {
	bs, err := l.Time().MarshalJSON()
	if err != nil {
		return nil, err
	}
	return bs, nil
}

// UnmarshalJSON sets the LastUpdatedTime from the JSON representation
func (l *LastUpdatedTime) UnmarshalJSON(b []byte) error {
	t := time.Time{}
	err := json.Unmarshal(b, &t)
	if err != nil {
		return err
	}

	*l = LastUpdatedTime(t)
	return nil
}
