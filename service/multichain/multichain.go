package multichain

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc"
	"github.com/sourcegraph/conc/pool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("TOKEN_PROCESSING_URL", "required")
}

const staleCommunityTime = time.Minute * 30

const maxCommunitySize = 10_000

var contractNameBlacklist = map[string]bool{
	"unidentified contract": true,
	"unknown contract":      true,
	"unknown":               true,
}

// SubmitUserTokensF is called to process a user's batch of tokens
type SubmitUserTokensF func(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, tokens []persist.TokenIdentifiers) error

type Provider struct {
	Repos   *postgres.Repositories
	Queries *db.Queries
	Cache   *redis.Cache
	Chains  map[persist.Chain][]any

	// some chains use the addresses of other chains, this will map of chain we want tokens from => chain that's address will be used for lookup
	WalletOverrides  WalletOverrideMap
	SubmitUserTokens SubmitUserTokensF
}

// BlockchainInfo retrieves blockchain info from all chains
type BlockchainInfo struct {
	Chain      persist.Chain `json:"chain_name"`
	ChainID    int           `json:"chain_id"`
	ProviderID string        `json:"provider_id"`
}

// ChainAgnosticToken is a token that is agnostic to the chain it is on
type ChainAgnosticToken struct {
	Descriptors ChainAgnosticTokenDescriptors `json:"descriptors"`

	TokenType persist.TokenType `json:"token_type"`

	TokenURI         persist.TokenURI              `json:"token_uri"`
	TokenID          persist.TokenID               `json:"token_id"`
	Quantity         persist.HexString             `json:"quantity"`
	OwnerAddress     persist.Address               `json:"owner_address"`
	OwnershipHistory []ChainAgnosticAddressAtBlock `json:"previous_owners"`
	TokenMetadata    persist.TokenMetadata         `json:"metadata"`
	ContractAddress  persist.Address               `json:"contract_address"`

	FallbackMedia persist.FallbackMedia `json:"fallback_media"`

	ExternalURL string `json:"external_url"`

	BlockNumber persist.BlockNumber `json:"block_number"`
	IsSpam      *bool               `json:"is_spam"`
}

// ChainAgnosticAddressAtBlock is an address at a block
type ChainAgnosticAddressAtBlock struct {
	Address persist.Address     `json:"address"`
	Block   persist.BlockNumber `json:"block"`
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Descriptors ChainAgnosticContractDescriptors `json:"descriptors"`
	Address     persist.Address                  `json:"address"`
	IsSpam      *bool                            `json:"is_spam"`

	LatestBlock persist.BlockNumber `json:"latest_block"`
}

type ChainAgnosticTokensAndContracts struct {
	Tokens    []ChainAgnosticToken    `json:"tokens"`
	Contracts []ChainAgnosticContract `json:"contracts"`
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
	CreatorAddress  persist.Address `json:"creator_address"`
}

// ChainAgnosticIdentifiers identify tokens despite their chain
type ChainAgnosticIdentifiers struct {
	ContractAddress persist.Address `json:"contract_address"`
	TokenID         persist.TokenID `json:"token_id"`
}

type ChainAgnosticCommunityOwner struct {
	Address persist.Address `json:"address"`
}

type TokenHolder struct {
	UserID        persist.DBID    `json:"user_id"`
	DisplayName   string          `json:"display_name"`
	Address       persist.Address `json:"address"`
	WalletIDs     []persist.DBID  `json:"wallet_ids"`
	PreviewTokens []string        `json:"preview_tokens"`
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

type errWithPriority struct {
	err      error
	priority int
}

// Configurer maintains provider settings
type Configurer interface {
	GetBlockchainInfo() BlockchainInfo
}

// NameResolver is able to resolve an address to a friendly display name
type NameResolver interface {
	GetDisplayNameByAddress(context.Context, persist.Address) string
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

type TokensContractFetcher interface {
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

type ContractsFetcher interface {
	GetContractByAddress(ctx context.Context, contract persist.Address) (ChainAgnosticContract, error)
}

type ContractsOwnerFetcher interface {
	GetContractsByOwnerAddress(ctx context.Context, owner persist.Address) ([]ChainAgnosticContract, error)
}

type ChildContractFetcher interface {
	GetChildContractsCreatedOnSharedContract(ctx context.Context, creatorAddress persist.Address) ([]ParentToChildEdge, error)
}

type OpenSeaChildContractFetcher interface {
	ChildContractFetcher
	IsOpenSea()
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

type ProviderSupplier interface {
	GetSubproviders() []any
}

type WalletOverrideMap = map[persist.Chain][]persist.Chain

// providersMatchingInterface returns providers that adhere to the given interface
func providersMatchingInterface[T any](providers []any) []T {
	matches := make([]T, 0)
	seen := map[string]bool{}
	for _, p := range providers {
		match, ok := p.(T)
		if !ok {
			continue
		}

		if id := p.(Configurer).GetBlockchainInfo().ProviderID; !seen[id] {
			seen[id] = true
			matches = append(matches, match)
		}

		// If the provider has subproviders, make sure we don't add them later
		if ps, ok := p.(ProviderSupplier); ok {
			for _, sp := range ps.GetSubproviders() {
				if id := sp.(Configurer).GetBlockchainInfo().ProviderID; !seen[id] {
					seen[id] = true
				}
			}
		}
	}
	return matches
}

// matchingProvidersByChains returns providers that adhere to the given interface by chain
func matchingProvidersByChains[T any](availableProviders map[persist.Chain][]any, requestedChains ...persist.Chain) map[persist.Chain][]T {
	matches := make(map[persist.Chain][]T, 0)
	for _, chain := range requestedChains {
		matching := providersMatchingInterface[T](availableProviders[chain])
		matches[chain] = matching
	}
	return matches
}

func matchingProvidersForChain[T any](availableProviders map[persist.Chain][]any, chain persist.Chain) []T {
	return matchingProvidersByChains[T](availableProviders, chain)[chain]
}

// SyncTokensByUserID updates the media for all tokens for a user
func (p *Provider) SyncTokensByUserID(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)
	chainsToAddresses := p.matchingWallets(user.Wallets, chains)

	wg := &conc.WaitGroup{}
	for c, a := range chainsToAddresses {
		logger.For(ctx).Infof("syncing chain %d tokens for user %s wallets %s", c, user.Username, a)
		chain := c
		addresses := a

		for _, addr := range addresses {
			addr := addr
			chain := chain
			wg.Go(func() {
				subWg := &conc.WaitGroup{}
				tokenFetchers := matchingProvidersForChain[TokensOwnerFetcher](p.Chains, chain)
				for i, p := range tokenFetchers {
					fetcher := p
					priority := i

					subWg.Go(func() {
						tokens, contracts, err := fetcher.GetTokensByWalletAddress(ctx, addr)
						if err != nil {
							errChan <- errWithPriority{err: err, priority: priority}
							return
						}

						incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
						incomingContracts <- chainContracts{chain: chain, contracts: contracts, priority: priority}
					})
				}
				subWg.Wait()
			})
		}
	}

	go func() {
		defer close(incomingTokens)
		defer close(incomingContracts)
		wg.Wait()
	}()

	_, err = p.receiveSyncedTokensForUser(ctx, user, chains, incomingTokens, incomingContracts, errChan, true)
	return err
}

type chainTokensAndContracts struct {
	tokens    chainTokens
	contracts chainContracts
}

// SyncTokensIncrementallyByUserID processes a user's tokens incrementally
func (p *Provider) SyncTokensIncrementallyByUserID(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	errChan := make(chan error)
	chainsToAddresses := p.matchingWallets(user.Wallets, chains)

	totalBuf := 0
	for c := range chainsToAddresses {
		totalBuf += len(matchingProvidersForChain[TokensIncrementalOwnerFetcher](p.Chains, c)) * 10
	}

	result := make(chan chainTokensAndContracts, totalBuf)

	walletWg := &conc.WaitGroup{}
	for c, a := range chainsToAddresses {
		logger.For(ctx).Infof("incrementally syncing chain %d tokens for user %s wallets %s", c, user.Username, a)
		chain := c
		addresses := a

		for _, addr := range addresses {
			addr := addr
			chain := chain
			walletWg.Go(func() {
				providerWg := &conc.WaitGroup{}
				tokenFetchers := matchingProvidersForChain[TokensIncrementalOwnerFetcher](p.Chains, chain)
				for i, p := range tokenFetchers {
					fetcher := p
					priority := i

					logger.For(ctx).Infof("incrementally fetching from provider %d (%T)", priority, fetcher)

					providerWg.Go(func() {

						inc, errs := fetcher.GetTokensIncrementallyByWalletAddress(ctx, addr)
					outer:
						for {
							select {
							case ts, ok := <-inc:
								if !ok {
									break outer
								}
								result <- chainTokensAndContracts{
									tokens:    chainTokens{chain: chain, tokens: ts.Tokens, priority: priority},
									contracts: chainContracts{chain: chain, contracts: ts.Contracts, priority: priority},
								}

							case err := <-errs:
								logger.For(ctx).Errorf("error while syncing tokens for user %s: %s (provider: %d (%T))", user.Username, err, priority, fetcher)
								errChan <- err
								return
							}
						}
					})
				}
				providerWg.Wait()
			})
		}
	}

	go func() {
		defer close(result)
		walletWg.Wait()
	}()

	return p.receiveSyncedTokensIncrementallyForUser(ctx, user, chains, result, errChan)
}

// SyncTokensByUserIDAndTokenIdentifiers updates the media for specific tokens for a user
func (p *Provider) SyncTokensByUserIDAndTokenIdentifiers(ctx context.Context, userID persist.DBID, tokenIdentifiers []persist.TokenUniqueIdentifiers) ([]persist.TokenGallery, error) {

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"tids": tokenIdentifiers, "user_id": userID})

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	chains, _ := util.Map(tokenIdentifiers, func(i persist.TokenUniqueIdentifiers) (persist.Chain, error) {
		return i.Chain, nil
	})

	chains = util.Dedupe(chains, false)

	matchingWallets := p.matchingWallets(user.Wallets, chains)

	chainAddresses := map[persist.ChainAddress]bool{}
	for chain, addresses := range matchingWallets {
		for _, address := range addresses {
			chainAddresses[persist.NewChainAddress(address, chain)] = true
		}
	}

	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)
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

	wg := &conc.WaitGroup{}
	for c, t := range chainsToTokenIdentifiers {
		logger.For(ctx).Infof("attempting to sync %d token(s) on chain=%d for user %s", len(t), c, user.Username)
		chain := c
		tids := t
		tokenFetchers := matchingProvidersForChain[TokensOwnerFetcher](p.Chains, chain)
		wg.Go(func() {
			subWg := &conc.WaitGroup{}
			for i, p := range tokenFetchers {
				incomingAgnosticTokens := make(chan ChainAgnosticToken)
				incomingAgnosticContracts := make(chan ChainAgnosticContract)
				innerErrChan := make(chan error)
				tokens := make([]ChainAgnosticToken, 0, len(tids))
				contracts := make([]ChainAgnosticContract, 0, len(tids))
				fetcher := p
				priority := i
				for _, tid := range tids {
					tid := tid
					subWg.Go(func() {
						token, contract, err := fetcher.GetTokenByTokenIdentifiersAndOwner(ctx, ChainAgnosticIdentifiers{
							ContractAddress: tid.ContractAddress,
							TokenID:         tid.TokenID,
						}, tid.OwnerAddress)
						if err != nil {
							innerErrChan <- err
							return
						}
						incomingAgnosticTokens <- token
						incomingAgnosticContracts <- contract
					})
				}
				for i := 0; i < len(tids)*2; i++ {
					select {
					case token := <-incomingAgnosticTokens:
						tokens = append(tokens, token)
					case contract := <-incomingAgnosticContracts:
						contracts = append(contracts, contract)
					case err := <-innerErrChan:
						errChan <- errWithPriority{err: err, priority: priority}
						return
					}
				}
				incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
				incomingContracts <- chainContracts{chain: chain, contracts: contracts, priority: priority}
			}
			subWg.Wait()
		})

	}

	go func() {
		defer close(incomingTokens)
		defer close(incomingContracts)
		wg.Wait()
	}()

	return p.receiveSyncedTokensForUser(ctx, user, chains, incomingTokens, incomingContracts, errChan, false)
}

