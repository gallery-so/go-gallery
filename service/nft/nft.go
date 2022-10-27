package nft

import (
	"github.com/mikeydub/go-gallery/service/persist"
)

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
