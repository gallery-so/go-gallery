package publicapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"

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
	return api.queries.GetHighlightMintClaimByID(ctx, id)
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

	recipient, err := api.validateMintClaimableForWallet(ctx, userID, walletID, collectionID)
	if err != nil {
		return "", err
	}

	// Mint one token from highlight
	claimID, status, collectionAddress, claimErr := api.highlightProvider.ClaimMint(ctx, collectionID, 1, recipient)
	// Stop early for errors where a transaction isn't initiated
	if errors.Is(claimErr, highlight.ErrHighlightChainNotSupported) ||
		util.ErrorIs[highlight.ErrHighlightCollectionMintUnavailable](claimErr) ||
		util.ErrorIs[highlight.ErrHighlightInternalError](claimErr) {
		return "", claimErr
	}

	// Save record of transaction
	storeParams := db.SaveHighlightMintClaimParams{
		ID:                    persist.GenerateID(),
		RecipientUserID:       userID,
		RecipientL1Chain:      recipient.Chain().L1Chain(),
		RecipientAddress:      recipient.Address(),
		RecipientWalletID:     walletID,
		HighlightCollectionID: collectionID,
		HighlightClaimID:      claimID,
		CollectionAddress:     collectionAddress.Address(),
		CollectionChain:       collectionAddress.Chain(),
		Status:                status,
	}
	if claimErr != nil {
		storeParams.ErrorMessage = util.ToNullString(claimErr.Error(), true)
	}
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

var ErrMintTxPending = fmt.Errorf("transaction is pending")
var ErrMintAlreadyClaimed = fmt.Errorf("already claimed mint for this collection")

func (api *MintAPI) validateMintClaimableForWallet(ctx context.Context, userID, walletID persist.DBID, collectionID string) (persist.ChainAddress, error) {
	err := api.claimableViaUser(ctx, userID, collectionID)
	if err != nil {
		return persist.ChainAddress{}, err
	}
	walletAddress, err := api.claimableViaWallet(ctx, userID, walletID, collectionID)
	return walletAddress, err
}

func hasPriorClaim(err error) (bool, error) {
	// nil              -> true, nil
	// pgx.ErrNoRows
	return !errors.Is(err, pgx.ErrNoRows), err
}

func (api *MintAPI) claimableViaUser(ctx context.Context, userID persist.DBID, collectionID string) error {
	claim, err := api.queries.GetHighlightCollectionClaimByUserID(ctx, db.GetHighlightCollectionClaimByUserIDParams{
		RecipientUserID:       userID,
		HighlightCollectionID: collectionID,
	})
	hasClaim, err := hasPriorClaim(err)
	if err != nil {
		return err
	}
	if !hasClaim {
		return nil
	}
	return claimableViaStatus(claim)
}

func (api *MintAPI) claimableViaWallet(ctx context.Context, userID, walletID persist.DBID, collectionID string) (persist.ChainAddress, error) {
	user, err := For(ctx).User.GetUserById(ctx, userID)
	if err != nil {
		return persist.ChainAddress{}, err
	}

	wallet, err := For(ctx).Wallet.GetWalletByID(ctx, walletID)
	if err != nil {
		return persist.ChainAddress{}, err
	}

	// Validate the user owns the wallet
	chainAddress := persist.NewChainAddress(wallet.Address, wallet.Chain)
	_, owns := util.FindFirst(user.Wallets, func(w persist.Wallet) bool { return w.ID == walletID })
	if !owns {
		return persist.ChainAddress{}, persist.ErrAddressNotOwnedByUser{ChainAddress: chainAddress, UserID: userID}
	}

	// Check if the there's already a status for the L1 address in the mint table
	claim, err := api.queries.GetHighlightCollectionClaimByWalletAddress(ctx, db.GetHighlightCollectionClaimByWalletAddressParams{
		RecipientL1Chain: chainAddress.Chain().L1Chain(),
		RecipientAddress: chainAddress.Address(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return persist.ChainAddress{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return chainAddress, nil
	}

	err = claimableViaStatus(claim)
	if err != nil {
		return persist.ChainAddress{}, err
	}

	return chainAddress, nil
}

func claimableViaStatus(claim db.HighlightMintClaim) error {
	switch claim.Status {
	case highlight.ClaimStatusTxFailed: // let the user retry if the tx failed
		return nil
	case
		highlight.ClaimStatusTxPending,       // mint in progress
		highlight.ClaimStatusTxSucceeded,     // minted
		highlight.ClaimStatusMediaProcessing: // minted
		return ErrMintTxPending
	case
		highlight.ClaimStatusMediaProcessed, // minted
		highlight.ClaimStatusMediaFailed:    // minted
		return ErrMintAlreadyClaimed
	case highlight.ClaimStatusFailedInternal: // investigate this error
		return fmt.Errorf("claimID=%s already failed with error: %s", claim.ID, claim.ErrorMessage.String)
	default:
		return fmt.Errorf("claimID=%s has an unexpected status=%s", claim.ID, claim.Status)
	}
}
