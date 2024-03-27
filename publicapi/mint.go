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
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type MintAPI struct {
	validator         *validator.Validate
	highlightProvider *highlight.Provider
	queries           *db.Queries
	taskClient        *task.Client
	throttler         *throttle.Locker
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

func (api *MintAPI) guardMinting(ctx context.Context, collectionID string, userID, walletID persist.DBID) (error, func()) {
	userLock := fmt.Sprintf("mint:platform:highlight:collection:%s:user:%s", collectionID, userID)
	err := api.throttler.Lock(ctx, userLock)
	if err != nil {
		return err, nil
	}

	walletLock := fmt.Sprintf("mint:platform:highlight:collection:%s:wallet:%s", collectionID, walletID)
	err = api.throttler.Lock(ctx, walletLock)
	if err != nil {
		return err, nil
	}

	return nil, func() {
		api.throttler.Unlock(ctx, fmt.Sprintf("mint:platform:highlight:collection:%s:user:%s", collectionID, userID))
		api.throttler.Unlock(ctx, fmt.Sprintf("mint:platform:highlight:collection:%s:wallet:%s", collectionID, walletID))
	}
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

	err, releaseLock := api.guardMinting(ctx, collectionID, userID, walletID)
	if util.ErrorIs[throttle.ErrThrottleLocked](err) {
		return "", ErrMintTxPending
	}
	if err != nil {
		return "", err
	}
	defer releaseLock()

	err = api.alreadyClaimedByUser(ctx, userID, collectionID)
	if err != nil {
		return "", err
	}

	recipient, err := api.validateClaimableForWallet(ctx, userID, walletID, collectionID)
	if err != nil {
		return "", err
	}

	// Mint one token from highlight
	claimID, status, collectionAddress, claimErr := api.highlightProvider.ClaimMint(ctx, collectionID, 1, recipient)

	// Stop early if a transaction isn't initiated
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

var mintedStatuses []string = util.MapWithoutError(
	[]highlight.ClaimStatus{
		highlight.ClaimStatusTxSucceeded,
		highlight.ClaimStatusMediaProcessed,
		highlight.ClaimStatusMediaFailed,
	},
	func(s highlight.ClaimStatus) string { return string(s) },
)

var pendingStatuses []string = util.MapWithoutError(
	[]highlight.ClaimStatus{
		highlight.ClaimStatusTxPending,
		highlight.ClaimStatusMediaProcessing,
		highlight.ClaimStatusFailedInternal, // Don't let the user retry on internal errors to be safe
	},
	func(s highlight.ClaimStatus) string { return string(s) },
)

func (api *MintAPI) alreadyClaimedByUser(ctx context.Context, userID persist.DBID, collectionID string) error {
	status, err := api.queries.HasMintedClaimsByUserID(ctx, db.HasMintedClaimsByUserIDParams{
		RecipientUserID:       userID,
		HighlightCollectionID: collectionID,
		MintedStatuses:        mintedStatuses,
		PendingStatuses:       pendingStatuses,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if status.HasMinted {
		return ErrMintAlreadyClaimed
	}
	if status.HasPending {
		return ErrMintTxPending
	}
	return nil
}

func (api *MintAPI) validateClaimableForWallet(ctx context.Context, userID, walletID persist.DBID, collectionID string) (persist.ChainAddress, error) {
	user, err := For(ctx).User.GetUserById(ctx, userID)
	if err != nil {
		return persist.ChainAddress{}, err
	}

	wallet, err := For(ctx).Wallet.GetWalletByID(ctx, walletID)
	if err != nil {
		return persist.ChainAddress{}, err
	}

	// Check that the user owns the wallet
	chainAddress := persist.NewChainAddress(wallet.Address, wallet.Chain)
	_, owns := util.FindFirst(user.Wallets, func(w persist.Wallet) bool { return w.ID == walletID })
	if !owns {
		return persist.ChainAddress{}, persist.ErrAddressNotOwnedByUser{ChainAddress: chainAddress, UserID: userID}
	}

	// Check if there's already a claim for this L1 address
	status, err := api.queries.HasMintedClaimsByWalletAddress(ctx, db.HasMintedClaimsByWalletAddressParams{
		RecipientL1Chain:      chainAddress.Chain().L1Chain(),
		RecipientAddress:      chainAddress.Address(),
		HighlightCollectionID: collectionID,
		MintedStatuses:        mintedStatuses,
		PendingStatuses:       pendingStatuses,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return chainAddress, nil
		}
		return persist.ChainAddress{}, err
	}
	if status.HasMinted {
		return persist.ChainAddress{}, ErrMintAlreadyClaimed
	}
	if status.HasPending {
		return persist.ChainAddress{}, ErrMintTxPending
	}
	return chainAddress, nil
}
