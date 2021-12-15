package persist

import (
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

// Time returns the time.Time representation of the LastUpdatedTime
func (l LastUpdatedTime) Time() time.Time {
	return time.Time(l)
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
