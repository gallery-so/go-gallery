package publicapi

import (
	"context"

	"github.com/go-playground/validator/v10"

	"github.com/mikeydub/go-gallery/service/multichain/highlight"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type MintAPI struct {
	validator         *validator.Validate
	highlightProvider *highlight.Provider
}

func (api *MintAPI) HighlightClaimMint(ctx context.Context, collectionID string, qty int, walletID persist.DBID) (string, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": validate.WithTag(collectionID, "required"),
		"quantity":     validate.WithTag(qty, "required,gt=0"),
		"walletID":     validate.WithTag(walletID, "required"),
	}); err != nil {
		return "", err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return "", err
	}

	user, err := For(ctx).User.GetUserById(ctx, userID)
	if err != nil {
		return "", err
	}

	wallet, err := For(ctx).Wallet.GetWalletByID(ctx, walletID)
	if err != nil {
		return "", err
	}

	chainAddress := persist.NewChainAddress(wallet.Address, wallet.Chain)
	_, owns := util.FindFirst(user.Wallets, func(w persist.Wallet) bool { return w.ID == walletID })
	if !owns {
		return "", persist.ErrAddressNotOwnedByUser{ChainAddress: chainAddress, UserID: userID}
	}

	claimID, err := api.highlightProvider.ClaimMint(ctx, collectionID, qty, chainAddress)
	if err == nil {
		// TODO: start an async job to process the token for the user...
		// 1) send a message to tokenprocessing
		// 2) verify that the token exists
		// 3) retry up to a limit
		// 4) add that token to the user
		// 5) run the pipeline for the token
		// 6) ensure the pipeline retries if no metadata for highlight
	}

	return claimID, err
}
