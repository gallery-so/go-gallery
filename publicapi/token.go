package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/throttle"
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
	throttler          *throttle.Locker
}

// ErrTokenRefreshFailed is a generic error that wraps all other OpenSea sync failures.
// Should be removed once we stop using OpenSea to sync NFTs.
type ErrTokenRefreshFailed struct {
	Message string
}

func (e ErrTokenRefreshFailed) Error() string {
	return e.Message
}
func (api TokenAPI) GetTokenById(ctx context.Context, tokenID persist.DBID) (*sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"tokenID": {tokenID, "required"},
	}); err != nil {
		return nil, err
	}

	token, err := api.loaders.TokenByTokenID.Load(tokenID)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (api TokenAPI) GetTokensByCollectionId(ctx context.Context, collectionID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByCollectionID.Load(collectionID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetNewTokensByFeedEventID returns new tokens added to a collection from an event.
// Since its possible for tokens to be deleted, the return size may not be the same size of
// the tokens added, so the caller should handle the matching of arguments to response if used in that context.
func (api TokenAPI) GetNewTokensByFeedEventID(ctx context.Context, eventID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"eventID": {eventID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.NewTokensByFeedEventID.Load(eventID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"walletID": {walletID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByWalletID.Load(walletID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByUserID(ctx context.Context, userID persist.DBID) ([]sqlc.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByUserID.Load(userID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) SyncTokens(ctx context.Context) error {
	// No validation to do
	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	if err := api.throttler.Lock(ctx, userID.String()); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, userID.String())

	err = api.multichainProvider.SyncTokens(ctx, userID)
	if err != nil {
		// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	api.loaders.ClearAllCaches()

	return nil
}

func (api TokenAPI) RefreshToken(ctx context.Context, tokenDBID persist.DBID) error {
	if err := validateFields(api.validator, validationMap{
		"tokenID": {tokenDBID, "required"},
	}); err != nil {
		return err
	}

	token, err := api.loaders.TokenByTokenID.Load(tokenDBID)
	if err != nil {
		return err
	}
	contract, err := api.loaders.ContractByContractId.Load(token.Contract)
	if err != nil {
		return err
	}

	err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(contract.Address, persist.TokenID(token.TokenID.String), persist.Chain(contract.Chain.Int32)))
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	api.loaders.ClearAllCaches()

	return nil
}

func (api TokenAPI) UpdateTokenInfo(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID, collectorsNote string) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"tokenID":        {tokenID, "required"},
		"collectorsNote": {collectorsNote, "token_note"},
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

	err = api.repos.TokenRepository.UpdateByID(ctx, tokenID, userID, update)
	if err != nil {
		return err
	}

	api.loaders.ClearAllCaches()

	// Send event
	dispatchEventToFeed(ctx, sqlc.Event{
		ActorID:        userID,
		Action:         persist.ActionCollectorsNoteAddedToToken,
		ResourceTypeID: persist.ResourceTypeToken,
		TokenID:        tokenID,
		SubjectID:      tokenID,
		Data: persist.EventData{
			TokenCollectionID:   collectionID,
			TokenCollectorsNote: collectorsNote,
		},
	})

	return nil
}