// TokenExists checks if a token exists according to any provider by its identifiers. It returns nil if the token exists.
// If a token exists, it will also update its contract and its descriptors in the database.
func (p *Provider) TokenExists(ctx context.Context, token persist.TokenIdentifiers, r retry.Retry) error {
	searchF := func(ctx context.Context) error {
		return p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, persist.TokenIdentifiers{
			TokenID:         token.TokenID,
			Chain:           token.Chain,
			ContractAddress: token.ContractAddress,
		})
	}

	retryCondition := func(err error) bool {
		logger.For(ctx).Errorf("polling for token: %s: retrying on error: %s", token.String(), err.Error())
		return true
	}

	return retry.RetryFunc(ctx, searchF, retryCondition, r)
}

// SyncTokenByUserWalletsAndTokenIdentifiersRetry attempts to sync a token for a user by their wallets and token identifiers.
func (p *Provider) SyncTokenByUserWalletsAndTokenIdentifiersRetry(ctx context.Context, user persist.User, t persist.TokenIdentifiers, r retry.Retry) (token db.Token, err error) {
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
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		// Try to sync the token from each wallet. This treats SyncTokensByUserIDAndTokenIdentifiers as a black box: it runs each
		// wallet in parallel and waits for each wallet to finish. We then check if a token exists in the database at the end and return
		// if it does. Otherwise, we retry until a token is found or the retry limit is reached.
		wg := sync.WaitGroup{}
		for _, w := range p.matchingWalletsChain(user.Wallets, token.Chain) {
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
		// Check if we got a token at the end of it
		token, err = p.Queries.GetTokenByUserTokenIdentifiers(ctx, db.GetTokenByUserTokenIdentifiersParams{
			OwnerID:         user.ID,
			TokenID:         t.TokenID,
			ContractAddress: t.ContractAddress,
			Chain:           t.Chain,
		})
		if err == pgx.ErrNoRows {
			return persist.ErrTokenNotFoundByUserTokenIdentifers{UserID: user.ID, Token: t}
		}
		return err
	}

	retryCondition := func(err error) bool {
		logger.For(ctx).Errorf("polling for token for user=%s: polling for token=%s: retrying on error: %s", user.ID, t.String(), err.Error())
		return true
	}

	err = retry.RetryFunc(ctx, searchF, retryCondition, r)

	return token, err
}

func (p *Provider) receiveSyncedTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, incomingTokens chan chainTokens, incomingContracts chan chainContracts, errChan chan error, replace bool) ([]persist.TokenGallery, error) {
	tokensFromProviders := make([]chainTokens, 0, len(user.Wallets))
	contractsFromProviders := make([]chainContracts, 0, len(user.Wallets))

	errs := []error{}
	discrepencyLog := map[int]int{}

outer:
	for {
		select {
		case incomingTokens := <-incomingTokens:
			discrepencyLog[incomingTokens.priority] = len(incomingTokens.tokens)
			tokensFromProviders = append(tokensFromProviders, incomingTokens)
		case incomingContracts, ok := <-incomingContracts:
			if !ok {
				break outer
			}
			contractsFromProviders = append(contractsFromProviders, incomingContracts)
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errChan:
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 && len(tokensFromProviders) == 0 {
		return nil, util.MultiErr(errs)
	}
	if !util.AllEqual(util.MapValues(discrepencyLog)) {
		logger.For(ctx).Debugf("discrepency: %+v", discrepencyLog)
	}

	_, persistedContracts, err := p.processContracts(ctx, contractsFromProviders, nil, false)
	if err != nil {
		return nil, err
	}

	currentTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, 0, 0)
	if err != nil {
		return nil, err
	}

	var newTokens []persist.TokenGallery
	if replace {
		_, newTokens, err = p.ReplaceHolderTokensForUser(ctx, user, tokensFromProviders, persistedContracts, chains, currentTokens)
	} else {
		_, newTokens, err = p.AddHolderTokensToUser(ctx, user, tokensFromProviders, persistedContracts, chains, currentTokens)
	}
	if err != nil {
		return nil, err
	}

	return newTokens, nil
}

func (p *Provider) receiveSyncedTokensIncrementallyForUser(ctx context.Context, user persist.User, chains []persist.Chain, result <-chan chainTokensAndContracts, errChan chan error) error {

	beginTime := time.Now()
	errs := []error{}
	currentTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, 0, 0)
	if err != nil {
		return err
	}
	currentContracts := util.MapWithoutError(currentTokens, func(t persist.TokenGallery) persist.ContractGallery { return t.Contract })
	currentContracts = util.DedupeWithTranslate(currentContracts, true, func(c persist.ContractGallery) persist.DBID { return c.ID })
	totalTokensReceived := 0
outer:
	for {
		select {
		case inc, ok := <-result:
			if !ok {
				break outer
			}
			currentContracts, _, err = p.processContracts(ctx, []chainContracts{inc.contracts}, currentContracts, false)
			if err != nil {
				return err
			}

			totalTokensReceived += len(inc.tokens.tokens)

			currentTokens, _, err = p.AddHolderTokensToUser(ctx, user, []chainTokens{inc.tokens}, currentContracts, chains, currentTokens)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 && totalTokensReceived == 0 {
		return util.MultiErr(errs)
	}

	// once we have all the tokens, remove any tokens that are no longer owned by the user
	_, err = p.Queries.DeleteTokensBeforeTimestamp(ctx, db.DeleteTokensBeforeTimestampParams{
		RemoveHolderStatus:  true,
		RemoveCreatorStatus: false,
		OnlyFromUserID:      sql.NullString{String: user.ID.String(), Valid: true},
		OnlyFromContractIds: util.MapWithoutError(currentContracts, func(c persist.ContractGallery) string { return c.ID.String() }),
		OnlyFromChains:      util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) }),
		Timestamp:           beginTime,
	})

	return err
}

