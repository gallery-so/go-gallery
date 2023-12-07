package persist

import (
	"fmt"
	"io"
	"strings"
)

type CommunityType int

const (
	CommunityTypeContract CommunityType = iota
	CommunityTypeArtBlocks
)

type CommunityCreatorType int

const (
	CommunityCreatorTypeOverride CommunityCreatorType = iota
	CommunityCreatorTypeProvider
)

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (c *CommunityType) UnmarshalGQL(v interface{}) error {
	n, ok := v.(string)
	if !ok {
		return fmt.Errorf("chain must be a string")
	}

	switch strings.ToLower(n) {
	case "contract":
		*c = CommunityTypeContract
	case "artblocks":
		*c = CommunityTypeArtBlocks
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface
func (c CommunityType) MarshalGQL(w io.Writer) {
	switch c {
	case CommunityTypeContract:
		w.Write([]byte(`"Contract"`))
	case CommunityTypeArtBlocks:
		w.Write([]byte(`"ArtBlocks"`))
	}
}

type CommunityKey struct {
	Type CommunityType
	Key1 string
	Key2 string
	Key3 string
	Key4 string
}

func (k CommunityKey) String() string {
	return fmt.Sprintf("%d:%s:%s:%s:%s", k.Type, k.Key1, k.Key2, k.Key3, k.Key4)
}

type ErrCommunityNotFound struct {
	ID  DBID
	Key CommunityKey
}

func (e ErrCommunityNotFound) Error() string {
	return fmt.Sprintf("Community not found for contractID %s, key %s", e.ID, e.Key)
}
