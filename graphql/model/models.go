package model

import (
	"fmt"
	"github.com/mikeydub/go-gallery/service/persist"
)

type GqlID string

func (r *CollectionNft) GetGqlIDField_NftID() string {
	return r.NftId.String()
}

func (r *CollectionNft) GetGqlIDField_CollectionID() string {
	return r.CollectionId.String()
}

type HelperCollectionNftData struct {
	NftId        persist.DBID
	CollectionId persist.DBID
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