// SyncCreatedTokensForNewContracts syncs tokens for contracts that the user created but does not
// currently have any tokens for.
func (p *Provider) SyncCreatedTokensForNewContracts(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	err := p.SyncContractsOwnedByUser(ctx, userID, chains)
	if err != nil {
		return err
	}

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	chainInts := util.MapWithoutError(chains, func(c persist.Chain) int32 { return int32(c) })
	rows, err := p.Queries.GetCreatedContractsByUserID(ctx, db.GetCreatedContractsByUserIDParams{
		UserID:           userID,
		Chains:           chainInts,
		NewContractsOnly: true,
	})

	if err != nil {
		return err
	}

	contracts := util.MapWithoutError(rows, func(r db.GetCreatedContractsByUserIDRow) db.Contract { return r.Contract })

	// Sync the individual contracts in parallel, so contracts with a lot of tokens won't block
	// contracts with fewer tokens, and progress isn't all or nothing (because a subsequent sync will omit
	// any contract that finished syncing in this attempt)
	if len(contracts) > 0 {
		wp := pool.New().WithErrors().WithContext(ctx)
		for _, contract := range contracts {
			contract := contract
			wp.Go(func(ctx context.Context) error {
				return p.syncCreatedTokensForContract(ctx, user, contract)
			})
		}

		if err := wp.Wait(); err != nil {
			return err
		}
	}

	// Remove creator status from any tokens this user is no longer the creator of
	return p.Queries.RemoveStaleCreatorStatusFromTokens(ctx, userID)
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

	return p.syncCreatedTokensForContract(ctx, user, contract)
}

func (p *Provider) syncCreatedTokensForContract(ctx context.Context, user persist.User, contract db.Contract) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": user.ID, "contract_id": contract.ID})

	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)

	chain := contract.Chain
	address := contract.Address

	logger.For(ctx).Infof("syncing chain %d creator tokens for user %s contract %s", chain, user.Username, address)

	tokenFetchers := matchingProvidersForChain[TokensContractFetcher](p.Chains, chain)

	wg := &conc.WaitGroup{}
	for i, f := range tokenFetchers {
		priority := i
		fetcher := f

		wg.Go(func() {
			tokens, contract, err := fetcher.GetTokensByContractAddress(ctx, address, 0, 0)
			if err != nil {
				errChan <- errWithPriority{err: err, priority: priority}
				return
			}

			logger.For(ctx).Infof("got %d tokens for contract %s", len(tokens), address)

			incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
			incomingContracts <- chainContracts{chain: chain, contracts: []ChainAgnosticContract{contract}, priority: priority}
		})
	}

	go func() {
		defer close(incomingTokens)
		defer close(incomingContracts)
		wg.Wait()
	}()

	tokensFromProviders := make([]chainTokens, 0, 1)
	contractsFromProviders := make([]chainContracts, 0, 1)

	errs := []error{}
	discrepencyLog := map[int]int{}

