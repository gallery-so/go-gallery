package multichain

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc"
	"github.com/sourcegraph/conc/pool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/logger"
	op "github.com/mikeydub/go-gallery/service/multichain/operation"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("TOKEN_PROCESSING_URL", "required")
}

var unknownContractNames = map[string]bool{
	"unidentified contract": true,
	"unknown contract":      true,
	"unknown":               true,
}

const maxCommunitySize = 1000

// SubmitTokens is called to process a batch of tokens
type SubmitTokensF func(ctx context.Context, tDefIDs []persist.DBID) error

type Provider struct {
	Repos        *postgres.Repositories
	Queries      *db.Queries
	SubmitTokens SubmitTokensF
	Chains       ProviderLookup
}

// ChainAgnosticToken is a token that is agnostic to the chain it is on
type ChainAgnosticToken struct {
	Descriptors     ChainAgnosticTokenDescriptors `json:"descriptors"`
	TokenType       persist.TokenType             `json:"token_type"`
	TokenURI        persist.TokenURI              `json:"token_uri"`
	TokenID         persist.TokenID               `json:"token_id"`
	Quantity        persist.HexString             `json:"quantity"`
	OwnerAddress    persist.Address               `json:"owner_address"`
	TokenMetadata   persist.TokenMetadata         `json:"metadata"`
	ContractAddress persist.Address               `json:"contract_address"`
	FallbackMedia   persist.FallbackMedia         `json:"fallback_media"`
	ExternalURL     string                        `json:"external_url"`
	BlockNumber     persist.BlockNumber           `json:"block_number"`
	IsSpam          *bool                         `json:"is_spam"`
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Descriptors ChainAgnosticContractDescriptors `json:"descriptors"`
	Address     persist.Address                  `json:"address"`
	IsSpam      *bool                            `json:"is_spam"`
	LatestBlock persist.BlockNumber              `json:"latest_block"`
}

type ChainAgnosticTokensAndContracts struct {
	Tokens    []ChainAgnosticToken    `json:"tokens"`
	Contracts []ChainAgnosticContract `json:"contracts"`
	// super hacky way to get the actual chain of the token
	ActualChain persist.Chain
}

