package model

import (
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

type GqlID string

func (r *CollectionToken) GetGqlIDField_TokenID() string {
	return r.HelperCollectionTokenData.TokenId.String()
}

func (r *CollectionToken) GetGqlIDField_CollectionID() string {
	return r.HelperCollectionTokenData.CollectionId.String()
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

type HelperCommunityData struct {
	ForceRefresh *bool
}

type HelperTokensAddedToCollectionFeedEventDataData struct {
	FeedEventId persist.DBID
}

type HelperCollectionCreatedFeedEventDataData struct {
	FeedEventId persist.DBID
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
