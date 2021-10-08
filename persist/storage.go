package persist

import "github.com/segmentio/ksuid"

// DBID represents a database ID
type DBID string

// GenerateID generates a application-wide unique ID
func GenerateID() DBID {
	id, err := ksuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return DBID(id.String())
}