// ChainAgnosticTokenDescriptors are the fields that describe a token but cannot be used to uniquely identify it
type ChainAgnosticTokenDescriptors struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ChainAgnosticContractDescriptors struct {
	Symbol          string          `json:"symbol"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ProfileImageURL string          `json:"profile_image_url"`
	OwnerAddress    persist.Address `json:"creator_address"`
}

// ChainAgnosticIdentifiers identify tokens despite their chain
type ChainAgnosticIdentifiers struct {
	ContractAddress persist.Address `json:"contract_address"`
	TokenID         persist.TokenID `json:"token_id"`
}

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("token(address=%s; id=%s)", t.ContractAddress, t.TokenID)
}

type ErrProviderFailed struct {
	Err      error
	Provider any
}

func (e ErrProviderFailed) Unwrap() error { return e.Err }
func (e ErrProviderFailed) Error() string {
	p := "provider"

	if i, ok := e.Provider.(Infoer); ok {
		info := i.ProviderInfo()
		p = fmt.Sprintf("%s(chain=%d, chainID=%d)", info.ProviderID, info.Chain, info.ChainID)
	}

	return fmt.Sprintf("calling %s failed: %s", p, e.Err)
}

type chainTokens struct {
	priority int
	chain    persist.Chain
	tokens   []ChainAgnosticToken
}

type chainContracts struct {
	priority  int
	chain     persist.Chain
	contracts []ChainAgnosticContract
}

type communityInfo interface {
	GetKey() persist.CommunityKey
	GetName() string
	GetDescription() string
	GetProfileImageURL() string
	GetCreatorAddresses() []persist.ChainAddress
	GetWebsiteURL() string
}

type ProviderInfo struct {
	Chain      persist.Chain `json:"chain_name"`
	ChainID    int           `json:"chain_id"`
	ProviderID string        `json:"provider_id"`
}

// Infoer maintains provider settings
type Infoer interface {
	ProviderInfo() ProviderInfo
}

// Verifier can verify that a signature is signed by a given key
type Verifier interface {
	VerifySignature(ctx context.Context, pubKey persist.PubKey, walletType persist.WalletType, nonce string, sig string) (bool, error)
}

// TokensOwnerFetcher supports fetching tokens for syncing
type TokensOwnerFetcher interface {
	GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokenByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

// TokensIncrementalOwnerFetcher supports fetching tokens for syncing incrementally
type TokensIncrementalOwnerFetcher interface {
	// NOTE: implementation MUST close the rec channel
	GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (rec <-chan ChainAgnosticTokensAndContracts, errChain <-chan error)
}

// TokensIncrementalContractFetcher supports fetching tokens by contract for syncing incrementally
type TokensIncrementalContractFetcher interface {
	// NOTE: implementations MUST close the rec channel
	// maxLimit is not for pagination, it is to make sure we don't fetch a bajilion tokens from an omnibus contract
	GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (rec <-chan ChainAgnosticTokensAndContracts, errChain <-chan error)
}

type TokensContractFetcher interface {
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

type ContractsFetcher interface {
	GetContractByAddress(ctx context.Context, contract persist.Address) (ChainAgnosticContract, error)
}

type ContractsOwnerFetcher interface {
	GetContractsByOwnerAddress(ctx context.Context, owner persist.Address) ([]ChainAgnosticContract, error)
}

// ContractRefresher supports refreshes of a contract
type ContractRefresher interface {
	ContractsFetcher
	RefreshContract(context.Context, persist.Address) error
}

// TokenMetadataFetcher supports fetching token metadata
type TokenMetadataFetcher interface {
	GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (persist.TokenMetadata, error)
}

type TokenDescriptorsFetcher interface {
	GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (ChainAgnosticTokenDescriptors, ChainAgnosticContractDescriptors, error)
}

type chainTokensAndContracts struct {
	tokens    chainTokens
	contracts chainContracts
}

// SyncTokensByUserID updates the media for all tokens for a user
func (p *Provider) SyncTokensByUserID(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	for _, w := range user.Wallets {
		fmt.Println(w)
	}

	chainsToAddresses := p.matchingWallets(user.Wallets, chains)
	if len(chainsToAddresses) == 0 {
		return nil
	}

	recCh := make(chan chainTokensAndContracts, len(chains)*len(chainsToAddresses)*8)
	errCh := make(chan error)
	wg := &conc.WaitGroup{}

	for c, a := range chainsToAddresses {
		fetcher, ok := p.Chains[c].(TokensIncrementalOwnerFetcher)
		if !ok {
			continue
		}

		for _, addr := range a {
			addr := addr
			chain := c

			f := func() {
				logger.For(ctx).Infof("syncing chain=%d; user=%s; wallet=%s", chain, user.Username.String(), addr)

				pageCh, pageErrCh := fetcher.GetTokensIncrementallyByWalletAddress(ctx, addr)
				for {
					select {
					case page, ok := <-pageCh:
						if !ok {
							return
						}

						// hack to get the chain
						chainOverride := chain

						if page.ActualChain != 0 {
							chainOverride = page.ActualChain
						}

						recCh <- chainTokensAndContracts{
							tokens:    chainTokens{chain: chainOverride, tokens: page.Tokens},
							contracts: chainContracts{chain: chainOverride, contracts: page.Contracts},
						}

					case err, ok := <-pageErrCh:
						if !ok {
							return
						}
						errCh <- ErrProviderFailed{Err: err, Provider: fetcher}
						return
					}
				}
			}

			wg.Go(f)
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	_, _, _, err = p.replaceHolderTokensForUser(ctx, user, chains, recCh, errCh)
	return err
}

// SyncCreatedTokensForNewContracts syncs tokens for contracts that the user created but does not currently have any tokens for.
func (p *Provider) SyncCreatedTokensForNewContracts(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	chainsToAddresses := p.matchingWallets(user.Wallets, chains)
	if len(chainsToAddresses) == 0 {
		return nil
	}

	recCh := make(chan chainTokensAndContracts, len(chains)*len(chainsToAddresses)*8)
	errCh := make(chan error)

	wg := &conc.WaitGroup{}
	for c, a := range chainsToAddresses {
		contractFetcher, contractOK := p.Chains[c].(ContractsOwnerFetcher)
		tokenFetcher, tokenOK := p.Chains[c].(TokensIncrementalContractFetcher)

		if !contractOK || !tokenOK {
			continue
		}

		for _, addr := range a {
			addr := addr
			chain := c

			wg.Go(func() {
				innerWg := &conc.WaitGroup{}

				contracts, err := contractFetcher.GetContractsByOwnerAddress(ctx, addr)
				if err != nil {
					errCh <- ErrProviderFailed{Err: err, Provider: contractFetcher}
					return
				}

				for _, contract := range contracts {
					c := contract

					f := func() {
						logger.For(ctx).Infof("syncing chain=%d; user=%s; contract=%s", chain, user.Username.String(), addr)

						pageCh, pageErrCh := tokenFetcher.GetTokensIncrementallyByContractAddress(ctx, c.Address, maxCommunitySize)
						for {
							select {
							case page, ok := <-pageCh:
								if !ok {
									return
								}
								recCh <- chainTokensAndContracts{
									tokens:    chainTokens{chain: chain, tokens: page.Tokens},
									contracts: chainContracts{chain: chain, contracts: contracts},
								}
							case err, ok := <-pageErrCh:
								if !ok {
									return
								}
								errCh <- ErrProviderFailed{Err: err, Provider: tokenFetcher}
								return
							}
						}
					}

					innerWg.Go(f)
				}
				innerWg.Wait()
			})
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	_, _, _, err = p.replaceCreatorTokensForUser(ctx, user, chains, recCh, errCh)
	if err != nil {
		return err
	}

	// Remove creator status from any tokens this user is no longer the creator of
	return p.Queries.RemoveStaleCreatorStatusFromTokens(ctx, userID)
}

// SyncTokensByUserIDAndTokenIdentifiers updates the media for specific tokens for a user
func (p *Provider) SyncTokensByUserIDAndTokenIdentifiers(ctx context.Context, userID persist.DBID, tokenIdentifiers []persist.TokenUniqueIdentifiers) ([]op.TokenFullDetails, error) {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	chains, _ := util.Map(tokenIdentifiers, func(i persist.TokenUniqueIdentifiers) (persist.Chain, error) { return i.Chain, nil })
	chains = util.Dedupe(chains, false)

	matchingWallets := p.matchingWallets(user.Wallets, chains)

	chainAddresses := map[persist.ChainAddress]bool{}
	for chain, addresses := range matchingWallets {
		for _, address := range addresses {
			chainAddresses[persist.NewChainAddress(address, chain)] = true
		}
	}

	chainsToTokenIdentifiers := make(map[persist.Chain][]persist.TokenUniqueIdentifiers)
	for _, tid := range tokenIdentifiers {
		// check if the user owns the wallet that owns the token
		if !chainAddresses[persist.NewChainAddress(tid.OwnerAddress, tid.Chain)] {
			continue
		}
		chainsToTokenIdentifiers[tid.Chain] = append(chainsToTokenIdentifiers[tid.Chain], tid)
	}

	for c, a := range chainsToTokenIdentifiers {
		chainsToTokenIdentifiers[c] = util.Dedupe(a, false)
	}

	recCh := make(chan chainTokensAndContracts, len(tokenIdentifiers))
	errCh := make(chan error)

	wg := &conc.WaitGroup{}

	for c, t := range chainsToTokenIdentifiers {
		chain := c
		tids := t

		fetcher, ok := p.Chains[chain].(TokensOwnerFetcher)
		if !ok {
			continue
		}

		for _, tid := range tids {
			tid := tid

			f := func() {
				logger.For(ctx).Infof("syncing chain=%d; user=%s; token=%s", chain, user.Username.String(), tid)

				id := ChainAgnosticIdentifiers{ContractAddress: tid.ContractAddress, TokenID: tid.TokenID}

				token, contract, err := fetcher.GetTokenByTokenIdentifiersAndOwner(ctx, id, tid.OwnerAddress)
				if err != nil {
					errCh <- ErrProviderFailed{Err: err, Provider: fetcher}
					return
				}

				recCh <- chainTokensAndContracts{
					tokens:    chainTokens{chain: chain, tokens: []ChainAgnosticToken{token}},
					contracts: chainContracts{chain: chain, contracts: []ChainAgnosticContract{contract}},
				}
			}

			wg.Go(f)
		}
	}

	go func() {
		defer close(recCh)
		defer close(errCh)
		wg.Wait()
	}()

	_, newTokens, _, err := p.addHolderTokensForUser(ctx, user, chains, recCh, errCh)
	return newTokens, err
}

// TokenExists checks if a token exists according to any provider by its identifiers. It returns nil if the token exists.
// If a token exists, it will also update its contract and its descriptors in the database.
func (p *Provider) TokenExists(ctx context.Context, token persist.TokenIdentifiers, r retry.Retry) (td db.TokenDefinition, err error) {
	searchF := func(ctx context.Context) error {
		td, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, persist.TokenIdentifiers{
			TokenID:         token.TokenID,
			Chain:           token.Chain,
			ContractAddress: token.ContractAddress,
		})
		return err
	}

	logRetryF := func(err error) bool {
		logger.For(ctx).Errorf("polling for token: %s: retrying on error: %s", token.String(), err.Error())
		return true
	}

	retry.RetryFunc(ctx, searchF, logRetryF, r)

	return td, err
}

// SyncTokenByUserWalletsAndTokenIdentifiersRetry attempts to sync a token for a user by their wallets and token identifiers.
func (p *Provider) SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx context.Context, userID persist.DBID, t persist.TokenIdentifiers, r retry.Retry) (token op.TokenFullDetails, err error) {
	user, err := p.Queries.GetUserById(ctx, userID)
	if err != nil {
		return op.TokenFullDetails{}, err
	}

	searchF := func(ctx context.Context) error {
		_, err := p.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
			OwnerID:         user.ID,
			TokenID:         t.TokenID,
			ContractAddress: t.ContractAddress,
			Chain:           t.Chain,
		})
		// Token alrady exists, do nothing
		if err == nil {
			return nil
		}
		// Unexpected error
		if err != nil && !util.ErrorIs[persist.ErrTokenNotFoundByUserTokenIdentifers](err) {
			return err
		}
		// Try to sync the token from each wallet. This treats SyncTokensByUserIDAndTokenIdentifiers as a black box: it runs each
		// wallet in parallel and waits for each wallet to finish. We then check if a token exists in the database at the end and return
		// if it does. Otherwise, we retry until a token is found or the retry limit is reached.
		wg := sync.WaitGroup{}
		for _, w := range p.matchingWalletsChain(user.Wallets, t.Chain) {
			w := w
			searchWallet := persist.TokenUniqueIdentifiers{
				Chain:           t.Chain,
				ContractAddress: t.ContractAddress,
				TokenID:         t.TokenID,
				OwnerAddress:    w,
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.SyncTokensByUserIDAndTokenIdentifiers(ctx, user.ID, []persist.TokenUniqueIdentifiers{searchWallet})
			}()
		}
		wg.Wait()
		token, err = op.TokenFullDetailsByUserTokenIdentifiers(ctx, p.Queries, user.ID, t)
		return err
	}

	logRetryF := func(err error) bool {
		logger.For(ctx).Errorf("polling for token for user=%s: polling for token=%s: retrying on error: %s", user.ID, t.String(), err.Error())
		return true
	}

	err = retry.RetryFunc(ctx, searchF, logRetryF, r)

	return token, err
}

func (p *Provider) SyncCreatedTokensForExistingContract(ctx context.Context, userID persist.DBID, contractID persist.DBID) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	contract, err := p.Queries.GetContractByID(ctx, contractID)
	if err != nil {
		return err
	}

	f, ok := p.Chains[contract.Chain].(TokensIncrementalContractFetcher)
	if !ok {
		return fmt.Errorf("no tokens contract fetcher for chain: %d", contract.Chain)
	}

	recCh := make(chan chainTokensAndContracts, 8)
	errCh := make(chan error)

	go func() {
		pageCh, pageErrCh := f.GetTokensIncrementallyByContractAddress(ctx, contract.Address, maxCommunitySize)
		defer close(recCh)
		defer close(errCh)
		for {
			select {
			case page, ok := <-pageCh:
				if !ok {
					return
				}
				recCh <- chainTokensAndContracts{
					tokens:    chainTokens{chain: contract.Chain, tokens: page.Tokens},
					contracts: chainContracts{chain: contract.Chain, contracts: page.Contracts},
				}
			case err, ok := <-pageErrCh:
				if !ok {
					return
				}
				errCh <- ErrProviderFailed{Err: err, Provider: f}
				return
			}
		}
	}()

	_, _, _, err = p.replaceCreatorTokensForUser(ctx, user, []persist.Chain{contract.Chain}, recCh, errCh)
	return err
}

func (p *Provider) processTokenCommunities(ctx context.Context, contracts []db.Contract, tokens []op.TokenFullDetails) error {
	knownProviders, err := p.Queries.GetCommunityContractProviders(ctx, util.MapWithoutError(contracts, func(c db.Contract) persist.DBID { return c.ID }))
	if err != nil {
		return fmt.Errorf("failed to retrieve contract community types: %w", err)
	}

	// TODO: Make this more flexible, allow other providers, etc (possibly via wire)
	return p.processArtBlocksTokenCommunities(ctx, knownProviders, tokens)
}

func (p *Provider) processTokensForUsers(ctx context.Context, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens,
	existingTokensForUsers map[persist.DBID][]op.TokenFullDetails, contracts []db.Contract,
	upsertParams op.TokenUpsertParams) (currentUserTokens map[persist.DBID][]op.TokenFullDetails, newUserTokens map[persist.DBID][]op.TokenFullDetails, err error) {

	upsertableDefinitions := make([]db.TokenDefinition, 0)
	upsertableTokens := make([]op.UpsertToken, 0)
	tokensIsNewForUser := make(map[persist.DBID]map[persist.TokenIdentifiers]bool)

	for userID, user := range users {
		tokens := chainTokensToUpsertableTokens(chainTokensForUsers[userID], contracts, user)
		tokensIsNewForUser[userID] = differenceTokens(
			util.MapWithoutError(tokens, func(t op.UpsertToken) persist.TokenIdentifiers { return t.Identifiers }),
			util.MapWithoutError(existingTokensForUsers[userID], func(t op.TokenFullDetails) persist.TokenIdentifiers {
				return persist.NewTokenIdentifiers(t.Definition.ContractAddress, t.Definition.TokenID, t.Definition.Chain)
			}),
		)
		definitions := chainTokensToUpsertableTokenDefinitions(ctx, chainTokensForUsers[userID], contracts)
		upsertableTokens = append(upsertableTokens, tokens...)
		upsertableDefinitions = append(upsertableDefinitions, definitions...)
	}

	uniqueTokens := dedupeTokenInstances(upsertableTokens)
	uniqueDefinitions := dedupeTokenDefinitions(upsertableDefinitions)

	upsertTime, upsertedTokens, err := op.BulkUpsert(ctx, p.Queries, uniqueTokens, uniqueDefinitions, upsertParams.SetCreatorFields, upsertParams.SetHolderFields)
	if err != nil {
		return nil, nil, err
	}

	// Create a lookup for userID to persisted token IDs
	currentUserTokens = make(map[persist.DBID][]op.TokenFullDetails)
	for _, token := range upsertedTokens {
		currentUserTokens[token.Instance.OwnerUserID] = append(currentUserTokens[token.Instance.OwnerUserID], token)
	}

	// TODO: Consider tracking (token_definition_id, community_type) in a table so we'd know whether we've already
	// evaluated a token for a given community type and can avoid checking it again.
	communityTokens := upsertedTokens
	err = p.processTokenCommunities(ctx, contracts, communityTokens)
	if err != nil {
		// Report errors, but don't return. We can retry token community memberships at some point, but the whole
		// sync shouldn't fail because a community provider's API was unavailable.
		err = fmt.Errorf("failed to process token communities: %w", err)
		logger.For(ctx).WithError(err).Error(err)
		sentryutil.ReportError(ctx, err)
	}

	if upsertParams.OptionalDelete != nil {
		numAffectedRows, err := p.Queries.DeleteTokensBeforeTimestamp(ctx, upsertParams.OptionalDelete.ToParams(upsertTime))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to delete tokens: %w", err)
		}
		logger.For(ctx).Infof("deleted %d tokens", numAffectedRows)
	}

	newUserTokens = make(map[persist.DBID][]op.TokenFullDetails, len(users))

	for userID := range users {
		newTokensForUser := tokensIsNewForUser[userID]
		currentTokensForUser := currentUserTokens[userID]
		newUserTokens[userID] = make([]op.TokenFullDetails, 0, len(currentTokensForUser))
		for _, token := range currentTokensForUser {
			tID := persist.NewTokenIdentifiers(token.Definition.ContractAddress, token.Definition.TokenID, token.Definition.Chain)
			if newTokensForUser[tID] {
				newUserTokens[userID] = append(newUserTokens[userID], token)
			}
		}
	}

	for userID := range users {
		// include the existing tokens that were not persisted with the bulk upsert
		currentUserTokens[userID] = util.DedupeWithTranslate(append(currentUserTokens[userID], existingTokensForUsers[userID]...), false, func(t op.TokenFullDetails) persist.DBID { return t.Instance.ID })
	}

	// Sort by the default ordering the frontend uses to displays tokens so that tokens are processed in the same order
	// The order is created time desc, token name desc, then token DBID desc.
	sort.Slice(upsertedTokens, func(i, j int) bool {
		tokenI := upsertedTokens[i]
		tokenJ := upsertedTokens[j]

		if tokenI.Instance.CreatedAt.After(tokenJ.Instance.CreatedAt) {
			return true
		} else if tokenI.Instance.CreatedAt.Before(tokenJ.Instance.CreatedAt) {
			return false
		}

		if tokenI.Definition.Name.String > tokenJ.Definition.Name.String {
			return true
		} else if tokenI.Definition.Name.String < tokenJ.Definition.Name.String {
			return false
		}

		return tokenI.Instance.ID > tokenJ.Instance.ID
	})

	batches := make([][]persist.DBID, 0)

	for _, t := range upsertedTokens {
		// Submit tokens that are missing media IDs. Tokens that are missing media IDs are new tokens, or tokens that weren't processed for whatever reason.
		if t.Definition.TokenMediaID == "" {
			curBatch := len(batches) - 1

			if curBatch == -1 {
				batches = append(batches, []persist.DBID{})
				curBatch = 0
			}

			// Break up into smaller batches so that a batch is handled per request
			if len(batches[curBatch]) >= 50 {
				batches = append(batches, make([]persist.DBID, 0, 50))
				curBatch++
			}

			batches[curBatch] = append(batches[curBatch], t.Definition.ID)
		}
	}

	for _, b := range batches {
		err = p.SubmitTokens(ctx, b)
		if err != nil {
			logger.For(ctx).Errorf("failed to submit batch: %s", err)
			sentryutil.ReportError(ctx, err)
		}
	}

	return currentUserTokens, newUserTokens, err
}

type addTokensFunc func(context.Context, persist.User, []chainTokens, []db.Contract, []op.TokenFullDetails) (allTokens []op.TokenFullDetails, newTokens []op.TokenFullDetails, err error)

func (p *Provider) addCreatorTokensOfContractsToUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, existingTokens []op.TokenFullDetails) (currentTokenState []op.TokenFullDetails, newTokens []op.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, existingTokens, op.TokenUpsertParams{
		SetCreatorFields: true,
		SetHolderFields:  false,
		OptionalDelete:   nil,
	})
}

func (p *Provider) addHolderTokensToUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, existingTokens []op.TokenFullDetails) (currentTokenState []op.TokenFullDetails, newTokens []op.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, existingTokens, op.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete:   nil,
	})
}

func (p *Provider) processTokensForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, existingTokens []op.TokenFullDetails, upsertParams op.TokenUpsertParams) (currentTokenState []op.TokenFullDetails, newTokens []op.TokenFullDetails, error error) {
	userMap := map[persist.DBID]persist.User{user.ID: user}
	providerTokenMap := map[persist.DBID][]chainTokens{user.ID: tokensFromProviders}
	existingTokenMap := map[persist.DBID][]op.TokenFullDetails{user.ID: existingTokens}
	currentUserTokens, newUserTokens, err := p.processTokensForUsers(ctx, userMap, providerTokenMap, existingTokenMap, contracts, upsertParams)
	if err != nil {
		return nil, nil, err
	}
	return currentUserTokens[user.ID], newUserTokens[user.ID], nil
}

func (p *Provider) receiveProviderData(ctx context.Context, user persist.User, recCh <-chan chainTokensAndContracts, errCh <-chan error, addTokensF addTokensFunc) (
	currentTokens []op.TokenFullDetails,
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	currentTokens, err = op.TokensFullDetailsByUserID(ctx, p.Queries, user.ID)
	if err != nil {
		return
	}

	currentContracts = util.MapWithoutError(currentTokens, func(t op.TokenFullDetails) db.Contract { return t.Contract })
	currentContracts = util.DedupeWithTranslate(currentContracts, true, func(c db.Contract) persist.DBID { return c.ID })

	for {
		select {
		case page, ok := <-recCh:
			if !ok {
				return
			}
			currentContracts, _, err = p.processContracts(ctx, []chainContracts{page.contracts}, currentContracts, false)
			if err != nil {
				return
			}
			currentTokens, newTokens, err = addTokensF(ctx, user, []chainTokens{page.tokens}, currentContracts, currentTokens)
			if err != nil {
				return
			}
		case <-ctx.Done():
			err = ctx.Err()
			return
		case err, ok := <-errCh:
			if ok {
				return nil, nil, nil, err
			}
		}
	}
}

func (p *Provider) addHolderTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	currentTokens []op.TokenFullDetails,
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	return p.receiveProviderData(ctx, user, recCh, errCh, p.addHolderTokensToUser)
}

func (p *Provider) replaceCreatorTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	currentTokens []op.TokenFullDetails,
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	now := time.Now()
	currentTokens, newTokens, currentContracts, err = p.receiveProviderData(ctx, user, recCh, errCh, p.addCreatorTokensOfContractsToUser)
	if err != nil {
		return
	}
	_, err = p.Queries.DeleteTokensBeforeTimestamp(ctx, db.DeleteTokensBeforeTimestampParams{
		RemoveHolderStatus:  false,
		RemoveCreatorStatus: true,
		OnlyFromUserID:      util.ToNullString(user.ID.String(), true),
		OnlyFromContractIds: util.MapWithoutError(currentContracts, func(c db.Contract) string { return c.ID.String() }),
		OnlyFromChains:      util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) }),
		Timestamp:           now,
	})
	if err != nil {
		return
	}
	return
}

func (p *Provider) replaceHolderTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, recCh <-chan chainTokensAndContracts, errCh <-chan error) (
	currentTokens []op.TokenFullDetails,
	newTokens []op.TokenFullDetails,
	currentContracts []db.Contract,
	err error,
) {
	now := time.Now()
	currentTokens, newTokens, currentContracts, err = p.receiveProviderData(ctx, user, recCh, errCh, p.addHolderTokensToUser)
	if err != nil {
		return
	}
	_, err = p.Queries.DeleteTokensBeforeTimestamp(ctx, db.DeleteTokensBeforeTimestampParams{
		RemoveHolderStatus:  true,
		RemoveCreatorStatus: false,
		OnlyFromUserID:      util.ToNullString(user.ID.String(), true),
		OnlyFromContractIds: util.MapWithoutError(currentContracts, func(c db.Contract) string { return c.ID.String() }),
		OnlyFromChains:      util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) }),
		Timestamp:           now,
	})
	if err != nil {
		return
	}
	return
}

func (p *Provider) GetTokensOfContractForWallet(ctx context.Context, contractAddress persist.ChainAddress, wallet persist.L1ChainAddress) ([]op.TokenFullDetails, error) {
	user, err := p.Repos.UserRepository.GetByChainAddress(ctx, wallet)
	if err != nil {
		return nil, err
	}

	f, ok := p.Chains[contractAddress.Chain()].(TokensOwnerFetcher)
	if !ok {
		return nil, fmt.Errorf("no tokens owner fetcher for chain: %d", contractAddress.Chain())
	}

	recCh := make(chan chainTokensAndContracts)
	errCh := make(chan error)

	go func() {
		defer close(recCh)
		defer close(errCh)
		tokens, contracts, err := f.GetTokensByWalletAddress(ctx, wallet.Address())
		if err != nil {
			errCh <- err
			return
		}

		var targetContract chainContracts
		var targetTokens []ChainAgnosticToken

		// Only process the contract and tokens of that contract instead of all tokens held in the wallet

		for _, c := range contracts {
			if c.Address == contractAddress.Address() {
				targetContract = chainContracts{chain: contractAddress.Chain(), contracts: []ChainAgnosticContract{c}}
				break
			}
		}

		if len(targetContract.contracts) == 0 {
			errCh <- fmt.Errorf("unable to fetch %s", contractAddress.String())
			return
		}

		for _, t := range tokens {
			if t.ContractAddress == contractAddress.Address() {
				targetTokens = append(targetTokens, t)
			}
		}

		if len(targetTokens) == 0 {
			errCh <- fmt.Errorf("unable to find tokens owned by wallet=%s for contract=%s", wallet.Address().String(), contractAddress.String())
			return
		}

		recCh <- chainTokensAndContracts{
			tokens:    chainTokens{chain: targetContract.chain, tokens: targetTokens},
			contracts: targetContract,
		}
	}()

	currentTokens, _, _, err := p.addHolderTokensForUser(ctx, user, []persist.Chain{contractAddress.Chain()}, recCh, errCh)
	if err != nil {
		return nil, err
	}

	targetTokens := util.Filter(currentTokens, func(t op.TokenFullDetails) bool {
		return persist.NewChainAddress(t.Contract.Address, t.Contract.Chain) == contractAddress
	}, false)

	return targetTokens, nil
}

// GetTokenMetadataByTokenIdentifiers will get the metadata for a given token identifier
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, chain persist.Chain) (persist.TokenMetadata, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fetcher, ok := p.Chains[chain].(TokenMetadataFetcher)
	if !ok {
		return nil, fmt.Errorf("no metadata fetchers for chain %d", chain)
	}

	return fetcher.GetTokenMetadataByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{ContractAddress: contractAddress, TokenID: tokenID})
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	if verifier, ok := p.Chains[pChainAddress.Chain()].(Verifier); ok {
		if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (p *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers) error {
	body, err := json.Marshal(map[string]any{
		"token_id":         ti.TokenID,
		"contract_address": ti.ContractAddress,
		"chain":            ti.Chain,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/media/process/token", env.GetString("TOKEN_PROCESSING_URL"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.BodyAsError(resp)
	}

	_, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, ti)
	return err
}

// RefreshTokenDescriptorsByTokenIdentifiers will refresh the token descriptors for a token by its identifiers.
func (p *Provider) RefreshTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) (db.TokenDefinition, error) {
	finalTokenDescriptors := ChainAgnosticTokenDescriptors{}
	finalContractDescriptors := ChainAgnosticContractDescriptors{}

	fetcher, ok := p.Chains[ti.Chain].(TokenDescriptorsFetcher)
	if !ok {
		return db.TokenDefinition{}, fmt.Errorf("no token descriptor fetchers for chain %d", ti.Chain)
	}

	tokenExists := false

	id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}

	token, contract, err := fetcher.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
	if err == nil {
		tokenExists = true
		// token
		if token.Name != "" && finalContractDescriptors.Name == "" {
			finalTokenDescriptors.Name = token.Name
		}
		if token.Description != "" && finalContractDescriptors.Description == "" {
			finalTokenDescriptors.Description = token.Description
		}

		// contract
		if (contract.Name != "" && !unknownContractNames[strings.ToLower(contract.Name)]) && finalContractDescriptors.Name == "" {
			finalContractDescriptors.Name = contract.Name
		}
		if contract.Description != "" && finalContractDescriptors.Description == "" {
			finalContractDescriptors.Description = contract.Description
		}
		if contract.Symbol != "" && finalContractDescriptors.Symbol == "" {
			finalContractDescriptors.Symbol = contract.Symbol
		}
		if contract.OwnerAddress != "" && finalContractDescriptors.OwnerAddress == "" {
			finalContractDescriptors.OwnerAddress = contract.OwnerAddress
		}
		if contract.ProfileImageURL != "" && finalContractDescriptors.ProfileImageURL == "" {
			finalContractDescriptors.ProfileImageURL = contract.ProfileImageURL
		}
	} else {
		logger.For(ctx).Infof("token %s-%s-%d not found for refresh (err: %s)", ti.TokenID, ti.ContractAddress, ti.Chain, err)
	}

	if !tokenExists {
		return db.TokenDefinition{}, persist.ErrTokenNotFoundByTokenIdentifiers{Token: ti}
	}

	c, err := p.Repos.ContractRepository.Upsert(ctx, db.Contract{
		Name:            util.ToNullString(finalContractDescriptors.Name, true),
		Symbol:          util.ToNullString(finalContractDescriptors.Symbol, true),
		Address:         persist.Address(ti.Chain.NormalizeAddress(ti.ContractAddress)),
		Chain:           ti.Chain,
		ProfileImageUrl: util.ToNullString(finalContractDescriptors.ProfileImageURL, true),
		Description:     util.ToNullString(finalContractDescriptors.Description, true),
		OwnerAddress:    finalContractDescriptors.OwnerAddress,
	}, true)
	if err != nil {
		return db.TokenDefinition{}, err
	}

	return p.Queries.UpdateTokenMetadataFieldsByTokenIdentifiers(ctx, db.UpdateTokenMetadataFieldsByTokenIdentifiersParams{
		Name:        util.ToNullString(finalTokenDescriptors.Name, true),
		Description: util.ToNullString(finalTokenDescriptors.Description, true),
		TokenID:     ti.TokenID,
		ContractID:  c.ID,
		Chain:       ti.Chain,
	})
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	var contracts []chainContracts

	if refresher, ok := p.Chains[ci.Chain].(ContractRefresher); ok {
		if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}

	if fetcher, ok := p.Chains[ci.Chain].(ContractsFetcher); ok {
		c, err := fetcher.GetContractByAddress(ctx, ci.ContractAddress)
		if err != nil {
			return err
		}
		contracts = append(contracts, chainContracts{chain: ci.Chain, contracts: []ChainAgnosticContract{c}})
	}

	_, _, err := p.processContracts(ctx, contracts, nil, false)
	return err
}

type ContractOwnerResult struct {
	Contracts []ChainAgnosticContract
	Chain     persist.Chain
}

func (p *Provider) SyncContractsOwnedByUser(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := p.Queries.GetUserById(ctx, userID)
	if err != nil {
		return err
	}

	if len(chains) == 0 {
		for chain := range p.Chains {
			chains = append(chains, chain)
		}
	}
	contractsFromProviders := []chainContracts{}

	searchAddresses := p.matchingWallets(user.Wallets, chains)
	providerPool := pool.NewWithResults[ContractOwnerResult]().WithContext(ctx)

	for chain, addresses := range searchAddresses {

		fetcher, ok := p.Chains[chain].(ContractsOwnerFetcher)
		if !ok {
			continue
		}

		for _, address := range addresses {
			c := chain
			a := address

			f := func(ctx context.Context) (ContractOwnerResult, error) {
				contracts, err := fetcher.GetContractsByOwnerAddress(ctx, a)
				if err != nil {
					logger.For(ctx).Errorf("error fetching contracts for address %s: %s", a, err)
					return ContractOwnerResult{Chain: c}, nil
				}
				logger.For(ctx).Debugf("found %d contracts for address %s", len(contracts), a)
				return ContractOwnerResult{Contracts: contracts, Chain: c}, nil
			}

			providerPool.Go(f)
		}
	}

	pResult, err := providerPool.Wait()
	if err != nil {
		return err
	}

	for _, result := range pResult {
		contractsFromProviders = append(contractsFromProviders, chainContracts{chain: result.Chain, contracts: result.Contracts})
	}

	_, _, err = p.processContracts(ctx, contractsFromProviders, nil, true)
	return err
}

// matchingWallets returns wallet addresses that belong to any of the passed chains
func (p *Provider) matchingWallets(wallets []persist.Wallet, chains []persist.Chain) map[persist.Chain][]persist.Address {
	matches := make(map[persist.Chain][]persist.Address)
	for _, chain := range chains {
		for _, wallet := range wallets {
			if wallet.Chain == chain {
				matches[chain] = append(matches[chain], wallet.Address)
			} else if overrides := chain.L1ChainGroup(); util.Contains(overrides, wallet.Chain) {
				matches[chain] = append(matches[chain], wallet.Address)
			}
		}
	}
	for chain, addresses := range matches {
		matches[chain] = util.Dedupe(addresses, true)
	}
	return matches
}

// matchingWalletsChain returns a list of wallets that match the given chain
func (p *Provider) matchingWalletsChain(wallets []persist.Wallet, chain persist.Chain) []persist.Address {
	return p.matchingWallets(wallets, []persist.Chain{chain})[chain]
}

// processContractCommunities ensures that every contract has a corresponding "contract community" in the database.
// This is the most basic community type, and every token belongs to a contract community (because every token belongs
// to a contract, and every contract belongs to a contract community).
func (d *Provider) processContractCommunities(ctx context.Context, contracts []db.Contract) ([]db.Community, error) {
	communities := make([]db.Community, 0, len(contracts))
	for _, contract := range contracts {
		// No need to fill in createdAt, lastUpdated, or deleted. They'll be handled by the upsert.
		communities = append(communities, db.Community{
			ID:              persist.GenerateID(),
			Version:         0,
			Name:            contract.Name.String,
			Description:     contract.Description.String,
			CommunityType:   persist.CommunityTypeContract,
			Key1:            fmt.Sprintf("%d", contract.Chain),
			Key2:            contract.Address.String(),
			ContractID:      contract.ID,
			ProfileImageUrl: contract.ProfileImageUrl,
		})
	}

	communities, err := d.Repos.CommunityRepository.UpsertCommunities(ctx, communities)
	if err != nil {
		return nil, err
	}

	communityByContractID := make(map[persist.DBID]db.Community)
	communityByKey := make(map[persist.CommunityKey]db.Community)

	for _, community := range communities {
		communityByContractID[community.ContractID] = community
		communityByKey[persist.CommunityKey{
			Type: community.CommunityType,
			Key1: community.Key1,
			Key2: community.Key2,
		}] = community
	}

	upsertCreatorsParams := db.UpsertCommunityCreatorsParams{}
	for _, contract := range contracts {
		creatorAddress, ok := util.FindFirst([]persist.Address{contract.OwnerAddress, contract.CreatorAddress}, func(a persist.Address) bool {
			return a != ""
		})

		if !ok {
			continue
		}

		if community, ok := communityByContractID[contract.ID]; ok {
			upsertCreatorsParams.Ids = append(upsertCreatorsParams.Ids, persist.GenerateID().String())
			upsertCreatorsParams.CreatorType = append(upsertCreatorsParams.CreatorType, int32(persist.CommunityCreatorTypeProvider))
			upsertCreatorsParams.CommunityID = append(upsertCreatorsParams.CommunityID, community.ID.String())
			upsertCreatorsParams.CreatorAddress = append(upsertCreatorsParams.CreatorAddress, creatorAddress.String())
			upsertCreatorsParams.CreatorAddressChain = append(upsertCreatorsParams.CreatorAddressChain, int32(contract.Chain))
			upsertCreatorsParams.CreatorAddressL1Chain = append(upsertCreatorsParams.CreatorAddressL1Chain, int32(contract.Chain.L1Chain()))
		}
	}

	if len(upsertCreatorsParams.Ids) > 0 {
		_, err = d.Queries.UpsertCommunityCreators(ctx, upsertCreatorsParams)
		if err != nil {
			err := fmt.Errorf("failed to upsert contract community creators: %w", err)
			logger.For(ctx).WithError(err).Error(err)
			sentryutil.ReportError(ctx, err)
		}
	}

	params := db.UpsertContractCommunityMembershipsParams{}

	for _, contract := range contracts {
		key := persist.CommunityKey{
			Type: persist.CommunityTypeContract,
			Key1: fmt.Sprintf("%d", contract.Chain),
			Key2: contract.Address.String(),
		}

		community, ok := communityByKey[key]
		if !ok {
			// This shouldn't happen. By this point, we've successfully upserted communities,
			// so we should be able to find one matching every contract we have.
			err = fmt.Errorf("couldn't find community with type: %d, key: %s for contract ID: %s", key.Type, key, contract.ID)
			sentryutil.ReportError(ctx, err)
			return nil, err
		}

		params.Ids = append(params.Ids, persist.GenerateID().String())
		params.ContractID = append(params.ContractID, contract.ID.String())
		params.CommunityID = append(params.CommunityID, community.ID.String())
	}

	_, err = d.Queries.UpsertContractCommunityMemberships(ctx, params)
	if err != nil {
		return nil, err
	}

	return communities, nil
}

// processContracts deduplicates contracts and upserts them into the database. If canOverwriteOwnerAddress is true, then
// the owner address of an existing contract will be overwritten if the new contract provides a non-empty owner address.
// An empty owner address will never overwrite an existing address, even if canOverwriteOwnerAddress is true.
func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts, existingContracts []db.Contract, canOverwriteOwnerAddress bool) (dbContracts []db.Contract, newContracts []db.Contract, err error) {
	contractsToUpsert := chainContractsToUpsertableContracts(contractsFromProviders, existingContracts)
	newUpsertedContracts, err := d.Repos.ContractRepository.BulkUpsert(ctx, contractsToUpsert, canOverwriteOwnerAddress)
	if err != nil {
		return nil, nil, err
	}

	dbContracts = util.DedupeWithTranslate(append(newUpsertedContracts, existingContracts...), false, func(c db.Contract) persist.DBID { return c.ID })
	if err != nil {
		return nil, nil, err
	}

	_, err = d.processContractCommunities(ctx, dbContracts)
	if err != nil {
		return nil, nil, err
	}

	return dbContracts, newUpsertedContracts, nil
}

// chainTokensToUpsertableTokenDefinitions returns a slice of token definitions that are ready to be upserted into the database from a slice of chainTokens.
func chainTokensToUpsertableTokenDefinitions(ctx context.Context, chainTokens []chainTokens, existingContracts []db.Contract) []db.TokenDefinition {
	definitions := make(map[persist.TokenIdentifiers]db.TokenDefinition)

	// Create a lookup of contracts to their IDs
	contractLookup := make(map[persist.ContractIdentifiers]db.Contract)
	for _, contract := range existingContracts {
		contractIdentifiers := persist.NewContractIdentifiers(contract.Address, contract.Chain)
		contractLookup[contractIdentifiers] = contract
	}

	for _, chainToken := range chainTokens {
		for _, token := range chainToken.tokens {
			tokenIdentifiers := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chainToken.chain)
			contractIdentifiers := persist.NewContractIdentifiers(token.ContractAddress, chainToken.chain)
			contract, ok := contractLookup[contractIdentifiers]
			if !ok {
				panic(fmt.Sprintf("contract %+v should have already been upserted", contractIdentifiers))
			}
			// Got a new token, add it to the list of token definitions
			if definition, ok := definitions[tokenIdentifiers]; !ok {
				definitions[tokenIdentifiers] = db.TokenDefinition{
					Name:            util.ToNullString(token.Descriptors.Name, true),
					Description:     util.ToNullString(token.Descriptors.Description, true),
					TokenID:         token.TokenID,
					TokenType:       token.TokenType,
					ExternalUrl:     util.ToNullString(token.ExternalURL, true),
					Chain:           chainToken.chain,
					Metadata:        token.TokenMetadata,
					FallbackMedia:   token.FallbackMedia,
					ContractAddress: token.ContractAddress,
					ContractID:      contract.ID,
					TokenMediaID:    "", // Upsert will handle this in the db if the definition already exists
					IsFxhash:        platform.IsFxhash(contractLookup[contractIdentifiers]),
				}
			} else {
				// Merge the token definition with the existing one. The fields that aren't merged below use the data of the first write.
				name := util.FirstNonEmptyString(definition.Name.String, token.Descriptors.Name)
				description := util.FirstNonEmptyString(definition.Description.String, token.Descriptors.Description)
				externalURL := util.FirstNonEmptyString(definition.ExternalUrl.String, token.ExternalURL)
				fallbackMedia, _ := util.FindFirst([]persist.FallbackMedia{definition.FallbackMedia, token.FallbackMedia}, func(m persist.FallbackMedia) bool { return m.IsServable() })
				metadata, _ := util.FindFirst([]persist.TokenMetadata{definition.Metadata, token.TokenMetadata}, func(m persist.TokenMetadata) bool { return len(m) > 0 })
				isFxhash := definition.IsFxhash || platform.IsFxhash(contractLookup[contractIdentifiers])

				definition.Name = util.ToNullString(name, true)
				definition.Description = util.ToNullString(description, true)
				definition.ExternalUrl = util.ToNullString(externalURL, true)
				definition.FallbackMedia = fallbackMedia
				definition.Metadata = metadata
				definition.IsFxhash = isFxhash
				definitions[tokenIdentifiers] = definition
			}
		}
	}

	tokenDefinitions := make([]db.TokenDefinition, 0, len(definitions))
	for _, d := range definitions {
		tokenDefinitions = append(tokenDefinitions, d)
	}

	return tokenDefinitions
}

// chainTokensToUpsertableTokens returns a unique slice of tokens that are ready to be upserted into the database.
func chainTokensToUpsertableTokens(tokens []chainTokens, existingContracts []db.Contract, ownerUser persist.User) []op.UpsertToken {
	addressToContract := make(map[string]db.Contract)

	util.Map(existingContracts, func(c db.Contract) (any, error) {
		addr := c.Chain.NormalizeAddress(c.Address)
		addressToContract[addr] = c
		return nil, nil
	})

	seenTokens := make(map[persist.TokenIdentifiers]op.UpsertToken)
	seenWallets := make(map[persist.TokenIdentifiers][]persist.Wallet)
	seenQuantities := make(map[persist.TokenIdentifiers]persist.HexString)
	addressToWallets := make(map[string]persist.Wallet)
	createdContracts := make(map[persist.Address]bool)

	for _, wallet := range ownerUser.Wallets {
		// could a normalized address ever overlap with the normalized address of another chain?
		normalizedAddress := wallet.Chain.NormalizeAddress(wallet.Address)
		addressToWallets[normalizedAddress] = wallet
	}

	sort.SliceStable(tokens, func(i int, j int) bool {
		return tokens[i].priority < tokens[j].priority
	})

	for _, contract := range existingContracts {
		// If the contract has an override creator, use that to determine whether this user is the contract's creator
		contractAddress := persist.Address(contract.Chain.NormalizeAddress(contract.Address))
		if contract.OverrideCreatorUserID != "" {
			createdContracts[contractAddress] = contract.OverrideCreatorUserID == ownerUser.ID
			continue
		}

		// If the contract doesn't have an override creator, use the first non-empty value in
		// (ownerAddress, creatorAddress) to determine the creator address. If this user
		// has a wallet matching the creator address, they're the creator of the contract.
		creatorAddress := contract.OwnerAddress
		if creatorAddress == "" {
			creatorAddress = contract.CreatorAddress
		}

		if wallet, ok := addressToWallets[contract.Chain.NormalizeAddress(creatorAddress)]; ok {
			createdContracts[contractAddress] = wallet.L1Chain == contract.L1Chain
		} else {
			createdContracts[contractAddress] = false
		}
	}

	for _, chainToken := range tokens {
		for _, token := range chainToken.tokens {

			if token.Quantity.BigInt().Cmp(big.NewInt(0)) == 0 {
				continue
			}

			ti := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chainToken.chain)

			contractAddress := chainToken.chain.NormalizeAddress(token.ContractAddress)
			contract, ok := addressToContract[contractAddress]
			if !ok {
				panic(fmt.Sprintf("no persisted contract for chain=%d, address=%s", chainToken.chain, contractAddress))
			}

			// Duplicate tokens will have the same values for these fields, so we only need to set them once
			if _, ok := seenTokens[ti]; !ok {
				seenTokens[ti] = op.UpsertToken{
					Identifiers: ti,
					Token: db.Token{
						OwnerUserID:    ownerUser.ID,
						BlockNumber:    sql.NullInt64{Int64: token.BlockNumber.BigInt().Int64(), Valid: true},
						IsCreatorToken: createdContracts[persist.Address(contractAddress)],
						ContractID:     contract.ID,
					},
				}
			}

			var found bool
			for _, wallet := range seenWallets[ti] {
				if wallet.Address == token.OwnerAddress {
					found = true
				}
			}
			if !found {
				if q, ok := seenQuantities[ti]; ok {
					seenQuantities[ti] = q.Add(token.Quantity)
				} else {
					seenQuantities[ti] = token.Quantity
				}
			}

			if w, ok := addressToWallets[chainToken.chain.NormalizeAddress(token.OwnerAddress)]; ok {
				seenWallets[ti] = append(seenWallets[ti], w)
				seenWallets[ti] = dedupeWallets(seenWallets[ti])
			}

			finalSeenToken := seenTokens[ti]
			finalSeenToken.Token.OwnedByWallets = util.MapWithoutError(seenWallets[ti], func(w persist.Wallet) persist.DBID { return w.ID })
			finalSeenToken.Token.Quantity = seenQuantities[ti]
			seenTokens[ti] = finalSeenToken
		}
	}

	res := make([]op.UpsertToken, len(seenTokens))

	i := 0
	for _, t := range seenTokens {
		res[i] = t
		i++
	}

	return res
}

type contractMetadata struct {
	Symbol          string
	Name            string
	OwnerAddress    persist.Address
	ProfileImageURL string
	Description     string
	IsSpam          bool
	priority        *int
}

// contractsToUpsertableContracts returns a unique slice of contracts that are ready to be upserted into the database.
func chainContractsToUpsertableContracts(contracts []chainContracts, existingContracts []db.Contract) []db.Contract {

	contractMetadatas := map[persist.ChainAddress]contractMetadata{}
	existingMetadatas := map[persist.ChainAddress]contractMetadata{}

	for _, contract := range existingContracts {
		existingMetadatas[persist.NewChainAddress(contract.Address, contract.Chain)] = contractMetadata{
			Symbol:          contract.Symbol.String,
			Name:            contract.Name.String,
			OwnerAddress:    contract.OwnerAddress,
			ProfileImageURL: contract.ProfileImageUrl.String,
			Description:     contract.Description.String,
			IsSpam:          contract.IsProviderMarkedSpam,
		}
	}

	sort.SliceStable(contracts, func(i, j int) bool {
		return contracts[i].priority < contracts[j].priority
	})

	for _, chainContract := range contracts {
		for _, contract := range chainContract.contracts {

			// we start by checking if there is a DB contract that is higher priority than whatever is currently in the map or whatever we are about to process
			// if it is higher priority, then we will use that as the starting point for the contract we are about to process
			existingMetadata, existsAlready := existingMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)]
			if existsAlready && existingMetadata.priority != nil && chainContract.priority >= *existingMetadata.priority {
				if startingMetadata, startingExists := contractMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)]; !startingExists || startingMetadata.priority == nil || *startingMetadata.priority < *existingMetadata.priority {
					contractMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)] = existingMetadata
				}
			}

			// this is the contract we will use to merge with the existing contract, at this point it could be the higher priority DB contract that we start with,
			// another contract that we have already processed in this scope, or empty if we have not processed a contract for this address yet and no higher priority DB contract existed
			meta := contractMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)]
			contractAsMetadata := contractToMetadata(contract)
			// merge the lower priority new contract into the higher priority "meta" contract. Given that "meta" is in fact empty, it will still have it's empty fields overwritten by the lower priority contract
			meta = mergeContractMetadata(contractAsMetadata, meta)
			if existsAlready {
				// this could be a no-op given that existingMetadata could have been the higher priority DB contract that we started with.
				// in the case that a contract existed in the DB and was not higher priority than what we were processing, we still want to consider it just in case it
				// can address any currently empty fields that this lower priority contract has set
				meta = mergeContractMetadata(existingMetadata, meta)
			}

			contractMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)] = meta
		}
	}

	res := make([]db.Contract, 0, len(contractMetadatas))
	for address, meta := range contractMetadatas {
		res = append(res, db.Contract{
			Chain:                address.Chain(),
			L1Chain:              address.Chain().L1Chain(),
			Address:              address.Address(),
			Symbol:               util.ToNullString(meta.Symbol, true),
			Name:                 util.ToNullString(meta.Name, true),
			ProfileImageUrl:      util.ToNullString(meta.ProfileImageURL, true),
			OwnerAddress:         meta.OwnerAddress,
			Description:          util.ToNullString(meta.Description, true),
			IsProviderMarkedSpam: meta.IsSpam,
		})
	}
	return res

}

func contractToMetadata(contract ChainAgnosticContract) contractMetadata {
	return contractMetadata{
		Symbol:          contract.Descriptors.Symbol,
		Name:            contract.Descriptors.Name,
		OwnerAddress:    contract.Descriptors.OwnerAddress,
		ProfileImageURL: contract.Descriptors.ProfileImageURL,
		Description:     contract.Descriptors.Description,
		IsSpam:          util.FromPointer(contract.IsSpam),
	}
}

func mergeContractMetadata(lower contractMetadata, higher contractMetadata) contractMetadata {
	if higher.Symbol != "" {
		lower.Symbol = higher.Symbol
	}
	if higher.Name != "" && !unknownContractNames[strings.ToLower(higher.Name)] {
		lower.Name = higher.Name
	}
	if higher.OwnerAddress != "" {
		lower.OwnerAddress = higher.OwnerAddress
	}
	if higher.Description != "" {
		lower.Description = higher.Description
	}
	if higher.ProfileImageURL != "" {
		lower.ProfileImageURL = higher.ProfileImageURL
	}
	if higher.IsSpam {
		// only one provider needs to mark it as spam for it to be spam
		lower.IsSpam = true
	}

	return lower
}

func dedupeWallets(wallets []persist.Wallet) []persist.Wallet {
	return util.DedupeWithTranslate(wallets, false, func(w persist.Wallet) string {
		return fmt.Sprintf("%d:%s", w.Chain, w.Address)
	})
}

func dedupeTokenDefinitions(tDefs []db.TokenDefinition) (uniqueDefs []db.TokenDefinition) {
	type Key struct {
		Val db.TokenDefinition
		ID  string
	}

	keys := util.MapWithoutError(tDefs, func(tDef db.TokenDefinition) Key {
		return Key{
			Val: tDef,
			ID:  fmt.Sprintf("%d:%s:%s", tDef.Chain, tDef.ContractID, tDef.TokenID),
		}
	})

	t := util.DedupeWithTranslate(keys, false, func(k Key) string { return k.ID })
	return util.MapWithoutError(t, func(k Key) db.TokenDefinition { return k.Val })
}

func dedupeTokenInstances(tokens []op.UpsertToken) (uniqueTokens []op.UpsertToken) {
	type Key struct {
		Val op.UpsertToken
		ID  string
	}

	keys := util.MapWithoutError(tokens, func(t op.UpsertToken) Key {
		return Key{
			Val: t,
			ID:  fmt.Sprintf("%d:%s:%s:%s", t.Identifiers.Chain, t.Token.ContractID, t.Identifiers.TokenID, t.Token.OwnerUserID),
		}
	})

	t := util.DedupeWithTranslate(keys, false, func(k Key) string { return k.ID })
	return util.MapWithoutError(t, func(k Key) op.UpsertToken { return k.Val })
}

// differenceTokens finds the difference of newState - oldState to get the new tokens
func differenceTokens(newState []persist.TokenIdentifiers, oldState []persist.TokenIdentifiers) map[persist.TokenIdentifiers]bool {
	old := make(map[persist.TokenIdentifiers]bool, len(oldState))
	for _, t := range oldState {
		old[t] = true
	}

	diff := make(map[persist.TokenIdentifiers]bool)
	for _, t := range newState {
		if !old[t] {
			diff[t] = true
		}
	}

	return diff
}
