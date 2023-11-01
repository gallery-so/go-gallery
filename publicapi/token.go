package publicapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v4"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/event"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
)

type TokenAPI struct {
	repos              *postgres.Repositories
	queries            *db.Queries
	loaders            *dataloader.Loaders
	validator          *validator.Validate
	ethClient          *ethclient.Client
	multichainProvider *multichain.Provider
	throttler          *throttle.Locker
	manager            *tokenmanage.Manager
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID": validate.WithTag(tokenID, "required"),
	}); err != nil {
		return nil, err
	}

	token, err := api.loaders.GetTokenByIdBatch.Load(tokenID)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

// GetTokenByIdIgnoreDisplayable returns a token by ID, ignoring the displayable flag.
func (api TokenAPI) GetTokenByIdIgnoreDisplayable(ctx context.Context, tokenID persist.DBID) (*db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID": validate.WithTag(tokenID, "required"),
	}); err != nil {
		return nil, err
	}

	token, err := api.loaders.GetTokenByIdIgnoreDisplayableBatch.Load(tokenID)
	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (api TokenAPI) GetTokenByEnsDomain(ctx context.Context, userID persist.DBID, domain string) (db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"domain": validate.WithTag(domain, "required"),
	}); err != nil {
		return db.Token{}, err
	}

	tokenID, err := eth.DeriveTokenID(domain)
	if err != nil {
		return db.Token{}, err
	}

	return api.loaders.GetTokenByUserTokenIdentifiersBatch.Load(db.GetTokenByUserTokenIdentifiersBatchParams{
		OwnerID:         userID,
		TokenID:         persist.TokenID(tokenID),
		ContractAddress: eth.EnsAddress,
		Chain:           persist.ChainETH,
	})
}

