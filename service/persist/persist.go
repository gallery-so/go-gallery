package persist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgtype"
	"github.com/lib/pq"

	"github.com/segmentio/ksuid"
)

var cleanString = func(r rune) rune {
	if unicode.IsGraphic(r) || unicode.IsPrint(r) {
		return r
	}
	return -1
}

// DBID represents a database ID
type DBID string

// DBIDList is a slice of DBIDs, used to implement scanner/valuer interfaces
type DBIDList []DBID

type DBIDTuple [2]DBID

func (l DBIDList) Value() (driver.Value, error) {
	return pq.Array(l).Value()
}

// Scan implements the Scanner interface for the DBIDList type
func (l *DBIDList) Scan(value interface{}) error {
	return pq.Array(l).Scan(value)
}

var notFoundError ErrNotFound

// ErrNotFound is a general error for when some entity is not found.
// Errors should wrap this error to provide more details on what was not found (e.g. ErrUserNotFound)
// and how it was not found (e.g. ErrUserNotFoundByID)
type ErrNotFound struct{}

func (e ErrNotFound) Error() string { return "entity not found" }

// NullString represents a string that may be null in the DB
type NullString string

// NullInt64 represents an int64 that may be null in the DB
type NullInt64 int64

// NullInt32 represents an int32 that may be null in the DB
type NullInt32 int32

// NullBool represents a bool that may be null in the DB
type NullBool bool

type CompleteIndex struct {
	Index  int `json:"start"`
	Length int `json:"end"`
}

func (c CompleteIndex) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *CompleteIndex) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]uint8), c)
}

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
	switch v := i.(type) {
	case DBID:
		*d = v
	case string:
		*d = DBID(v)
	}
	return nil
}

// Value implements the database/sql driver Valuer interface for the DBID type
func (d DBID) Value() (driver.Value, error) {
	return d.String(), nil
}

func (n NullString) String() string {
	return string(n)
}

// Value implements the database/sql driver Valuer interface for the NullString type
func (n NullString) Value() (driver.Value, error) {
	return strings.ToValidUTF8(strings.ReplaceAll(n.String(), "\\u0000", ""), ""), nil
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

// Int32 returns the int32 representation of the NullInt32
func (n NullInt32) Int32() int32 {
	return int32(n)
}

// Int returns the int representation of the NullInt32
func (n NullInt32) Int() int {
	return int(n)
}

func (n NullInt32) String() string {
	return fmt.Sprint(n.Int32())
}

// Value implements the database/sql driver Valuer interface for the NullInt32 type
func (n NullInt32) Value() (driver.Value, error) {
	return n.Int32(), nil
}

// Scan implements the database/sql Scanner interface for the NullInt32 type
func (n *NullInt32) Scan(value interface{}) error {
	if value == nil {
		*n = NullInt32(0)
		return nil
	}
	// database/sql spec says integer values should be returned as int64, even if the underlying column is int32
	*n = NullInt32(value.(int64))
	return nil
}

// Bool returns the bool representation of the NullBool
func (n NullBool) Bool() bool {
	return bool(n)
}

func (n NullBool) BoolPointer() *bool {
	res := bool(n)
	return &res
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

// RemoveDuplicateDBIDs ensures that an array of DBIDs has no repeat items
func RemoveDuplicateDBIDs(a []DBID) []DBID {
	result := make([]DBID, 0, len(a))
	m := map[DBID]bool{}

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			result = append(result, val)
		}
	}

	return result
}

// RemoveDuplicateAddresses ensures that an array of addresses has no repeat items
func RemoveDuplicateAddresses(a []EthereumAddress) []EthereumAddress {
	result := make([]EthereumAddress, 0, len(a))
	m := map[EthereumAddress]bool{}

	for _, val := range a {
		if _, ok := m[val]; !ok {
			m[val] = true
			result = append(result, val)
		}
	}

	return result
}

func ContainsDBID(pSrc []DBID, pID DBID) bool {
	for _, v := range pSrc {
		if v == pID {
			return true
		}
	}
	return false
}

func ToDBIDs[T any](them []T, convert func(T) (DBID, error)) ([]DBID, error) {
	result := make([]DBID, len(them))
	for i, v := range them {
		d, err := convert(v)
		if err != nil {
			return nil, err
		}
		result[i] = d
	}
	return result, nil
}

func ToJSONB(v any) (pgtype.JSONB, error) {
	byt, err := json.Marshal(v)
	if err != nil {
		return pgtype.JSONB{}, err
	}
	ret := pgtype.JSONB{}
	err = ret.Set(byt)
	return ret, err
}
