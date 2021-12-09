package persist

import (
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