func (api TokenAPI) GetTokensByCollectionId(ctx context.Context, collectionID persist.DBID, limit *int) ([]db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": validate.WithTag(collectionID, "required"),
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.GetTokensByCollectionIdBatch.Load(db.GetTokensByCollectionIdBatchParams{
		CollectionID: collectionID,
		Limit:        util.ToNullInt32(limit),
	})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByContractIdPaginate(ctx context.Context, contractID persist.DBID, before, after *string, first, last *int, onlyGalleryUsers bool) ([]db.Token, PageInfo, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractID": validate.WithTag(contractID, "required"),
	}); err != nil {
		return nil, PageInfo{}, err
	}

	if err := validatePaginationParams(api.validator, first, last); err != nil {
		return nil, PageInfo{}, err
	}

	queryFunc := func(params boolTimeIDPagingParams) ([]interface{}, error) {

		logger.For(ctx).Infof("GetTokensByContractIdPaginate: %+v", params)
		tokens, err := api.queries.GetTokensByContractIdPaginate(ctx, db.GetTokensByContractIdPaginateParams{
			ID:                 contractID,
			Limit:              params.Limit,
			GalleryUsersOnly:   onlyGalleryUsers,
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
		total, err := api.queries.CountTokensByContractId(ctx, db.CountTokensByContractIdParams{
			ID:               contractID,
			GalleryUsersOnly: onlyGalleryUsers,
		})
		return int(total), err
	}

	cursorFunc := func(i interface{}) (bool, time.Time, persist.DBID, error) {
		if token, ok := i.(db.Token); ok {
			owner, err := api.loaders.GetTokenOwnerByIDBatch.Load(token.ID)
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

func (api TokenAPI) GetTokensByIDs(ctx context.Context, tokenIDs []persist.DBID) ([]db.Token, error) {
	tokens, errs := api.loaders.GetTokenByIdBatch.LoadAll(tokenIDs)
	foundTokens := tokens[:0]
	for i, t := range tokens {
		if errs[i] == nil {
			foundTokens = append(foundTokens, t)
		} else if _, ok := errs[i].(persist.ErrTokenNotFoundByID); !ok {
			return []db.Token{}, errs[i]
		}
	}

	return foundTokens, nil
}

// GetNewTokensByFeedEventID returns new tokens added to a collection from an event.
// Since its possible for tokens to be deleted, the return size may not be the same size of
// the tokens added, so the caller should handle the matching of arguments to response if used in that context.
func (api TokenAPI) GetNewTokensByFeedEventID(ctx context.Context, eventID persist.DBID) ([]db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"eventID": validate.WithTag(eventID, "required"),
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.GetNewTokensByFeedEventIdBatch.Load(eventID)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) GetTokensByWalletID(ctx context.Context, walletID persist.DBID) ([]db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"walletID": validate.WithTag(walletID, "required"),
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.GetTokensByWalletIdsBatch.Load(persist.DBIDList{walletID})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetTokensByUserID returns all tokens owned by a user. ownershipFilter is optional and may be nil or empty,
// which will cause all tokens to be returned. If filter values are provided, only the tokens matching the
// filter will be returned.
func (api TokenAPI) GetTokensByUserID(ctx context.Context, userID persist.DBID, ownershipFilter []persist.TokenOwnershipType) ([]db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
	}); err != nil {
		return nil, err
	}

	params := db.GetTokensByUserIdBatchParams{
		OwnerUserID: userID,
	}

	if len(ownershipFilter) > 0 {
		params.IncludeHolder = util.Contains(ownershipFilter, persist.TokenOwnershipTypeHolder)
		params.IncludeCreator = util.Contains(ownershipFilter, persist.TokenOwnershipTypeCreator)
	} else {
		// If no filters are specified, include everything
		params.IncludeHolder = true
		params.IncludeCreator = true
	}

	return api.loaders.GetTokensByUserIdBatch.Load(params)
}

func (api TokenAPI) SyncTokensAdmin(ctx context.Context, chains []persist.Chain, userID persist.DBID) error {
	key := fmt.Sprintf("sync:owned:%s", userID.String())

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	defer api.throttler.Unlock(ctx, key)

	if err := api.multichainProvider.SyncTokensByUserID(ctx, userID, chains); err != nil {
		// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil
}

func (api TokenAPI) SyncTokens(ctx context.Context, chains []persist.Chain, incrementally bool) error {
	userID, err := getAuthenticatedUserID(ctx)

	if err != nil {
		return err
	}

	key := fmt.Sprintf("sync:owned:%s", userID.String())

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, key)

	if incrementally {
		err := api.multichainProvider.SyncTokensIncrementallyByUserID(ctx, userID, chains)
		if err != nil {
			return ErrTokenRefreshFailed{Message: err.Error()}
		}

	} else {
		err = api.multichainProvider.SyncTokensByUserID(ctx, userID, chains)
		if err != nil {
			// Wrap all OpenSea sync failures in a generic type that can be returned to the frontend as an expected error type
			return ErrTokenRefreshFailed{Message: err.Error()}
		}
	}

	return nil
}

func (api TokenAPI) SyncCreatedTokensAdmin(ctx context.Context, includeChains []persist.Chain, userID persist.DBID) error {
	key := fmt.Sprintf("sync:created:new-contracts:%s", userID.String())

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, key)

	return api.multichainProvider.SyncCreatedTokensForNewContracts(ctx, userID, includeChains)
}

func (api TokenAPI) SyncCreatedTokensForNewContracts(ctx context.Context, includeChains []persist.Chain) error {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("sync:created:new-contracts:%s", userID.String())

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, key)

	return api.multichainProvider.SyncCreatedTokensForNewContracts(ctx, userID, includeChains)
}

func (api TokenAPI) SyncCreatedTokensForExistingContract(ctx context.Context, contractID persist.DBID) error {
	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("sync:created:contract:%s:%s", userID.String(), contractID.String())

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, key)

	return api.multichainProvider.SyncCreatedTokensForExistingContract(ctx, userID, contractID)
}