outer:
	for {
		select {
		case incomingTokens := <-incomingTokens:
			discrepencyLog[incomingTokens.priority] = len(incomingTokens.tokens)
			tokensFromProviders = append(tokensFromProviders, incomingTokens)
		case incomingContracts, ok := <-incomingContracts:
			if !ok {
				break outer
			}
			contractsFromProviders = append(contractsFromProviders, incomingContracts)
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 && len(tokensFromProviders) == 0 {
		return util.MultiErr(errs)
	}
	if !util.AllEqual(util.MapValues(discrepencyLog)) {
		logger.For(ctx).Debugf("discrepency: %+v", discrepencyLog)
	}

	_, persistedContracts, err := p.processContracts(ctx, contractsFromProviders, nil, false)
	if err != nil {
		return err
	}

	_, _, err = p.ReplaceCreatorTokensOfContractsForUser(ctx, user, tokensFromProviders, persistedContracts)
	return err
}

type ProviderChildContractResult struct {
	Priority int
	Chain    persist.Chain
	Edges    []ParentToChildEdge
}

type ParentToChildEdge struct {
	Parent   ChainAgnosticContract // Providers may optionally provide the parent if its convenient to do so
	Children []ChildContract
}

type ChildContract struct {
	ChildID        string // Uniquely identifies a child contract within a parent contract
	Name           string
	Description    string
	OwnerAddress   persist.Address
	CreatorAddress persist.Address
	Tokens         []ChainAgnosticToken
}

// combinedProviderChildContractResults is a helper for combining results from multiple providers
type combinedProviderChildContractResults []ProviderChildContractResult

func (c combinedProviderChildContractResults) ParentContracts() []persist.ContractGallery {
	combined := make([]chainContracts, 0)
	for _, result := range c {
		contracts := make([]ChainAgnosticContract, 0)
		for _, edge := range result.Edges {
			contracts = append(contracts, edge.Parent)
		}
		combined = append(combined, chainContracts{
			priority:  result.Priority,
			chain:     result.Chain,
			contracts: contracts,
		})
	}
	return contractsToNewDedupedContracts(combined, nil)
}

// SyncTokensCreatedOnSharedContracts queries each provider to identify contracts created by the given user.
func (p *Provider) SyncTokensCreatedOnSharedContracts(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if len(chains) == 0 {
		for chain := range p.Chains {
			chains = append(chains, chain)
		}
	}

	fetchers := matchingProvidersByChains[ChildContractFetcher](p.Chains, chains...)
	searchAddresses := p.matchingWallets(user.Wallets, chains)
	providerPool := pool.NewWithResults[ProviderChildContractResult]().WithContext(ctx)

	// Fetch all tokens created by the user
	for chain, addresses := range searchAddresses {
		for priority, fetcher := range fetchers[chain] {
			for _, address := range addresses {
				c := chain
				p := priority
				f := fetcher
				a := address
				providerPool.Go(func(ctx context.Context) (ProviderChildContractResult, error) {
					contractEdges, err := f.GetChildContractsCreatedOnSharedContract(ctx, a)
					if err != nil {
						return ProviderChildContractResult{}, err
					}
					return ProviderChildContractResult{
						Priority: p,
						Chain:    c,
						Edges:    contractEdges,
					}, nil
				})
			}
		}
	}

	pResult, err := providerPool.Wait()
	if err != nil {
		return err
	}

	combinedResult := combinedProviderChildContractResults(pResult)

	parentContracts, err := p.Repos.ContractRepository.BulkUpsert(ctx, combinedResult.ParentContracts(), true)
	if err != nil {
		return err
	}

	contractToDBID := make(map[persist.ContractIdentifiers]persist.DBID)
	for _, c := range parentContracts {
		contractToDBID[c.ContractIdentifiers()] = c.ID
	}

	params := db.UpsertChildContractsParams{}

	for _, result := range combinedResult {
		for _, edge := range result.Edges {
			for _, child := range edge.Children {
				params.ID = append(params.ID, persist.GenerateID().String())
				params.Name = append(params.Name, child.Name)
				params.Address = append(params.Address, child.ChildID)
				params.CreatorAddress = append(params.CreatorAddress, child.CreatorAddress.String())
				params.OwnerAddress = append(params.OwnerAddress, child.OwnerAddress.String())
				params.Chain = append(params.Chain, int32(result.Chain))
				params.Description = append(params.Description, child.Description)
				params.ParentIds = append(params.ParentIds, contractToDBID[persist.NewContractIdentifiers(edge.Parent.Address, result.Chain)].String())
			}
		}
	}

	_, err = p.Queries.UpsertChildContracts(ctx, params)
	return err
}

func (p *Provider) prepTokensForTokenProcessing(ctx context.Context, tokensFromProviders []chainTokens, existingTokens []persist.TokenGallery, contracts []persist.ContractGallery, user persist.User) ([]persist.TokenGallery, map[persist.TokenIdentifiers]bool, error) {
	providerTokens := tokensToNewDedupedTokens(tokensFromProviders, existingTokens, contracts, user)

	tokenLookup := make(map[persist.TokenIdentifiers]persist.TokenGallery)
	for _, token := range existingTokens {
		tokenLookup[token.TokenIdentifiers()] = token
	}

	newTokens := make(map[persist.TokenIdentifiers]bool)

	for _, token := range providerTokens {
		existingToken, exists := tokenLookup[token.TokenIdentifiers()]

		if !exists || existingToken.TokenMediaID == "" {
			newTokens[token.TokenIdentifiers()] = true
		}
	}

	return providerTokens, newTokens, nil
}

func (p *Provider) processTokensForUsers(ctx context.Context, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens,
	existingTokensForUsers map[persist.DBID][]persist.TokenGallery, contracts []persist.ContractGallery, chains []persist.Chain,
	upsertParams postgres.TokenUpsertParams) (currentUserTokens map[persist.DBID][]persist.TokenGallery, newUserTokens map[persist.DBID][]persist.TokenGallery, err error) {

	tokensToUpsert := make([]persist.TokenGallery, 0, len(chainTokensForUsers)*3)
	tokenIsNewForUser := make(map[persist.DBID]map[persist.TokenIdentifiers]bool)

	for userID, user := range users {
		tokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, chainTokensForUsers[userID], existingTokensForUsers[userID], contracts, user)
		if err != nil {
			return nil, nil, err
		}

		tokensToUpsert = append(tokensToUpsert, tokens...)
		tokenIsNewForUser[userID] = newTokens
	}

	upsertTime, persistedTokens, err := p.Repos.TokenRepository.UpsertTokens(ctx, tokensToUpsert, upsertParams.SetCreatorFields, upsertParams.SetHolderFields)
	if err != nil {
		return nil, nil, err
	}

	currentUserTokens = make(map[persist.DBID][]persist.TokenGallery)
	for _, token := range persistedTokens {
		currentUserTokens[token.OwnerUserID] = append(currentUserTokens[token.OwnerUserID], token)
	}

	if upsertParams.OptionalDelete != nil {
		numAffectedRows, err := p.Queries.DeleteTokensBeforeTimestamp(ctx, upsertParams.OptionalDelete.ToParams(upsertTime))

		if err != nil {
			return nil, nil, fmt.Errorf("failed to delete tokens: %w", err)
		}

		logger.For(ctx).Infof("deleted %d tokens", numAffectedRows)
	}

	newUserTokens = make(map[persist.DBID][]persist.TokenGallery)

	errors := make([]error, 0)
	for userID := range users {
		newTokensForUser := tokenIsNewForUser[userID]
		currentTokensForUser := currentUserTokens[userID]

		newPersistedTokens := make([]persist.TokenGallery, 0, len(currentTokensForUser))
		newPersistedTokenIDs := make([]persist.DBID, 0, len(currentTokensForUser))
		newPersistedTokenIdentifiers := make([]persist.TokenIdentifiers, 0, len(currentTokensForUser))

		for _, token := range currentTokensForUser {
			if newTokensForUser[token.TokenIdentifiers()] {
				newPersistedTokens = append(newPersistedTokens, token)
				newPersistedTokenIDs = append(newPersistedTokenIDs, token.ID)
				newPersistedTokenIdentifiers = append(newPersistedTokenIdentifiers, token.TokenIdentifiers())
			}
		}

		newUserTokens[userID] = newPersistedTokens

		err = p.SubmitUserTokens(ctx, userID, newPersistedTokenIDs, newPersistedTokenIdentifiers)
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 1 {
		return nil, nil, errors[0]
	}

	for userID := range users {
		// include the existing tokens that were not persisted with the bulk upsert
		currentUserTokens[userID] = util.DedupeWithTranslate(append(currentUserTokens[userID], existingTokensForUsers[userID]...), false, func(t persist.TokenGallery) persist.DBID { return t.ID })
	}

	return currentUserTokens, newUserTokens, nil
}

// ReplaceCreatorTokensOfContractsForUser will update a user's creator tokens for the given contracts, adding new
// tokens and removing creator status from tokens that the user is no longer the creator of. The removal step is
// scoped to the provided contracts, and tokens from other contracts will be unaffected.
func (p *Provider) ReplaceCreatorTokensOfContractsForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []persist.ContractGallery) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	contractIDs := util.MapWithoutError(contracts, func(contract persist.ContractGallery) persist.DBID { return contract.ID })
	chains := util.MapWithoutError(contracts, func(contract persist.ContractGallery) persist.Chain { return contract.Chain })
	chains = util.Dedupe(chains, true)

	existingTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, 0, 0)
	if err != nil {
		return nil, nil, err
	}

	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, chains, existingTokens, postgres.TokenUpsertParams{
		SetCreatorFields: true,
		SetHolderFields:  false,
		OptionalDelete: &postgres.TokenUpsertDeletionParams{
			DeleteCreatorStatus: true,
			DeleteHolderStatus:  false,
			OnlyFromUserID:      util.ToPointer(user.ID),
			OnlyFromContracts:   contractIDs,
			OnlyFromChains:      chains,
		},
	})
}

// AddHolderTokensToUser will append to a user's existing holder tokens
func (p *Provider) AddHolderTokensToUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, chains []persist.Chain, existingTokens []persist.TokenGallery) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, chains, existingTokens, postgres.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete:   nil,
	})
}

// ReplaceHolderTokensForUser will replace a user's existing holder tokens with the new tokens
func (p *Provider) ReplaceHolderTokensForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, chains []persist.Chain, existingTokens []persist.TokenGallery) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, chains, existingTokens, postgres.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete: &postgres.TokenUpsertDeletionParams{
			DeleteCreatorStatus: false,
			DeleteHolderStatus:  true,
			OnlyFromUserID:      util.ToPointer(user.ID),
			OnlyFromContracts:   nil,
			OnlyFromChains:      chains,
		},
	})
}

func (p *Provider) processTokensForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, chains []persist.Chain, existingTokens []persist.TokenGallery, upsertParams postgres.TokenUpsertParams) (currentTokenState []persist.TokenGallery, newTokens []persist.TokenGallery, error error) {

	userMap := map[persist.DBID]persist.User{user.ID: user}
	providerTokenMap := map[persist.DBID][]chainTokens{user.ID: tokensFromProviders}
	existingTokenMap := map[persist.DBID][]persist.TokenGallery{user.ID: existingTokens}

	currentUserTokens, newUserTokens, err := p.processTokensForUsers(ctx, userMap, providerTokenMap, existingTokenMap, contracts, chains, upsertParams)
	if err != nil {
		return nil, nil, err
	}

	return currentUserTokens[user.ID], newUserTokens[user.ID], nil
}

func (p *Provider) processTokensForOwnersOfContract(ctx context.Context, contract persist.ContractGallery, users map[persist.DBID]persist.User,
	chainTokensForUsers map[persist.DBID][]chainTokens, upsertParams postgres.TokenUpsertParams) (map[persist.DBID][]persist.TokenGallery, map[persist.DBID][]persist.TokenGallery, error) {
	chains := []persist.Chain{contract.Chain}
	contracts := []persist.ContractGallery{contract}

	existingTokens, err := p.Repos.TokenRepository.GetByContractID(ctx, contract.ID)
	if err != nil {
		return nil, nil, err
	}

	existingTokensForUsers := make(map[persist.DBID][]persist.TokenGallery)
	for _, token := range existingTokens {
		existingTokensForUsers[token.OwnerUserID] = append(existingTokensForUsers[token.OwnerUserID], token)
	}

	return p.processTokensForUsers(ctx, users, chainTokensForUsers, existingTokensForUsers, contracts, chains, upsertParams)
}

