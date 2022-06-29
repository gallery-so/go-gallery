package model

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

type GqlID string

var Cursor connCursor

func (r *CollectionToken) GetGqlIDField_TokenID() string {
	return r.TokenId.String()
}

func (r *CollectionToken) GetGqlIDField_CollectionID() string {
	return r.TokenId.String()
}

func (r *Community) GetGqlIDField_Chain() string {
	return fmt.Sprint(r.ContractAddress.Chain())
}

func (r *Community) GetGqlIDField_ContractAddress() string {
	return r.ContractAddress.Address().String()
}

type HelperCollectionTokenData struct {
	TokenId      persist.DBID
	CollectionId persist.DBID
}

type HelperTokenHolderData struct {
	UserId    persist.DBID
	WalletIds []persist.DBID
}

type HelperTokensAddedToCollectionFeedEventDataData struct {
	FeedEventId persist.DBID
}

type HelperFeedConnectionData struct {
	UserId  persist.DBID
	ByFirst bool
}

type ErrInvalidIDFormat struct {
	message string
}

func (e ErrInvalidIDFormat) Error() string {
	return fmt.Sprintf("invalid ID format: %s", e.message)
}

type ErrInvalidIDType struct {
	typeName string
}

func (e ErrInvalidIDType) Error() string {
	return fmt.Sprintf("no fetch method found for ID type '%s'", e.typeName)
}

var errBadCursorFormat = errors.New("bad cursor format")

type connCursor struct{}

func (connCursor) DBIDEncodeToCursor(id persist.DBID) string {
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func (connCursor) DecodeToDBID(s *string) (*persist.DBID, error) {
	if s == nil {
		return nil, nil
	}

	dec, err := base64.StdEncoding.DecodeString(string(*s))
	if err != nil {
		return nil, errBadCursorFormat
	}

	dbid := persist.DBID(dec)

	return &dbid, nil
}