func (api TokenAPI) SyncCreatedTokensForExistingContractAdmin(ctx context.Context, userID persist.DBID, chainAddress persist.ChainAddress) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID":  validate.WithTag(userID, "required"),
		"chain":   validate.WithTag(chainAddress.Chain(), "chain"),
		"address": validate.WithTag(chainAddress.Address(), "required"),
	}); err != nil {
		return err
	}

	contract, err := api.queries.GetContractByChainAddress(ctx, db.GetContractByChainAddressParams{
		Address: chainAddress.Address(),
		Chain:   chainAddress.Chain(),
	})
	if err != nil {
		return err
	}

	key := fmt.Sprintf("sync:created:contract:%s:%s", userID.String(), contract.ID)

	if err := api.throttler.Lock(ctx, key); err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}
	defer api.throttler.Unlock(ctx, key)

	return api.multichainProvider.SyncCreatedTokensForExistingContract(ctx, userID, contract.ID)
}

func (api TokenAPI) RefreshToken(ctx context.Context, tokenDBID persist.DBID) error {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID": validate.WithTag(tokenDBID, "required"),
	}); err != nil {
		return err
	}

	// XXX: TODO CHANGE THIS TO A LOADER
	td, err := api.queries.GetTokenDefinitionByTokenDbid(ctx, tokenDBID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persist.ErrTokenDefinitionNotFoundByTokenDBID{ID: tokenDBID}
		}
		return fmt.Errorf("failed to load token: %w", err)
	}

	tID := persist.NewTokenIdentifiers(td.ContractAddress, td.TokenID, td.Chain)

	err = api.multichainProvider.RefreshToken(ctx, tID)
	if err != nil {
		return ErrTokenRefreshFailed{Message: err.Error()}
	}

	return nil
}

func (api TokenAPI) RefreshTokensInCollection(ctx context.Context, ci persist.ContractIdentifiers) error {
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"contractIdentifiers": validate.WithTag(ci, "required"),
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"collectionID": validate.WithTag(collectionDBID, "required"),
	}); err != nil {
		return err
	}

	tokens, err := api.loaders.GetTokensByCollectionIdBatch.Load(db.GetTokensByCollectionIdBatchParams{
		CollectionID: collectionDBID,
	})
	if err != nil {
		return err
	}
	wp := workerpool.New(10)
	errChan := make(chan error)
	for _, token := range tokens {
		token := token
		wp.Submit(func() {
			td, err := api.queries.GetTokenDefinitionByTokenDbid(ctx, token.ID)
			if err != nil {
				errChan <- err
				return
			}

			err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(td.ContractAddress, td.TokenID, td.Chain))
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
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID":        validate.WithTag(tokenID, "required"),
		"collectorsNote": validate.WithTag(collectorsNote, "token_note"),
	}); err != nil {
		return err
	}

	// Sanitize
	collectorsNote = validate.SanitizationPolicy.Sanitize(collectorsNote)

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	err = api.queries.UpdateTokenCollectorsNoteByTokenDbidUserId(ctx, db.UpdateTokenCollectorsNoteByTokenDbidUserIdParams{
		ID:             tokenID,
		OwnerUserID:    userID,
		CollectorsNote: util.ToNullString(collectorsNote, true),
	})
	if err != nil {
		return err
	}

	galleryID, err := api.queries.GetGalleryIDByCollectionID(ctx, collectionID)
	if err != nil {
		return err
	}

	// Send event
	return event.Dispatch(ctx, db.Event{
		ActorID:        persist.DBIDToNullStr(userID),
		Action:         persist.ActionCollectorsNoteAddedToToken,
		ResourceTypeID: persist.ResourceTypeToken,
		TokenID:        tokenID,
		CollectionID:   collectionID,
		GalleryID:      galleryID,
		SubjectID:      tokenID,
		Data: persist.EventData{
			TokenCollectionID:   collectionID,
			TokenCollectorsNote: collectorsNote,
		},
	})
}

func (api TokenAPI) SetSpamPreference(ctx context.Context, tokens []persist.DBID, isSpam bool) error {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokens": validate.WithTag(tokens, "required,unique"),
	}); err != nil {
		return err
	}

	userID, err := getAuthenticatedUserID(ctx)
	if err != nil {
		return err
	}

	return api.queries.UpdateTokensAsUserMarkedSpam(ctx, db.UpdateTokensAsUserMarkedSpamParams{
		IsUserMarkedSpam: sql.NullBool{Bool: isSpam, Valid: true},
		OwnerUserID:      userID,
		TokenIds:         tokens,
	})
}

