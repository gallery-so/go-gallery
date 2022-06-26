package cursor

import (
	"encoding/base64"

	"github.com/mikeydub/go-gallery/service/persist"
)

func DBIDEncodeToCursor(id persist.DBID) string {
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func DecodeToDBID(cursor *string) (*persist.DBID, error) {
	if cursor == nil {
		return nil, nil
	}

	dec, err := base64.StdEncoding.DecodeString(string(*cursor))
	if err != nil {
		return nil, err
	}

	dbid := persist.DBID(dec)

	return &dbid, nil
}
