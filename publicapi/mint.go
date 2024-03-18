package publicapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/highlight"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type MintAPI struct {
	validator         *validator.Validate
	highlightProvider *highlight.Provider
	queries           *db.Queries
	taskClient        *task.Client
}

func (api *MintAPI) GetHighlightMintClaimByID(ctx context.Context, id persist.DBID) (db.HighlightMintClaim, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"id": validate.WithTag(id, "required"),
	}); err != nil {
		return db.HighlightMintClaim{}, err
	}
	return api.queries.GetHighlightMintClaim(ctx, id)
}

func (api *MintAPI) ClaimHighlightMint(ctx context.Context, collectionID string, walletID persist.DBID) (persist.DBID, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": validate.WithTag(collectionID, "required"),
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

	// TODO: Check if user already claimed, or should this be enforced by the contract

	// Validate the user owns the wallet
	chainAddress := persist.NewChainAddress(wallet.Address, wallet.Chain)
	_, owns := util.FindFirst(user.Wallets, func(w persist.Wallet) bool { return w.ID == walletID })
	if !owns {
		return "", persist.ErrAddressNotOwnedByUser{ChainAddress: chainAddress, UserID: userID}
	}

	// Mint one token from highlight
	claimID, status, contract, claimErr := api.highlightProvider.ClaimMint(ctx, collectionID, 1, chainAddress)
	// Catch errors not related to issues with the transaction itself
	if claimErr != nil && (errors.Is(claimErr, highlight.ErrHighlightChainNotSupported) ||
		util.ErrorIs[highlight.ErrHighlightCollectionMintUnavailable](claimErr) ||
		util.ErrorIs[highlight.ErrHighlightInternalError](claimErr)) {
		return "", claimErr
	}

	storeParams := db.SaveHighlightMintClaimParams{
		ID:                    persist.GenerateID(),
		UserID:                userID,
		Chain:                 contract.Chain(),
		ContractAddress:       contract.Address(),
		RecipientWalletID:     wallet.ID,
		HighlightCollectionID: collectionID,
		ClaimID:               claimID,
		Status:                status,
	}
	if claimErr != nil {
		storeParams.ErrorMessage = util.ToNullString(claimErr.Error(), true)
	}

	// Save record of transaction
	claimDBID, err := api.queries.SaveHighlightMintClaim(ctx, storeParams)
	if err != nil {
		err = fmt.Errorf("failed to save highlight mint claimID=%s: %s", claimID, err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", err
	}

	if claimErr != nil {
		return "", claimErr
	}

	// Create task to track transaction
	err = api.taskClient.CreateTaskForHighlightMintClaim(ctx, task.HighlightMintClaimMessage{ClaimID: claimDBID, Attempts: 0})
	if err != nil {
		err = fmt.Errorf("failed to save create highlight mint claim task for claimID=%s: %s", claimID, err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return "", err
	}

	return claimDBID, nil
}