func (api TokenAPI) GetTokenDefinitionAndMediaByTokenDBID(ctx context.Context, tokenDBID persist.DBID) (db.TokenDefinition, db.TokenMedia, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenDBID": validate.WithTag(tokenDBID, "required"),
	}); err != nil {
		return db.TokenDefinition{}, db.TokenMedia{}, err
	}
	panic("not implemented")
}

func (api TokenAPI) GetTokenDefinitionAndMediaByTokenIdentifiers(ctx context.Context, tokenIdentifiers persist.TokenIdentifiers) (db.TokenDefinition, db.TokenMedia, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"address": validate.WithTag(tokenIdentifiers.ContractAddress, "required"),
		"tokenID": validate.WithTag(tokenIdentifiers.TokenID, "required"),
	}); err != nil {
		return db.TokenDefinition{}, db.TokenMedia{}, err
	}
	tokenDefAndMedia, err := api.queries.GetTokenDefinitionAndMediaByTokenIdentifiers(ctx, db.GetTokenDefinitionAndMediaByTokenIdentifiersParams{
		Chain:           tokenIdentifiers.Chain,
		ContractAddress: tokenIdentifiers.ContractAddress,
		TokenID:         tokenIdentifiers.TokenID,
	})
	return tokenDefAndMedia.TokenDefinition, tokenDefAndMedia.TokenMedia, err
}

func (api TokenAPI) GetMediaByTokenDefinitionID(ctx context.Context, id persist.DBID) (db.TokenMedia, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenDefinitionID": validate.WithTag(id, "required"),
	}); err != nil {
		return db.TokenMedia{}, err
	}
	return api.loaders.GetMediaByTokenDefinitionIDIgnoringStatusBatch.Load(id)
}

func (api TokenAPI) ViewToken(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (db.Event, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID":      validate.WithTag(tokenID, "required"),
		"collectionID": validate.WithTag(collectionID, "required"),
	}); err != nil {
		return db.Event{}, err
	}

	td, err := api.queries.GetTokenDefinitionByTokenDbid(ctx, tokenID)
	if err != nil {
		return db.Event{}, err
	}

	currCol, err := api.queries.GetCollectionById(ctx, collectionID)
	if err != nil {
		return db.Event{}, err
	}

	gc := util.MustGetGinContext(ctx)

	if auth.GetUserAuthedFromCtx(gc) {
		userID, err := getAuthenticatedUserID(ctx)
		if err != nil {
			return db.Event{}, err
		}
		eventPtr, err := api.repos.EventRepository.Add(ctx, db.Event{
			ActorID:        persist.DBIDToNullStr(userID),
			Action:         persist.ActionViewedToken,
			ResourceTypeID: persist.ResourceTypeToken,
			TokenID:        tokenID,
			CollectionID:   collectionID,
			GalleryID:      currCol.GalleryID,
			SubjectID:      tokenID,
			Data: persist.EventData{
				TokenContractID:   td.ContractID,
				TokenDefinitionID: td.ID,
			},
		})
		if err != nil {
			return db.Event{}, err
		}
		return *eventPtr, nil
	}
	return db.Event{}, nil
}

// GetProcessingStateByTokenDefinitionID returns true if a token is queued for processing, or is currently being processed.
func (api TokenAPI) GetProcessingStateByTokenDefinitionID(ctx context.Context, id persist.DBID) (bool, error) {
	return api.manager.Processing(ctx, id), nil
}

func (api TokenAPI) GetTokenDefinitionByTokenDBID(ctx context.Context, id persist.DBID) (db.TokenDefinition, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenDBID": validate.WithTag(id, "required"),
	}); err != nil {
		return db.TokenDefinition{}, err
	}
	return api.loaders.GetTokenDefinitionByTokenDbidBatch.Load(id)
}

func (api TokenAPI) GetTokenDefinitionByID(ctx context.Context, id persist.DBID) (db.TokenDefinition, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenDefinitionID": validate.WithTag(id, "required"),
	}); err != nil {
		return db.TokenDefinition{}, err
	}
	return api.loaders.GetTokenDefinitionByIdBatch.Load(id)
}
