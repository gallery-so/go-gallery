package publicapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gammazero/workerpool"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
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
	queries            *db.Queries
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

func (api TokenAPI) GetTokenById(ctx context.Context, tokenID persist.DBID) (*db.Token, error) {
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

func (api TokenAPI) GetTokensByCollectionId(ctx context.Context, collectionID persist.DBID) ([]db.Token, error) {
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

func (api TokenAPI) GetTokensByContractId(ctx context.Context, contractID persist.DBID) ([]db.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByContractID.Load(contractID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByContractIdPaginate(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int) ([]db.Token, PageInfo, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"contractID": {contractID, "required"},
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {

		logger.For(ctx).Infof("GetTokensByContractIdPaginate: %+v", params)
		tokens, err := api.queries.GetTokensByContractIdPaginate(ctx, db.GetTokensByContractIdPaginateParams{
			Contract:           contractID,
			Limit:              params.Limit,
			CurBeforeUniversal: params.CursorBeforeBool,
			CurAfterUniversal:  params.CursorAfterBool,
			CurBeforeTime:      params.CursorBeforeTime,
			CurBeforeID:        params.CursorBeforeID,
			CurAfterTime:       params.CursorAfterTime,
			CurAfterID:         params.CursorAfterID,
			PagingForward:      params.PagingForward,
		})
		if err != nil {
			return nil, err
		}

		results := make([]interface{}, len(tokens))
		for i, token := range tokens {
			results[i] = token
		}

		return results, nil
	}

	countFunc := func() (int, error) {
		total, err := api.queries.CountTokensByContractId(ctx, contractID)
		return int(total), err
	}

	cursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {
		if token, ok := i.(db.Token); ok {
			owner, err := api.loaders.OwnerByTokenID.Load(token.ID)
			if err != nil {
				return false, time.Time{}, "", err
			}
			return owner.Universal, token.CreatedAt, token.ID, nil
		}
		return false, time.Time{}, "", fmt.Errorf("interface{} is not a token")
	}

	paginator := boolTimeIDPaginator{
		QueryFunc:  queryFunc,
		CursorFunc: cursorFunc,
		CountFunc:  countFunc,
	}

	results, pageInfo, err := paginator.paginate(before, after, first, last)

	if err != nil {
		return nil, PageInfo{}, err
	}

	tokens := make([]db.Token, len(results))
	for i, result := range results {
		if token, ok := result.(db.Token); ok {
			tokens[i] = token
		} else {
			return nil, PageInfo{}, fmt.Errorf("interface{} is not a token: %T", token)
		}
	}

	return tokens, pageInfo, nil
}

func (api TokenAPI) GetTokensByTokenIDs(ctx context.Context, tokenIDs []persist.DBID) ([]db.Token, []error) {
	return api.loaders.TokenByTokenID.LoadAll(tokenIDs)
}

// GetNewTokensByFeedEventID returns new tokens added to a collection from an event.
// Since its possible for tokens to be deleted, the return size may not be the same size of
// the tokens added, so the caller should handle the matching of arguments to response if used in that context.
func (api TokenAPI) GetNewTokensByFeedEventID(ctx context.Context, eventID persist.DBID) ([]db.Token, error) {
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

func (api TokenAPI) GetTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]db.Token, error) {
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

func (api TokenAPI) GetTokensByUserID(ctx context.Context, userID persist.DBID) ([]db.Token, error) {
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

func (api TokenAPI) GetTokensByUserIDAndContractID(ctx context.Context, userID, contractID persist.DBID) ([]db.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID":     {userID, "required"},
		"contractID": {contractID, "required"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByUserIDAndContractID.Load(persist.DBIDTuple{userID, contractID})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByUserIDAndChain(ctx context.Context, userID persist.DBID, chain persist.Chain) ([]db.Token, error) {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"userID": {userID, "required"},
		"chain":  {chain, "chain"},
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.TokensByUserIDAndChain.Load(dataloader.IDAndChain{ID: userID, Chain: chain})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// Short-term workaround to create admin-only functions. Should be removed when the
// admin UI is back up and running.
func isAdminUser(userID persist.DBID) bool {
	switch userID {
	case "a3ff91986625382ff776067619200efe":
		return true
	case "85dd971e87c9574a962af22e23e52d95":
		return true
	case "872b4e915dd0e2006a368b32fb6b685a":
		return true
	case "23LydFAYGJY03L7ZMVKIsfDzM9A":
		return true
	case "213enLGfyDLSd2ZX8TLMbf5qUPQ":
		return true
	case "217M1MtDpVQ0sZLhnH91m1AAGdq":
		return true
	default:
		return false
	}
}

func (api TokenAPI) SyncTokens(ctx context.Context, chains []persist.Chain, asUserID *persist.DBID) error {
	// No validation to do
	authedUserID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	userID := authedUserID
	if asUserID != nil && isAdminUser(authedUserID) {
		userID = *asUserID
	}

	if err := api.throttler.Lock(ctx, userID.String()); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, userID.String())

	err = api.multichainProvider.SyncTokens(ctx, userID, chains)
	if err != nil {
		// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

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
		return fmt.Errorf("failed to load token: %w", err)
	}
	contract, err := api.loaders.ContractByContractID.Load(token.Contract)
	if err != nil {
		return fmt.Errorf("failed to load contract for token: %w", err)
	}

	addresses := []persist.Address{}
	for _, walletID := range token.OwnedByWallets {
		wa, err := api.loaders.WalletByWalletID.Load(walletID)
		if err != nil {
			if strings.Contains(err.Error(), "no rows in result set") {
				continue
			}
			return fmt.Errorf("failed to load wallet %s: %w", walletID, err)
		}
		addresses = append(addresses, wa.Address)
	}

	err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(contract.Address, persist.TokenID(token.TokenID.String), persist.Chain(contract.Chain.Int32)), addresses)
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil
}

func (api TokenAPI) RefreshTokensInCollection(ctx context.Context, ci persist.ContractIdentifiers) error {
	if err := validateFields(api.validator, validationMap{
		"contractIdentifiers": {ci, "required"},
	}); err != nil {
		return err
	}

	err := api.multichainProvider.RefreshTokensForContract(ctx, ci)
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil
}

func (api TokenAPI) RefreshCollection(ctx context.Context, collectionDBID persist.DBID) error {
	if err := validateFields(api.validator, validationMap{
		"collectionID": {collectionDBID, "required"},
	}); err != nil {
		return err
	}

	tokens, err := api.loaders.TokensByCollectionID.Load(collectionDBID)
	if err != nil {
		return err
	}
	wp := workerpool.New(10)
	errChan := make(chan error)
	for _, token := range tokens {
		token := token
		wp.Submit(func() {
			contract, err := api.loaders.ContractByContractID.Load(token.Contract)
			if err != nil {
				errChan <- err
				return
			}

			addresses := []persist.Address{}
			for _, walletID := range token.OwnedByWallets {
				wa, err := api.loaders.WalletByWalletID.Load(walletID)
				if err != nil {
					errChan <- err
					return
				}
				addresses = append(addresses, wa.Address)
			}

			err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(contract.Address, persist.TokenID(token.TokenID.String), persist.Chain(contract.Chain.Int32)), addresses)
			if err != nil {
				errChan <- ErrTokenRefreshFailed{Message: err.Error()}
				return
			}
		})
	}
	go func() {
		wp.StopWait()
		errChan <- nil
	}()
	if err := <-errChan; err != nil {
		return err
	}

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

	// Send event
	_, err = dispatchEvent(ctx, db.Event{
		ActorID:        userID,
		Action:         persist.ActionCollectorsNoteAddedToToken,
		ResourceTypeID: persist.ResourceTypeToken,
		TokenID:        tokenID,
		SubjectID:      tokenID,
		Data: persist.EventData{
			TokenCollectionID:   collectionID,
			TokenCollectorsNote: collectorsNote,
		},
	}, api.validator, nil)

	return err
}

func (api TokenAPI) SetSpamPreference(ctx context.Context, tokens []persist.DBID, isSpam bool) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"tokens": {tokens, "required,unique"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	err = api.repos.TokenRepository.TokensAreOwnedByUser(ctx, userID, tokens)
	if err != nil {
		return err
	}

	return api.repos.TokenRepository.FlagTokensAsUserMarkedSpam(ctx, userID, tokens, isSpam)
}

func (api TokenAPI) DeepRefreshByChain(ctx context.Context, chain persist.Chain) error {
	// Validate
	if err := validateFields(api.validator, validationMap{
		"chain": {chain, "chain"},
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUser(ctx)
	if err != nil {
		return err
	}

	return api.multichainProvider.DeepRefreshByChain(ctx, userID, chain)
}
