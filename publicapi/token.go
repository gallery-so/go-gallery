package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/event"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

type TokenAPI struct {
	repos              *persist.Repositories
	queries            *sqlc.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
}

// ErrTokenRefreshFailed is a generic error that wraps all other OpenSea sync failures.
// Should be removed once we stop using OpenSea to sync NFTs.
type ErrTokenRefreshFailed struct {
	Message string
}

func (e ErrTokenRefreshFailed) Error() string {
	return e.Message
}
func (api TokenAPI) GetNftById(ctx context.Context, nftID persist.DBID) (*sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"nftID": {nftID, "required"},
	}); err != nil {
		return nil, err
	}

	nft, err := api.loaders.TokenByID.Load(nftID)
	if err != nil {
		return nil, err
	}

	return &nft, nil
}

func (api TokenAPI) GetNftsByCollectionId(ctx context.Context, collectionID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.TokensByCollectionID.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

func (api TokenAPI) GetNftsByUserID(ctx context.Context, userID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"ownerUserID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	nfts, err := api.loaders.TokenByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return nfts, nil
}

func (api TokenAPI) GetTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"walletID": {walletID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByWalletId.Load(walletID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) RefreshTokens(ctx context.Context, addresses []*persist.Address) error {
	// No validation to do here -- addresses is an optional comma-separated list of addresses

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = api.multichainProvider.UpdateTokensForUser(ctx, userID)
	if err != nil {
		// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	api.loaders.ClearAllCaches()

	return nil
}

func (api TokenAPI) UpdateNftInfo(ctx context.Context, nftID persist.DBID, collectionID persist.DBID, collectorsNote string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"nftID":          {nftID, "required"},
		"collectorsNote": {collectorsNote, "nft_note"},
	}); err != nil {
		return err
	}

	// Sanitize
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	update := persist.TokenUpdateInfoInput{
		CollectorsNote: persist.NullString(collectorsNote),
	}

	err = api.repos.TokenRepository.UpdateByID(ctx, nftID, userID, update)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	// Send event
	nftData := persist.NftEvent{CollectionID: collectionID, CollectorsNote: persist.NullString(collectorsNote)}
	dispatchNftEvent(ctx, persist.NftCollectorsNoteAddedEvent, userID, nftID, nftData)

	return nil
}

func dispatchTokenEvent(ctx context.Context, eventCode persist.EventCode, userID persist.DBID, nftID persist.DBID, nftData persist.NftEvent) {
	gc := util.GinContextFromContext(ctx)
	nftHandlers := event.For(gc).Nft
	evt := persist.NftEventRecord{
		UserID: userID,
		NftID:  nftID,
		Code:   eventCode,
		Data:   nftData,
	}

	nftHandlers.Dispatch(ctx, evt)
}

func dispatchNftEvent(ctx context.Context, eventCode persist.EventCode, userID persist.DBID, nftID persist.DBID, nftData persist.NftEvent) {
	gc := util.GinContextFromContext(ctx)
	nftHandlers := event.For(gc).Nft
	evt := persist.NftEventRecord{
		UserID: userID,
		NftID:  nftID,
		Code:   eventCode,
		Data:   nftData,
	}

	nftHandlers.Dispatch(ctx, evt)
}
