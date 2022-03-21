package model

import "github.com/mikeydub/go-gallery/service/persist"

type GqlID string

func (r *GalleryNft) GetGqlIDField_NftID() string {
	return r.NftId.String()
}

func (r *GalleryNft) GetGqlIDField_CollectionID() string {
	return r.CollectionId.String()
}

type HelperGalleryNftData struct {
	NftId        persist.DBID
	CollectionId persist.DBID
}
