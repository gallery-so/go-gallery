package publicapi

import (
	"context"

	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/service/multichain"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/spf13/viper"

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

func (api TokenAPI) RefreshTokens(ctx context.Context) error {
	// No validation to do
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
	go func(ctx context.Context) {
		if hub := sentryutil.SentryHubFromContext(ctx); hub != nil {
			sentryutil.SetEventContext(hub.Scope(), userID, tokenID, persist.ActionCollectorsNoteAddedToToken)
		}

		err := event.DispatchEventToFeed(ctx, sqlc.Event{
			ActorID:        userID,
			Action:         persist.ActionCollectorsNoteAddedToToken,
			ResourceTypeID: persist.ResourceTypeToken,
			TokenID:        tokenID,
			SubjectID:      tokenID,
			Data: persist.EventData{
				TokenCollectionID:   collectionID,
				TokenCollectorsNote: collectorsNote,
			},
			FeedWindowSize: viper.GetInt("GCLOUD_FEED_BUFFER_SECS"),
		})

		if err != nil {
			sentryutil.ReportError(ctx, err)
		}
	}(sentryutil.NewSentryHubContext(ctx))

	return nil
}
