package nft

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// GetPreviewsForUser returns a slice of 3 preview URLs from a user's collections
func GetPreviewsForUser(pCtx context.Context, galleryRepo persist.GalleryRepository, u persist.DBID) ([]persist.NullString, error) {
	galleries, err := galleryRepo.GetByUserID(pCtx, u)
	if err != nil {
		return nil, err
	}
	result := make([]persist.NullString, 0, 3)

	for _, g := range galleries {
		previews := GetPreviewsFromCollections(g.Collections)
		result = append(result, previews...)
		if len(result) > 2 {
			break
		}
	}
	if len(result) > 3 {
		return result[:3], nil
	}
	return result, nil
}

// GetPreviewsForUserToken returns a slice of 3 preview URLs from a user's collections
func GetPreviewsForUserToken(pCtx context.Context, galleryRepo persist.GalleryTokenRepository, u persist.DBID) ([]persist.NullString, error) {
	galleries, err := galleryRepo.GetByUserID(pCtx, u)
	if err != nil {
		return nil, err
	}
	result := make([]persist.NullString, 0, 3)

	for _, g := range galleries {
		previews := GetPreviewsFromCollectionsToken(g.Collections)
		result = append(result, previews...)
		if len(result) > 2 {
			break
		}
	}
	if len(result) > 3 {
		return result[:3], nil
	}
	return result, nil
}

// GetPreviewsFromCollections returns a slice of 3 preview URLs from a slice of CollectionTokens
func GetPreviewsFromCollections(pColls []persist.Collection) []persist.NullString {
	result := make([]persist.NullString, 0, 3)

outer:
	for _, c := range pColls {
		for _, n := range c.NFTs {
			if n.ImageThumbnailURL != "" {
				result = append(result, n.ImageThumbnailURL)
			}
			if len(result) > 2 {
				break outer
			}
		}
		if len(result) > 2 {
			break outer
		}
	}
	return result

}

// GetPreviewsFromCollectionsToken returns a slice of 3 preview URLs from a slice of CollectionTokens
func GetPreviewsFromCollectionsToken(pColls []persist.CollectionToken) []persist.NullString {
	result := make([]persist.NullString, 0, 3)

outer:
	for _, c := range pColls {
		for _, n := range c.NFTs {
			if n.Media.ThumbnailURL != "" {
				result = append(result, n.Media.ThumbnailURL)
			}
			if len(result) > 2 {
				break outer
			}
		}
		if len(result) > 2 {
			break outer
		}
	}
	return result

}