func (p *Provider) processTokenMedia(ctx context.Context, tokenID persist.TokenID, contractAddress persist.Address, chain persist.Chain) error {
	input := map[string]any{
		"token_id":         tokenID,
		"contract_address": contractAddress,
		"chain":            chain,
	}
	asJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/media/process/token", env.GetString("TOKEN_PROCESSING_URL")), bytes.NewBuffer(asJSON))
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

	return nil
}

func (p *Provider) GetCommunityOwners(ctx context.Context, communityIdentifiers persist.ChainAddress, forceRefresh bool, limit, offset int) ([]TokenHolder, error) {

	cacheKey := fmt.Sprintf("%s-%d-%d", communityIdentifiers.String(), limit, offset)
	if !forceRefresh {
		bs, err := p.Cache.Get(ctx, cacheKey)
		if err == nil && len(bs) > 0 {
			var owners []TokenHolder
			err = json.Unmarshal(bs, &owners)
			if err != nil {
				return nil, err
			}
			return owners, nil
		}
	}

	dbHolders, err := p.Repos.ContractRepository.GetOwnersByAddress(ctx, communityIdentifiers.Address(), communityIdentifiers.Chain(), limit, offset)
	if err != nil {
		return nil, err
	}

	holders, err := tokenHoldersToTokenHolders(ctx, dbHolders, p.Repos.UserRepository)
	if err != nil {
		return nil, err
	}

	bs, err := json.Marshal(holders)
	if err != nil {
		return nil, err
	}
	err = p.Cache.Set(ctx, cacheKey, bs, staleCommunityTime)
	if err != nil {
		return nil, err
	}
	return holders, nil
}

func (p *Provider) GetTokensOfContractForWallet(ctx context.Context, contractAddress persist.Address, wallet persist.ChainAddress, limit, offset int) ([]persist.TokenGallery, error) {
	user, err := p.Repos.UserRepository.GetByChainAddress(ctx, wallet)
	if err != nil {
		if _, ok := err.(persist.ErrWalletNotFound); ok {
			return nil, nil
		}
		return nil, err
	}

	contractFetchers := matchingProvidersForChain[TokensContractFetcher](p.Chains, wallet.Chain())

	tokensFromProviders := make([]chainTokens, 0, len(contractFetchers))
	contracts := make([]chainContracts, 0, len(contractFetchers))
	for i, tFetcher := range contractFetchers {
		tokensOfOwner, contract, err := tFetcher.GetTokensByContractAddressAndOwner(ctx, wallet.Address(), contractAddress, limit, offset)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, chainContracts{
			priority:  i,
			chain:     wallet.Chain(),
			contracts: []ChainAgnosticContract{contract},
		})

		tokensFromProviders = append(tokensFromProviders, chainTokens{
			priority: i,
			chain:    wallet.Chain(),
			tokens:   tokensOfOwner,
		})
	}

	_, persistedContracts, err := p.processContracts(ctx, contracts, nil, false)
	if err != nil {
		return nil, err
	}

	existingTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, 0, 0)
	if err != nil {
		return nil, err
	}

	allUserTokens, _, err := p.AddHolderTokensToUser(ctx, user, tokensFromProviders, persistedContracts, []persist.Chain{wallet.Chain()}, existingTokens)
	if err != nil {
		return nil, err
	}

	persistedContractIDs := map[persist.DBID]bool{}
	for _, contract := range persistedContracts {
		persistedContractIDs[contract.ID] = true
	}
	return util.Filter(allUserTokens, func(t persist.TokenGallery) bool {
		return persistedContractIDs[t.Contract.ID]
	}, true), nil
}

type FieldRequirementLevel int

const (
	FieldRequirementLevelNone FieldRequirementLevel = iota
	FieldRequirementLevelAllOptional
	FieldRequirementLevelAllRequired
	FieldRequirementLevelOneRequired
)

type FieldRequest[T any] struct {
	FieldNames []string
	Level      FieldRequirementLevel
}

func (f FieldRequest[T]) MatchesFilter(filter persist.TokenMetadata) bool {
	switch f.Level {
	case FieldRequirementLevelAllRequired:
		for _, fieldName := range f.FieldNames {
			it, ok := util.GetValueFromMapUnsafe(filter, fieldName, util.DefaultSearchDepth).(T)
			if !ok || util.IsEmpty(&it) {
				return false
			}
		}
	case FieldRequirementLevelOneRequired:
		found := false
		for _, fieldName := range f.FieldNames {
			it, ok := util.GetValueFromMapUnsafe(filter, fieldName, util.DefaultSearchDepth).(T)
			if ok && !util.IsEmpty(&it) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	case FieldRequirementLevelAllOptional:
		hasNone := true
		for _, fieldName := range f.FieldNames {
			it, ok := util.GetValueFromMapUnsafe(filter, fieldName, util.DefaultSearchDepth).(T)
			if ok && !util.IsEmpty(&it) {
				hasNone = false
				break
			}
		}
		if hasNone {
			return false
		}
	}

	return true
}

type MetadataResult struct {
	Priority int
	Metadata persist.TokenMetadata
}

// GetTokenMetadataByTokenIdentifiers will get the metadata for a given token identifier
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, chain persist.Chain, requestedFields []FieldRequest[string]) (persist.TokenMetadata, error) {

	metadataFetchers := matchingProvidersForChain[TokenMetadataFetcher](p.Chains, chain)

	if len(metadataFetchers) == 0 {
		return nil, fmt.Errorf("no metadata fetchers for chain %d", chain)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wp := pool.New().WithMaxGoroutines(len(metadataFetchers)).WithContext(ctx)
	metadatas := make(chan MetadataResult)
	for i, metadataFetcher := range metadataFetchers {
		i := i
		metadataFetcher := metadataFetcher
		wp.Go(func(ctx context.Context) error {
			metadata, err := metadataFetcher.GetTokenMetadataByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{ContractAddress: contractAddress, TokenID: tokenID})
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					logger.For(ctx).Warnf("error fetching token metadata from provider %d (%T): %s", i, metadataFetcher, err)
				}
				switch caught := err.(type) {
				case util.ErrHTTP:
					if caught.Status == http.StatusNotFound {
						return err
					}
				}
			}
			metadatas <- MetadataResult{Priority: i, Metadata: metadata}
			return nil
		})
	}

	go func() {
		defer close(metadatas)
		err := wp.Wait()
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.For(ctx).Warnf("error fetching token metadata after wait: %s", err)
		}
	}()

	prioritiesEncountered := []int{}

	var betterThanNothing persist.TokenMetadata
	var result MetadataResult
metadatas:
	for metadata := range metadatas {
		if metadata.Metadata != nil {
			betterThanNothing = metadata.Metadata

			for _, fieldRequest := range requestedFields {
				if !fieldRequest.MatchesFilter(metadata.Metadata) {
					logger.For(ctx).Infof("metadata %+v does not match field request %+v", metadata, fieldRequest)
					prioritiesEncountered = append(prioritiesEncountered, metadata.Priority)
					continue metadatas
				}
			}
			logger.For(ctx).Infof("got metadata %+v", metadata)
			if lowestIntNotInList(prioritiesEncountered, len(metadataFetchers)) == metadata.Priority {
				// short circuit if we've found the highest priority metadata
				return metadata.Metadata, nil
			}
			prioritiesEncountered = append(prioritiesEncountered, metadata.Priority)

			if result.Metadata == nil || metadata.Priority < result.Priority {
				result = metadata
			}
		}
	}
	if result.Metadata != nil {
		return result.Metadata, nil
	}

	if betterThanNothing != nil {
		return betterThanNothing, nil
	}

	return nil, fmt.Errorf("no metadata found for token %s-%s-%d", tokenID, contractAddress, chain)
}

