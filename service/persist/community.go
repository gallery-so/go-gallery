package persist

import (
	"fmt"
	"io"
	"strings"
)

type CommunityType int

const (
	CommunityTypeContract CommunityType = iota
	CommunityTypeProhibition
)

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (c *CommunityType) UnmarshalGQL(v interface{}) error {
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("Chain must be an string")
	}

	switch strings.ToLower(n) {
	case "contract":
		*c = CommunityTypeContract
	case "prohibition":
		*c = CommunityTypeProhibition
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (c CommunityType) MarshalGQL(w io.Writer) {
	switch c {
	case CommunityTypeContract:
		w.Write([]byte(`"Contract"`))
	case CommunityTypeProhibition:
		w.Write([]byte(`"Prohibition"`))
	}
}

type CommunityKey struct {
	Type    CommunityType
	Subtype string
	Key     string
}

func (k CommunityKey) String() string {
	return fmt.Sprintf("%d:%s:%s", k.Type, k.Subtype, k.Key)
}

type ErrCommunityNotFound struct {
	ID  DBID
	Key CommunityKey
}

func (e ErrCommunityNotFound) Error() string {
	return fmt.Sprintf("Community not found for contractID %s, key %s", e.ID, e.Key)
}
