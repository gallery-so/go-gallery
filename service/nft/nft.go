package nft

import (
	"context"
	"errors"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"github.com/mikeydub/go-gallery/service/persist"
)

var errInvalidPreviewsInput = errors.New("user_id or username required for previews")

// GetPreviewsForUserInput is the input for receiving at most 3 image previews for the first NFTs displayed in a user's gallery
type GetPreviewsForUserInput struct {
	UserID   persist.DBID `form:"user_id"`
	Username string       `form:"username"`
}

// GetPreviewsForUser returns a slice of 3 preview URLs from a user's collections
func GetPreviewsForUser(pCtx context.Context, galleryRepo postgres.GalleryRepository, userRepo postgres.UserRepository, u GetPreviewsForUserInput) ([]persist.NullString, error) {
	var galleries []persist.Gallery
	var err error
	if u.UserID != "" {
		galleries, err = galleryRepo.GetByUserID(pCtx, u.UserID)
	} else if u.Username != "" {
		user, err := userRepo.GetByUsername(pCtx, u.Username)
		if err != nil {
			return nil, err
		}
		galleries, err = galleryRepo.GetByUserID(pCtx, user.ID)
	} else {
		return nil, errInvalidPreviewsInput
	}
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
func GetPreviewsForUserToken(pCtx context.Context, galleryRepo postgres.GalleryRepository, userRepo postgres.UserRepository, u GetPreviewsForUserInput) ([]persist.NullString, error) {
	var galleries []persist.Gallery
	var err error
	if u.UserID != "" {
		galleries, err = galleryRepo.GetByUserID(pCtx, u.UserID)
	} else if u.Username != "" {
		user, err := userRepo.GetByUsername(pCtx, u.Username)
		if err != nil {
			return nil, err
		}
		galleries, err = galleryRepo.GetByUserID(pCtx, user.ID)
	} else {
		return nil, errInvalidPreviewsInput
	}
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

// GetPreviewsFromCollectionsToken returns a slice of 3 preview URLs from a slice of CollectionTokens
func GetPreviewsFromCollectionsToken(pColls []persist.Collection) []persist.NullString {
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

// TODO this should be in multichain
// func RefreshOpenseaNFTs(ctx context.Context, userID persist.DBID, walletAddress string, nftRepo persist.NFTRepository, userRepo postgres.UserRepository) error {

// 	addresses := []persist.Wallet{}
// 	if walletAddress != "" {
// 		addresses = []persist.Wallet{}
// 		addressesStrings := strings.Split(walletAddress, ",")
// 		for _, address := range addressesStrings {
// 			addresses = append(addresses, persist.Wallet{Address: persist.Address(address), Chain: persist.ChainETH})
// 		}
// 		ownsWallet, err := user.DoesUserOwnWallets(ctx, userID, addresses, userRepo)
// 		if err != nil {
// 			return err
// 		}

// 		if !ownsWallet {
// 			return user.ErrDoesNotOwnWallets{ID: userID, Addresses: addresses}
// 		}
// 	}

// 	return nil
// }
