package publicapi

import (
	"context"
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

	results, err := api.loaders.GetTokensByUserIdBatch.Load(params)
	if err != nil {
		return nil, err
	}

	tokens := util.MapWithoutError(results, func(r db.GetTokensByUserIdBatchRow) db.Token {
		return r.Token
	})

	return tokens, nil
}

func (api TokenAPI) GetTokensByUserIDAndChain(ctx context.Context, userID persist.DBID, chain persist.Chain) ([]db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"userID": validate.WithTag(userID, "required"),
		"chain":  validate.WithTag(chain, "chain"),
	}); err != nil {
		return nil, err
	}

	tokens, err := api.loaders.GetTokensByUserIdAndChainBatch.Load(db.GetTokensByUserIdAndChainBatchParams{
		OwnerUserID: userID,
		Chain:       chain,
	})
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (api TokenAPI) SyncTokensAdmin(ctx context.Context, chains []persist.Chain, userID persist.DBID) error {
	chains, closing, err := syncableChains(ctx, userID, chains, api.throttler)
	if err != nil {
		return err
	}
	defer closing()

	if len(chains) == 0 {
		return nil
	}

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

	chains, closing, err := syncableChains(ctx, userID, chains, api.throttler)
	if err != nil {
		return err
	}
	defer closing()

	if len(chains) == 0 {
		return nil
	}

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

	contract, err := api.repos.ContractRepository.GetByAddress(ctx, chainAddress.Address(), chainAddress.Chain())
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

	token, err := api.loaders.GetTokenByIdBatch.Load(tokenDBID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	contract, err := api.loaders.GetContractsByIDs.Load(token.Contract.String())
	if err != nil {
		return fmt.Errorf("failed to load contract for token: %w", err)
	}

	err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(contract.Address, token.TokenID, contract.Chain))
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
			contract, err := api.loaders.GetContractsByIDs.Load(token.Contract.String())
			if err != nil {
				errChan <- err
				return
			}

			err = api.multichainProvider.RefreshToken(ctx, persist.NewTokenIdentifiers(contract.Address, token.TokenID, contract.Chain))
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

	update := persist.TokenUpdateInfoInput{
		CollectorsNote: persist.NullString(collectorsNote),
	}

	err = api.repos.TokenRepository.UpdateByID(ctx, tokenID, userID, update)
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

	err = api.repos.TokenRepository.TokensAreOwnedByUser(ctx, userID, tokens)
	if err != nil {
		return err
	}

	return api.repos.TokenRepository.FlagTokensAsUserMarkedSpam(ctx, userID, tokens, isSpam)
}

func (api TokenAPI) MediaByMediaID(ctx context.Context, mediaID persist.DBID) (db.TokenMedia, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"mediaID": validate.WithTag(mediaID, "required"),
	}); err != nil {
		return db.TokenMedia{}, err
	}

	return api.loaders.GetMediaByMediaIDIgnoringStatus.Load(mediaID)
}

// MediaByTokenIdentifiers returns media for a token and optionally returns a token instance with fallback media matching the identifiers if any exists.
func (api TokenAPI) MediaByTokenIdentifiers(ctx context.Context, tokenIdentifiers persist.TokenIdentifiers) (db.TokenMedia, db.Token, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"address": validate.WithTag(tokenIdentifiers.ContractAddress, "required"),
		"tokenID": validate.WithTag(tokenIdentifiers.TokenID, "required"),
	}); err != nil {
		return db.TokenMedia{}, db.Token{}, err
	}

	// Check if the user is logged in, and if so sort results by prioritizing results specific to the user first
	userID, _ := getAuthenticatedUserID(ctx)

	// This query only returns a row if there is matching media, and it may not have a token instance even if it did return a row
	media, err := api.queries.GetMediaByUserTokenIdentifiers(ctx, db.GetMediaByUserTokenIdentifiersParams{
		UserID:  userID,
		Chain:   tokenIdentifiers.Chain,
		Address: tokenIdentifiers.ContractAddress,
		TokenID: tokenIdentifiers.TokenID,
	})

	// Got media and a token instance
	if err == nil && media.TokenInstanceID != "" {
		token, err := api.GetTokenById(ctx, media.TokenInstanceID)
		if err != nil || token == nil {
			return media.TokenMedia, db.Token{}, err
		}
		return media.TokenMedia, *token, err
	}

	// Unexpected error
	if err != nil && err != pgx.ErrNoRows {
		return db.TokenMedia{}, db.Token{}, err
	}

	// Try to find a suitable instance with fallback media
	token, err := api.queries.GetFallbackTokenByUserTokenIdentifiers(ctx, db.GetFallbackTokenByUserTokenIdentifiersParams{
		UserID:  userID,
		Chain:   tokenIdentifiers.Chain,
		Address: tokenIdentifiers.ContractAddress,
		TokenID: tokenIdentifiers.TokenID,
	})

	// Unexpected error
	if err != nil && err != pgx.ErrNoRows {
		return media.TokenMedia, db.Token{}, err
	}

	return media.TokenMedia, token, nil
}

func (api TokenAPI) ViewToken(ctx context.Context, tokenID persist.DBID, collectionID persist.DBID) (db.Event, error) {
	// Validate
	if err := validate.ValidateFields(api.validator, validate.ValidationMap{
		"tokenID":      validate.WithTag(tokenID, "required"),
		"collectionID": validate.WithTag(collectionID, "required"),
	}); err != nil {
		return db.Event{}, err
	}

	token, err := api.loaders.GetTokenByIdBatch.Load(tokenID)
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
				TokenContractID: token.Contract,
			},
		})
		if err != nil {
			return db.Event{}, err
		}
		return *eventPtr, nil
	}
	return db.Event{}, nil
}

// GetProcessingState returns true if a token is queued for processing, or is currently being processed.
func (api TokenAPI) GetProcessingState(ctx context.Context, tokenID persist.DBID) (bool, error) {
	token, err := api.loaders.GetTokenByIdBatch.Load(tokenID)
	if err != nil {
		return false, err
	}
	contract, err := api.loaders.GetContractsByIDs.Load(token.Contract.String())
	if err != nil {
		return false, err
	}
	t := persist.NewTokenIdentifiers(contract.Address, token.TokenID, contract.Chain)
	return api.manager.Processing(ctx, t), nil
}

// syncableChains returns a list of chains that the user is allowed to sync, and a callback to release the locks for those chains.
func syncableChains(ctx context.Context, userID persist.DBID, chains []persist.Chain, throttler *throttle.Locker) ([]persist.Chain, func(), error) {
	chainsToSync := make([]persist.Chain, 0, len(chains))
	acquiredLocks := make([]string, 0, len(chains))

	for _, chain := range chains {
		k := fmt.Sprintf("sync:owned:%d:%s", chain, userID)
		err := throttler.Lock(ctx, k)
		if err != nil && util.ErrorAs[throttle.ErrThrottleLocked](err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		chainsToSync = append(chainsToSync, chain)
		acquiredLocks = append(acquiredLocks, k)
	}

	callback := func() {
		for _, lock := range acquiredLocks {
			throttler.Unlock(ctx, lock)
		}
	}

	return chainsToSync, callback, nil
}