// given a list of ints and a max, return the lowest int not in the list or max if all are in the list
// for example, if the list is [0,1,3] and max is 4, return 2 or if the list is [1,2] and max is 4, return 0
func lowestIntNotInList(list []int, max int) int {
	sort.Ints(list)
	for i := 0; i < max; i++ {
		if !util.Contains(list, i) {
			return i
		}
	}
	return max
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	verifiers := matchingProvidersForChain[Verifier](p.Chains, pChainAddress.Chain())
	for _, verifier := range verifiers {
		if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (p *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers) error {
	err := p.processTokenMedia(ctx, ti.TokenID, ti.ContractAddress, ti.Chain)
	if err != nil {
		return err
	}
	return p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, ti)
}

// RefreshTokenDescriptorsByTokenIdentifiers will refresh the token descriptors for a token by its identifiers.
func (p *Provider) RefreshTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) error {
	finalTokenDescriptors := ChainAgnosticTokenDescriptors{}
	finalContractDescriptors := ChainAgnosticContractDescriptors{}
	tokenFetchers := matchingProvidersForChain[TokenDescriptorsFetcher](p.Chains, ti.Chain)
	tokenExists := false

	for _, tokenFetcher := range tokenFetchers {
		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}

		token, contract, err := tokenFetcher.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
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
			if (contract.Name != "" && !contractNameBlacklist[strings.ToLower(contract.Name)]) && finalContractDescriptors.Name == "" {
				finalContractDescriptors.Name = contract.Name
			}
			if contract.Description != "" && finalContractDescriptors.Description == "" {
				finalContractDescriptors.Description = contract.Description
			}
			if contract.Symbol != "" && finalContractDescriptors.Symbol == "" {
				finalContractDescriptors.Symbol = contract.Symbol
			}
			if contract.CreatorAddress != "" && finalContractDescriptors.CreatorAddress == "" {
				finalContractDescriptors.CreatorAddress = contract.CreatorAddress
			}
			if contract.ProfileImageURL != "" && finalContractDescriptors.ProfileImageURL == "" {
				finalContractDescriptors.ProfileImageURL = contract.ProfileImageURL
			}
		} else {
			logger.For(ctx).Infof("token %s-%s-%d not found for refresh (err: %s)", ti.TokenID, ti.ContractAddress, ti.Chain, err)
		}
	}

	if !tokenExists {
		return persist.ErrTokenNotFoundByTokenIdentifiers{Token: ti}
	}

	contractID, err := p.Repos.ContractRepository.UpsertByAddress(ctx, ti.ContractAddress, ti.Chain, persist.ContractGallery{
		Chain:           ti.Chain,
		Address:         persist.Address(ti.Chain.NormalizeAddress(ti.ContractAddress)),
		Symbol:          persist.NullString(finalContractDescriptors.Symbol),
		Name:            persist.NullString(finalContractDescriptors.Name),
		Description:     persist.NullString(finalContractDescriptors.Description),
		ProfileImageURL: persist.NullString(finalContractDescriptors.ProfileImageURL),
		OwnerAddress:    finalContractDescriptors.CreatorAddress,
	})
	if err != nil {
		return err
	}

	return p.Queries.UpdateTokenMetadataFieldsByTokenIdentifiers(ctx, db.UpdateTokenMetadataFieldsByTokenIdentifiersParams{
		Name:        util.ToNullString(finalTokenDescriptors.Name, true),
		Description: util.ToNullString(finalTokenDescriptors.Description, true),
		TokenID:     ti.TokenID,
		ContractID:  contractID,
		Chain:       ti.Chain,
	})
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	contractRefreshers := matchingProvidersForChain[ContractRefresher](p.Chains, ci.Chain)
	contractFetchers := matchingProvidersForChain[ContractsFetcher](p.Chains, ci.Chain)
	var contracts []chainContracts
	for _, refresher := range contractRefreshers {
		if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	for i, fetcher := range contractFetchers {
		c, err := fetcher.GetContractByAddress(ctx, ci.ContractAddress)
		if err != nil {
			return err
		}
		contracts = append(contracts, chainContracts{priority: i, chain: ci.Chain, contracts: []ChainAgnosticContract{c}})
	}

	_, _, err := p.processContracts(ctx, contracts, nil, false)
	return err
}

// RefreshTokensForContract refreshes all tokens in a given contract
func (p *Provider) RefreshTokensForContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	contractRefreshers := matchingProvidersForChain[TokensContractFetcher](p.Chains, ci.Chain)

	tokensFromProviders := []chainTokens{}
	contractsFromProviders := []chainContracts{}
	tokensReceive := make(chan chainTokens)
	contractsReceive := make(chan chainContracts)
	errChan := make(chan errWithPriority)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	for i, fetcher := range contractRefreshers {
		wg.Add(1)
		go func(priority int, p TokensContractFetcher) {
			defer wg.Done()
			tokens, contract, err := p.GetTokensByContractAddress(ctx, ci.ContractAddress, maxCommunitySize, 0)
			if err != nil {
				errChan <- errWithPriority{priority: priority, err: err}
				return
			}
			tokensReceive <- chainTokens{chain: ci.Chain, tokens: tokens, priority: priority}
			contractsReceive <- chainContracts{chain: ci.Chain, contracts: []ChainAgnosticContract{contract}, priority: priority}
		}(i, fetcher)
	}
	go func() {
		defer close(done)
		wg.Wait()
	}()

outer:
	for {
		select {
		case err := <-errChan:
			logger.For(ctx).Errorf("error fetching tokens for contract %s-%d: %s", ci.ContractAddress, ci.Chain, err)
			if err.priority == 0 {
				return err
			}
		case tokens := <-tokensReceive:
			tokensFromProviders = append(tokensFromProviders, tokens)
		case contract := <-contractsReceive:
			contractsFromProviders = append(contractsFromProviders, contract)
		case <-done:
			logger.For(ctx).Debug("done refreshing tokens for collection")
			break outer
		}
	}

	logger.For(ctx).Debug("creating users")

	chainTokensForUsers, users, err := p.createUsersForTokens(ctx, tokensFromProviders, ci.Chain)
	if err != nil {
		return err
	}

	logger.For(ctx).Debug("creating contracts")

	_, persistedContracts, err := p.processContracts(ctx, contractsFromProviders, nil, false)
	if err != nil {
		return err
	}

	// We should receive exactly one contract from processContracts
	if len(persistedContracts) != 1 {
		return fmt.Errorf("expected one contract to be returned from processContracts, got %d", len(persistedContracts))
	}

	_, _, err = p.processTokensForOwnersOfContract(ctx, persistedContracts[0], users, chainTokensForUsers, postgres.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete:   nil,
	})

	return err
}

type ContractOwnerResult struct {
	Priority  int
	Contracts []ChainAgnosticContract
	Chain     persist.Chain
}

func (p *Provider) SyncContractsOwnedByUser(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if len(chains) == 0 {
		for chain := range p.Chains {
			chains = append(chains, chain)
		}
	}
	contractsFromProviders := []chainContracts{}

	contractFetchers := matchingProvidersByChains[ContractsOwnerFetcher](p.Chains, chains...)
	searchAddresses := p.matchingWallets(user.Wallets, chains)
	providerPool := pool.NewWithResults[ContractOwnerResult]().WithContext(ctx)

	for chain, addresses := range searchAddresses {
		for priority, fetcher := range contractFetchers[chain] {
			for _, address := range addresses {

				c := chain
				pr := priority
				f := fetcher
				a := address
				providerPool.Go(func(ctx context.Context) (ContractOwnerResult, error) {

					contracts, err := f.GetContractsByOwnerAddress(ctx, a)
					if err != nil {
						return ContractOwnerResult{Priority: pr}, err
					}

					logger.For(ctx).Debugf("found %d contracts for address %s", len(contracts), a)

					return ContractOwnerResult{Contracts: contracts, Chain: c, Priority: pr}, nil
				})
			}
		}
	}

	pResult, err := providerPool.Wait()
	if err != nil {
		return err
	}

	for _, result := range pResult {
		contractsFromProviders = append(contractsFromProviders, chainContracts{chain: result.Chain, contracts: result.Contracts, priority: result.Priority})
	}

	_, _, err = p.processContracts(ctx, contractsFromProviders, nil, true)
	if err != nil {
		return err
	}

	return nil
	//return p.SyncTokensCreatedOnSharedContracts(ctx, userID, chains)

}

type tokenForUser struct {
	userID   persist.DBID
	token    ChainAgnosticToken
	chain    persist.Chain
	priority int
}

