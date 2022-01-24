package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
)

// DBID represents a database ID
type DBID string

// CreationTime represents the time a record was created
type CreationTime time.Time

// LastUpdatedTime represents the time a record was last updated
type LastUpdatedTime time.Time

// NullString represents a string that may be null in the DB
type NullString string

// NullInt64 represents an int64 that may be null in the DB
type NullInt64 int64

// NullBool represents a bool that may be null in the DB
type NullBool bool

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
	if i == nil {
		*d = DBID("")
		return nil
	}
	if it, ok := i.([]uint8); ok {
		*d = DBID(it)
		return nil
	}
	*d = DBID(i.(string))
	return nil
}

// Value implements the database/sql driver Valuer interface for the DBID type
func (d DBID) Value() (driver.Value, error) {
	if d.String() == "" {
		return GenerateID().String(), nil
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
	if i == nil {
		*c = CreationTime{}
		return nil
	}
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
	if i == nil {
		*l = LastUpdatedTime{}
		return nil
	}
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

func (n NullString) String() string {
	return string(n)
}

// Value implements the database/sql driver Valuer interface for the NullString type
func (n NullString) Value() (driver.Value, error) {
	if n.String() == "" {
		return "", nil
	}
	return strings.ToValidUTF8(n.String(), ""), nil
}

// Scan implements the database/sql Scanner interface for the NullString type
func (n *NullString) Scan(value interface{}) error {
	if value == nil {
		*n = NullString("")
		return nil
	}
	*n = NullString(value.(string))
	return nil
}

// Int64 returns the int64 representation of the NullInt64
func (n NullInt64) Int64() int64 {
	return int64(n)
}

func (n NullInt64) String() string {
	return fmt.Sprint(n.Int64())
}

// Value implements the database/sql driver Valuer interface for the NullInt64 type
func (n NullInt64) Value() (driver.Value, error) {
	return n.Int64(), nil
}

// Scan implements the database/sql Scanner interface for the NullInt64 type
func (n *NullInt64) Scan(value interface{}) error {
	if value == nil {
		*n = NullInt64(0)
		return nil
	}
	*n = NullInt64(value.(int64))
	return nil
}

// Bool returns the bool representation of the NullBool
func (n NullBool) Bool() bool {
	return bool(n)
}

func (n NullBool) String() string {
	return fmt.Sprint(n.Bool())
}

// Value implements the database/sql driver Valuer interface for the NullBool type
func (n NullBool) Value() (driver.Value, error) {
	return n.Bool(), nil
}

// Scan implements the database/sql Scanner interface for the NullBool type
func (n *NullBool) Scan(value interface{}) error {
	if value == nil {
		*n = NullBool(false)
		return nil
	}
	*n = NullBool(value.(bool))
	return nil
}
