package nft

import (
	"context"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/service/opensea"
	"strings"

	"github.com/mikeydub/go-gallery/service/persist"
)

var errInvalidPreviewsInput = errors.New("user_id or username required for previews")

// GetPreviewsForUserInput is the input for receiving at most 3 image previews for the first NFTs displayed in a user's gallery
type GetPreviewsForUserInput struct {
	UserID   persist.DBID `form:"user_id"`
	Username string       `form:"username"`
}

// GetPreviewsForUser returns a slice of 3 preview URLs from a user's collections
func GetPreviewsForUser(pCtx context.Context, galleryRepo persist.GalleryRepository, userRepo persist.UserRepository, u GetPreviewsForUserInput) ([]persist.NullString, error) {
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
func GetPreviewsForUserToken(pCtx context.Context, galleryRepo persist.GalleryTokenRepository, userRepo persist.UserRepository, u GetPreviewsForUserInput) ([]persist.NullString, error) {
	var galleries []persist.GalleryToken
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

func RefreshOpenseaNFTs(ctx context.Context, userID persist.DBID, walletAddresses string, nftRepo persist.NFTRepository, userRepo persist.UserRepository) error {

	addresses := []persist.Address{}
	if walletAddresses != "" {
		addresses = []persist.Address{persist.Address(walletAddresses)}
		if strings.Contains(walletAddresses, ",") {
			addressesStrings := strings.Split(walletAddresses, ",")
			for _, address := range addressesStrings {
				addresses = append(addresses, persist.Address(address))
			}
		}
		ownsWallet, err := DoesUserOwnWallets(ctx, userID, addresses, userRepo)
		if err != nil {
			return err
		}

		if !ownsWallet {
			return ErrDoesNotOwnWallets{ID: userID, Addresses: addresses}
		}
	}

	return nil
}

func GetOpenseaNFTs(ctx context.Context, userID persist.DBID, walletAddresses string, nftRepo persist.NFTRepository, userRepo persist.UserRepository,
	collRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository, backupRepo persist.BackupRepository) error {

	var addresses []persist.Address
	if walletAddresses != "" {
		addresses = []persist.Address{persist.Address(walletAddresses)}
		if strings.Contains(walletAddresses, ",") {
			addressesStrings := strings.Split(walletAddresses, ",")
			for _, address := range addressesStrings {
				addresses = append(addresses, persist.Address(address))
			}
		}
		ownsWallet, err := DoesUserOwnWallets(ctx, userID, addresses, userRepo)
		if err != nil {
			return err
		}

		if !ownsWallet {
			return ErrDoesNotOwnWallets{ID: userID, Addresses: addresses}
		}
	}

	err := opensea.UpdateAssetsForAcc(ctx, userID, addresses, nftRepo, userRepo, collRepo, galleryRepo, backupRepo)
	if err != nil {
		return err
	}

	return nil
}

func DoesUserOwnWallets(pCtx context.Context, userID persist.DBID, walletAddresses []persist.Address, userRepo persist.UserRepository) (bool, error) {
	user, err := userRepo.GetByID(pCtx, userID)
	if err != nil {
		return false, err
	}
	for _, walletAddress := range walletAddresses {
		if !ContainsWalletAddresses(user.Addresses, walletAddress) {
			return false, nil
		}
	}
	return true, nil
}

func ContainsWalletAddresses(a []persist.Address, b persist.Address) bool {
	for _, v := range a {
		if v == b {
			return true
		}
	}

	return false
}

type ErrDoesNotOwnWallets struct {
	ID        persist.DBID
	Addresses []persist.Address
}

func (e ErrDoesNotOwnWallets) Error() string {
	return fmt.Sprintf("user with ID %s does not own all wallets: %+v", e.ID, e.Addresses)
}