// this function returns a map of user IDs to their new tokens as well as a map of user IDs to the users themselves
func (p *Provider) createUsersForTokens(ctx context.Context, tokens []chainTokens, chain persist.Chain) (map[persist.DBID][]chainTokens, map[persist.DBID]persist.User, error) {
	users := map[persist.DBID]persist.User{}
	userTokens := map[persist.DBID]map[int]chainTokens{}
	seenTokens := map[persist.TokenUniqueIdentifiers]bool{}

	userChan := make(chan persist.User)
	tokensForUserChan := make(chan tokenForUser)
	errChan := make(chan error)
	done := make(chan struct{})
	wp := workerpool.New(100)

	mu := &sync.Mutex{}

	ownerAddresses := make([]string, 0, len(tokens))

	for _, chainToken := range tokens {
		for _, token := range chainToken.tokens {
			ownerAddresses = append(ownerAddresses, token.OwnerAddress.String())
		}
	}

	// get all current users

	allCurrentUsers, err := p.Queries.GetUsersByChainAddresses(ctx, db.GetUsersByChainAddressesParams{
		Addresses: ownerAddresses,
		Chain:     int32(chain),
	})
	if err != nil {
		return nil, nil, err
	}

	// figure out which users are not in the database

	addressesToUsers := map[string]persist.User{}

	for _, user := range allCurrentUsers {
		traits := persist.Traits{}
		err = user.Traits.AssignTo(&traits)
		if err != nil {
			return nil, nil, err
		}
		addressesToUsers[string(user.Address)] = persist.User{
			Version:            persist.NullInt32(user.Version.Int32),
			ID:                 user.ID,
			CreationTime:       user.CreatedAt,
			Deleted:            persist.NullBool(user.Deleted),
			LastUpdated:        user.LastUpdated,
			Username:           persist.NullString(user.Username.String),
			UsernameIdempotent: persist.NullString(user.UsernameIdempotent.String),
			Wallets:            user.Wallets,
			Bio:                persist.NullString(user.Bio.String),
			Traits:             traits,
			Universal:          persist.NullBool(user.Universal),
		}
	}

	logger.For(ctx).Debugf("found %d users", len(addressesToUsers))

	// create users for those that are not in the database

	for _, chainToken := range tokens {
		resolvers := matchingProvidersForChain[NameResolver](p.Chains, chainToken.chain)

		for _, agnosticToken := range chainToken.tokens {
			if agnosticToken.OwnerAddress == "" {
				continue
			}
			tid := persist.TokenUniqueIdentifiers{Chain: chainToken.chain, ContractAddress: agnosticToken.ContractAddress, TokenID: agnosticToken.TokenID, OwnerAddress: agnosticToken.OwnerAddress}
			if seenTokens[tid] {
				continue
			}
			seenTokens[tid] = true
			ct := chainToken
			t := agnosticToken
			wp.Submit(func() {
				user, ok := addressesToUsers[string(t.OwnerAddress)]
				if !ok {
					username := t.OwnerAddress.String()
					for _, resolver := range resolvers {
						doBreak := func() bool {
							displayCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
							defer cancel()
							display := resolver.GetDisplayNameByAddress(displayCtx, t.OwnerAddress)

							// if display is an empty address, this will return empty
							displayCopy := strings.TrimLeft(display, "0x")
							displayCopy = strings.TrimRight(displayCopy, "0")
							if displayCopy != "" {
								username = display
								return true
							}
							return false
						}()
						if doBreak {
							break
						}
					}
					func() {
						mu.Lock()
						defer mu.Unlock()
						userID, err := p.Repos.UserRepository.Create(ctx, persist.CreateUserInput{
							Username:     username,
							ChainAddress: persist.NewChainAddress(t.OwnerAddress, ct.chain),
							Universal:    true,
						}, nil)
						if err != nil {
							if _, ok := err.(persist.ErrUsernameNotAvailable); ok {
								logger.For(ctx).Infof("username %s not available", username)
								user, err = p.Repos.UserRepository.GetByUsername(ctx, username)
								if err != nil {
									errChan <- err
									return
								}
							} else if _, ok := err.(persist.ErrAddressOwnedByUser); ok {
								logger.For(ctx).Infof("address %s already owned by user", t.OwnerAddress)
								user, err = p.Repos.UserRepository.GetByChainAddress(ctx, persist.NewChainAddress(t.OwnerAddress, ct.chain))
								if err != nil {
									errChan <- err
									return
								}
							} else if _, ok := err.(persist.ErrWalletCreateFailed); ok {
								logger.For(ctx).Infof("wallet creation failed for address %s", t.OwnerAddress)
								user, err = p.Repos.UserRepository.GetByChainAddress(ctx, persist.NewChainAddress(t.OwnerAddress, ct.chain))
								if err != nil {
									errChan <- err
									return
								}
							} else {
								errChan <- err
								return
							}
						} else {
							user, err = p.Repos.UserRepository.GetByID(ctx, userID)
							if err != nil {
								errChan <- err
								return
							}
						}
					}()
				}

				err = p.Repos.UserRepository.FillWalletDataForUser(ctx, &user)
				if err != nil {
					errChan <- err
					return
				}
				userChan <- user
				tokensForUserChan <- tokenForUser{
					userID:   user.ID,
					token:    t,
					chain:    ct.chain,
					priority: ct.priority,
				}
			})
		}
	}

	go func() {
		defer close(done)
		wp.StopWait()
	}()

outer:
	for {
		select {
		case user := <-userChan:
			logger.For(ctx).Debugf("got user %s", user.Username)
			users[user.ID] = user
			if userTokens[user.ID] == nil {
				userTokens[user.ID] = map[int]chainTokens{}
			}
		case token := <-tokensForUserChan:
			chainTokensForUser := userTokens[token.userID]
			tokensInChainTokens, ok := chainTokensForUser[token.priority]
			if !ok {
				tokensInChainTokens = chainTokens{chain: token.chain, tokens: []ChainAgnosticToken{}, priority: token.priority}
			}
			tokensInChainTokens.tokens = append(tokensInChainTokens.tokens, token.token)
			chainTokensForUser[token.priority] = tokensInChainTokens
			userTokens[token.userID] = chainTokensForUser
		case err := <-errChan:
			return nil, nil, err
		case <-done:
			break outer
		}
	}

	chainTokensForUser := map[persist.DBID][]chainTokens{}
	for userID, chainTokens := range userTokens {
		for _, chainToken := range chainTokens {
			chainTokensForUser[userID] = append(chainTokensForUser[userID], chainToken)
		}
	}

	logger.For(ctx).Infof("created %d users for tokens", len(users))
	return chainTokensForUser, users, nil
}

// matchingWallets returns wallet addresses that belong to any of the passed chains
func (p *Provider) matchingWallets(wallets []persist.Wallet, chains []persist.Chain) map[persist.Chain][]persist.Address {
	matches := make(map[persist.Chain][]persist.Address)
	for _, chain := range chains {
		for _, wallet := range wallets {
			if wallet.Chain == chain {
				matches[chain] = append(matches[chain], wallet.Address)
			} else if overrides, ok := p.WalletOverrides[chain]; ok && util.Contains(overrides, wallet.Chain) {
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

// processContracts deduplicates contracts and upserts them into the database. If canOverwriteOwnerAddress is true, then
// the owner address of an existing contract will be overwritten if the new contract provides a non-empty owner address.
// An empty owner address will never overwrite an existing address, even if canOverwriteOwnerAddress is true.
func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts, existingContracts []persist.ContractGallery, canOverwriteOwnerAddress bool) (currentContractState []persist.ContractGallery, newContracts []persist.ContractGallery, err error) {
	contractsToUpsert := contractsToNewDedupedContracts(contractsFromProviders, existingContracts)
	newUpsertedContracts, err := d.Repos.ContractRepository.BulkUpsert(ctx, contractsToUpsert, canOverwriteOwnerAddress)
	if err != nil {
		return nil, nil, err
	}

	return util.DedupeWithTranslate(append(newUpsertedContracts, existingContracts...), false, func(c persist.ContractGallery) persist.DBID { return c.ID }), newUpsertedContracts, nil
}

func tokensToNewDedupedTokens(tokens []chainTokens, existingTokens []persist.TokenGallery, existingContracts []persist.ContractGallery, ownerUser persist.User) []persist.TokenGallery {
	addressToContract := make(map[string]persist.ContractGallery)

	util.Map(existingContracts, func(c persist.ContractGallery) (any, error) {
		addressToContract[c.Chain.NormalizeAddress(c.Address)] = c
		return nil, nil
	})

	seenTokens := make(map[persist.TokenIdentifiers]persist.TokenGallery)
	existingTokenLookup := make(map[persist.TokenIdentifiers]persist.TokenGallery)

	dbidToAddress := make(map[persist.DBID]persist.ChainAddress)
	util.Map(existingContracts, func(c persist.ContractGallery) (any, error) {
		dbidToAddress[c.ID] = persist.NewChainAddress(c.Address, c.Chain)
		return nil, nil
	})

	for _, token := range existingTokens {
		ca := dbidToAddress[token.Contract.ID]
		ti := persist.NewTokenIdentifiers(ca.Address(), token.TokenID, ca.Chain())
		existingTokenLookup[ti] = token
	}

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
			// TODO: Figure out the implication for L2 chains here. Might want a function like
			// Chain.IsCompatibleWith(Chain) to determine whether a wallet on one chain can claim
			// ownership of a contract on a different chain.
			createdContracts[contractAddress] = wallet.Chain == contract.Chain
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

			existingToken, existsAlready := existingTokenLookup[ti]

			// we add a higher priority token to the seen tokens as a sort of pre-seed when there isn't currently a higher priority token in the map
			// and the existing token from the DB is higher priority than the current token we are about to process
			if existsAlready && existingToken.Priority != nil && chainToken.priority >= *existingToken.Priority {
				if startingSeenToken, startingExists := seenTokens[ti]; !startingExists || startingSeenToken.Priority == nil || *startingSeenToken.Priority < *existingToken.Priority {
					seenTokens[ti] = existingToken
				}
			}
			// if there was in fact a higher priority DB token than the current token or any token in the map, than this DB token will be the baseline for comparing against the current token
			initialSeenToken, seen := seenTokens[ti]

			contractAddress := chainToken.chain.NormalizeAddress(token.ContractAddress)
			contract := addressToContract[contractAddress]

			candidateToken := persist.TokenGallery{
				TokenType:            token.TokenType,
				Chain:                chainToken.chain,
				Name:                 persist.NullString(token.Descriptors.Name),
				Description:          persist.NullString(token.Descriptors.Description),
				TokenURI:             "", // We don't save tokenURI information anymore
				TokenID:              token.TokenID,
				OwnerUserID:          ownerUser.ID,
				FallbackMedia:        token.FallbackMedia,
				TokenMetadata:        token.TokenMetadata,
				Contract:             contract,
				ExternalURL:          persist.NullString(token.ExternalURL),
				BlockNumber:          token.BlockNumber,
				IsProviderMarkedSpam: token.IsSpam,
				IsCreatorToken:       createdContracts[persist.Address(contractAddress)],
				Priority:             &chainToken.priority,
			}

			// If we've never seen the incoming token before, then add it.
			if !seen {
				seenTokens[ti] = candidateToken
			} else {
				seenTokens[ti] = mergeToken(initialSeenToken, candidateToken)
			}

			// if the token exists already in the DB, then we need to merge the token with the existing token
			// in the case that it washigher priority above, the "curSeenToken" will be the existing token so it will be merging against itself, essentially a no-op.
			// if the token was not a higher priority token, then we will still use it to merge to ensure that even lower priority tokens have their fields considered
			// when a higher priority token may have bad or empty data
			if existsAlready {
				curSeenToken := seenTokens[ti]
				seenTokens[ti] = mergeToken(curSeenToken, existingToken)
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
			ownership := fromMultichainToAddressAtBlock(token.OwnershipHistory)
			finalSeenToken.OwnershipHistory = ownership
			finalSeenToken.OwnedByWallets = seenWallets[ti]
			finalSeenToken.Quantity = seenQuantities[ti]
			seenTokens[ti] = finalSeenToken
		}
	}

	res := make([]persist.TokenGallery, len(seenTokens))
	i := 0
	for _, t := range seenTokens {
		if t.Name == "" {
			name, ok := util.GetValueFromMapUnsafe(t.TokenMetadata, "name", util.DefaultSearchDepth).(string)
			if ok {
				t.Name = persist.NullString(name)
			}
		}
		if t.Description == "" {
			description, ok := util.GetValueFromMapUnsafe(t.TokenMetadata, "description", util.DefaultSearchDepth).(string)
			if ok {
				t.Description = persist.NullString(description)
			}
		}

		res[i] = t
		i++
	}
	return res
}

func mergeToken(initial, candidate persist.TokenGallery) persist.TokenGallery {
	if len(initial.TokenMetadata) < len(candidate.TokenMetadata) {
		if initial.FallbackMedia.IsServable() && !candidate.FallbackMedia.IsServable() {
			candidate.FallbackMedia = initial.FallbackMedia
		}
		return candidate
	}
	if !initial.FallbackMedia.IsServable() && candidate.FallbackMedia.IsServable() {
		initial.FallbackMedia = candidate.FallbackMedia
	}
	return initial
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

func contractsToNewDedupedContracts(contracts []chainContracts, existingContracts []persist.ContractGallery) []persist.ContractGallery {

	contractMetadatas := map[persist.ChainAddress]contractMetadata{}
	existingMetadatas := map[persist.ChainAddress]contractMetadata{}

	for _, contract := range existingContracts {
		existingMetadatas[persist.NewChainAddress(contract.Address, contract.Chain)] = contractMetadata{
			Symbol:          contract.Symbol.String(),
			Name:            contract.Name.String(),
			OwnerAddress:    contract.OwnerAddress,
			ProfileImageURL: contract.ProfileImageURL.String(),
			Description:     contract.Description.String(),
			IsSpam:          contract.IsProviderMarkedSpam,
			priority:        contract.Priority,
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

	res := make([]persist.ContractGallery, 0, len(contractMetadatas))
	for address, meta := range contractMetadatas {
		res = append(res, persist.ContractGallery{
			Chain:                address.Chain(),
			Address:              address.Address(),
			Symbol:               persist.NullString(meta.Symbol),
			Name:                 persist.NullString(meta.Name),
			ProfileImageURL:      persist.NullString(meta.ProfileImageURL),
			OwnerAddress:         meta.OwnerAddress,
			Description:          persist.NullString(meta.Description),
			IsProviderMarkedSpam: meta.IsSpam,
		})
	}
	return res

}

func contractToMetadata(contract ChainAgnosticContract) contractMetadata {
	return contractMetadata{
		Symbol:          contract.Descriptors.Symbol,
		Name:            contract.Descriptors.Name,
		OwnerAddress:    contract.Descriptors.CreatorAddress,
		ProfileImageURL: contract.Descriptors.ProfileImageURL,
		Description:     contract.Descriptors.Description,
		IsSpam:          util.FromPointer(contract.IsSpam),
	}
}

func mergeContractMetadata(lower contractMetadata, higher contractMetadata) contractMetadata {
	if higher.Symbol != "" {
		lower.Symbol = higher.Symbol
	}
	if higher.Name != "" && !contractNameBlacklist[strings.ToLower(higher.Name)] {
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

func tokenHoldersToTokenHolders(ctx context.Context, owners []persist.TokenHolder, userRepo *postgres.UserRepository) ([]TokenHolder, error) {
	seenUsers := make(map[persist.DBID]persist.TokenHolder)
	allUserIDs := make([]persist.DBID, 0, len(owners))
	for _, owner := range owners {
		if _, ok := seenUsers[owner.UserID]; !ok {
			allUserIDs = append(allUserIDs, owner.UserID)
			seenUsers[owner.UserID] = owner
		}
	}
	allUsers, err := userRepo.GetByIDs(ctx, allUserIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get users for token holders: %s", err)
	}
	res := make([]TokenHolder, 0, len(seenUsers))
	for _, user := range allUsers {
		owner := seenUsers[user.ID]
		username := user.Username.String()
		previews := make([]string, 0, len(owner.PreviewTokens))
		for _, p := range owner.PreviewTokens {
			previews = append(previews, p.String())
		}
		res = append(res, TokenHolder{
			UserID:        owner.UserID,
			DisplayName:   username,
			WalletIDs:     owner.WalletIDs,
			PreviewTokens: previews,
		})
	}

	return res, nil
}

func fromMultichainToAddressAtBlock(addresses []ChainAgnosticAddressAtBlock) []persist.AddressAtBlock {
	res := make([]persist.AddressAtBlock, len(addresses))
	for i, addr := range addresses {
		res[i] = persist.AddressAtBlock{Address: addr.Address, Block: addr.Block}
	}
	return res
}

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("%s-%s", t.ContractAddress, t.TokenID)
}

func (e errWithPriority) Error() string {
	return fmt.Sprintf("error with priority %d: %s", e.priority, e.err)
}

func dedupeWallets(wallets []persist.Wallet) []persist.Wallet {
	deduped := map[persist.Address]persist.Wallet{}
	for _, wallet := range wallets {
		deduped[wallet.Address] = wallet
	}

	ret := make([]persist.Wallet, 0, len(wallets))
	for _, wallet := range deduped {
		ret = append(ret, wallet)
	}

	return ret
}
