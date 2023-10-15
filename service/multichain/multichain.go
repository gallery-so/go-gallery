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

// SubmitTokens is called to process a batch of tokens
type SubmitTokensF func(ctx context.Context, tokenDefinitionIDs []persist.DBID) error

type Provider struct {
	Repos   *postgres.Repositories
	Queries *db.Queries
	Cache   *redis.Cache
	Chains  map[persist.Chain][]any

	// some chains use the addresses of other chains, this will map of chain we want tokens from => chain that's address will be used for lookup
	WalletOverrides WalletOverrideMap
	SubmitTokens    SubmitTokensF
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

	_, err = p.receiveSyncedTokensForUser(ctx, user, chains, incomingTokens, incomingContracts, errChan, false)
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
func (p *Provider) SyncTokensByUserIDAndTokenIdentifiers(ctx context.Context, userID persist.DBID, tokenIdentifiers []persist.TokenUniqueIdentifiers) ([]postgres.TokenFullDetails, error) {

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
func (p *Provider) TokenExists(ctx context.Context, token persist.TokenIdentifiers, r retry.Retry) (td db.TokenDefinition, err error) {
	searchF := func(ctx context.Context) error {
		td, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, persist.TokenIdentifiers{
			TokenID:         token.TokenID,
			Chain:           token.Chain,
			ContractAddress: token.ContractAddress,
		})
		return err
	}

	retryCondition := func(err error) bool {
		logger.For(ctx).Errorf("polling for token: %s: retrying on error: %s", token.String(), err.Error())
		return true
	}

	retry.RetryFunc(ctx, searchF, retryCondition, r)

	return
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

func (p *Provider) receiveSyncedTokensForUser(ctx context.Context, user persist.User, chains []persist.Chain, incomingTokens chan chainTokens, incomingContracts chan chainContracts, errChan chan error, replace bool) ([]postgres.TokenFullDetails, error) {
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

	currentTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	var newTokens []postgres.TokenFullDetails
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
	currentTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID)
	if err != nil {
		return err
	}
	currentContracts := util.MapWithoutError(currentTokens, func(t postgres.TokenFullDetails) db.Contract { return t.Contract })
	currentContracts = util.DedupeWithTranslate(currentContracts, true, func(c db.Contract) persist.DBID { return c.ID })
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
		OnlyFromContractIds: util.MapWithoutError(currentContracts, func(c db.Contract) string { return c.ID.String() }),
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

func (c combinedProviderChildContractResults) ParentContracts() []db.Contract {
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
	return chainContractsToUpsertableContracts(combined, nil)
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
		contractIdentifiers := persist.NewContractIdentifiers(c.Address, c.Chain)
		contractToDBID[contractIdentifiers] = c.ID
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

func (p *Provider) processTokensForUsers(ctx context.Context, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens,
	existingTokensForUsers map[persist.DBID][]postgres.TokenFullDetails, contracts []db.Contract,
	upsertParams postgres.TokenUpsertParams) (currentUserTokens map[persist.DBID][]postgres.TokenFullDetails, newUserTokens map[persist.DBID][]postgres.TokenFullDetails, err error) {

	upsertableDefinitions := make([]db.TokenDefinition, 0)
	upsertableTokens := make([]db.Token, 0)

	for userID, user := range users {
		tokens := chainTokensToUpsertableTokens(chainTokensForUsers[userID], contracts, user)
		definitions := chainTokensToUpsertableTokenDefinitions(chainTokensForUsers[userID], contracts)
		upsertableTokens = append(upsertableTokens, tokens...)
		upsertableDefinitions = append(upsertableDefinitions, definitions...)
	}

	uniqueTokens := dedupeTokens(upsertableTokens)
	uniqueDefinitions := dedupeTokenDefinitions(upsertableDefinitions)

	upsertTime, upsertedTokens, err := p.Repos.TokenRepository.UpsertTokens(ctx, uniqueTokens, uniqueDefinitions, upsertParams.SetCreatorFields, upsertParams.SetHolderFields)
	if err != nil {
		return nil, nil, err
	}

	if upsertParams.OptionalDelete != nil {
		numAffectedRows, err := p.Queries.DeleteTokensBeforeTimestamp(ctx, upsertParams.OptionalDelete.ToParams(upsertTime))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to delete tokens: %w", err)
		}
		logger.For(ctx).Infof("deleted %d tokens", numAffectedRows)
	}

	// Create a lookup for userID to persisted token IDs
	currentUserTokens = make(map[persist.DBID][]postgres.TokenFullDetails)
	for _, token := range upsertedTokens {
		currentUserTokens[token.Instance.OwnerUserID] = append(currentUserTokens[token.Instance.OwnerUserID], token)
	}

	newUserTokens = make(map[persist.DBID][]postgres.TokenFullDetails, len(users))

	for userID := range users {
		currentTokensForUser := currentUserTokens[userID]
		newPersistedTokens := util.Filter(currentTokensForUser, func(t postgres.TokenFullDetails) bool { return t.Instance.CreatedAt.Equal(t.Instance.LastUpdated) }, false)
		newUserTokens[userID] = newPersistedTokens
	}

	for userID := range users {
		// include the existing tokens that were not persisted with the bulk upsert
		currentUserTokens[userID] = util.DedupeWithTranslate(append(currentUserTokens[userID], existingTokensForUsers[userID]...), false, func(t postgres.TokenFullDetails) persist.DBID { return t.Instance.ID })
	}

	// Submit tokens that are missing media IDs. Tokens that are missing media IDs are new tokens, or tokens that weren't processed for whatever reason.
	// This means we won't refresh tokens that have already been seen.
	definitionsToProcess := util.Filter(upsertedTokens, func(t postgres.TokenFullDetails) bool { return t.Media.ID == "" }, false)
	definitionIDs := util.MapWithoutError(definitionsToProcess, func(t postgres.TokenFullDetails) persist.DBID { return t.Definition.ID })
	err = p.SubmitTokens(ctx, definitionIDs)

	return currentUserTokens, newUserTokens, err
}

// ReplaceCreatorTokensOfContractsForUser will update a user's creator tokens for the given contracts, adding new
// tokens and removing creator status from tokens that the user is no longer the creator of. The removal step is
// scoped to the provided contracts, and tokens from other contracts will be unaffected.
func (p *Provider) ReplaceCreatorTokensOfContractsForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract) (currentTokenState []postgres.TokenFullDetails, newTokens []postgres.TokenFullDetails, err error) {
	contractIDs := util.MapWithoutError(contracts, func(contract db.Contract) persist.DBID { return contract.ID })
	chains := util.MapWithoutError(contracts, func(contract db.Contract) persist.Chain { return contract.Chain })
	chains = util.Dedupe(chains, true)

	existingTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}

	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, existingTokens, postgres.TokenUpsertParams{
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
func (p *Provider) AddHolderTokensToUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, chains []persist.Chain, existingTokens []postgres.TokenFullDetails) (currentTokenState []postgres.TokenFullDetails, newTokens []postgres.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, existingTokens, postgres.TokenUpsertParams{
		SetCreatorFields: false,
		SetHolderFields:  true,
		OptionalDelete:   nil,
	})
}

// ReplaceHolderTokensForUser will replace a user's existing holder tokens with the new tokens
func (p *Provider) ReplaceHolderTokensForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, chains []persist.Chain, existingTokens []postgres.TokenFullDetails) (currentTokenState []postgres.TokenFullDetails, newTokens []postgres.TokenFullDetails, err error) {
	return p.processTokensForUser(ctx, user, tokensFromProviders, contracts, existingTokens, postgres.TokenUpsertParams{
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

func (p *Provider) processTokensForUser(ctx context.Context, user persist.User, tokensFromProviders []chainTokens, contracts []db.Contract, existingTokens []postgres.TokenFullDetails, upsertParams postgres.TokenUpsertParams) (currentTokenState []postgres.TokenFullDetails, newTokens []postgres.TokenFullDetails, error error) {
	userMap := map[persist.DBID]persist.User{user.ID: user}
	providerTokenMap := map[persist.DBID][]chainTokens{user.ID: tokensFromProviders}
	existingTokenMap := map[persist.DBID][]postgres.TokenFullDetails{user.ID: existingTokens}

	currentUserTokens, newUserTokens, err := p.processTokensForUsers(ctx, userMap, providerTokenMap, existingTokenMap, contracts, upsertParams)
	if err != nil {
		return nil, nil, err
	}

	return currentUserTokens[user.ID], newUserTokens[user.ID], nil
}

func (p *Provider) processTokensForOwnersOfContract(ctx context.Context, contract db.Contract, users map[persist.DBID]persist.User,
	chainTokensForUsers map[persist.DBID][]chainTokens, upsertParams postgres.TokenUpsertParams) (map[persist.DBID][]postgres.TokenFullDetails, map[persist.DBID][]postgres.TokenFullDetails, error) {
	existingTokens, err := p.Repos.TokenRepository.GetByContractID(ctx, contract.ID)
	if err != nil {
		return nil, nil, err
	}

	existingTokensForUsers := make(map[persist.DBID][]postgres.TokenFullDetails)
	for _, t := range existingTokens {
		existingTokensForUsers[t.Instance.OwnerUserID] = append(existingTokensForUsers[t.Instance.OwnerUserID], t)
	}

	return p.processTokensForUsers(ctx, users, chainTokensForUsers, existingTokensForUsers, []db.Contract{contract}, upsertParams)
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

func (p *Provider) GetTokensOfContractForWallet(ctx context.Context, contractAddress persist.Address, wallet persist.ChainAddress, limit, offset int) ([]postgres.TokenFullDetails, error) {
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

	existingTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID)
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
	return util.Filter(allUserTokens, func(t postgres.TokenFullDetails) bool { return persistedContractIDs[t.Contract.ID] }, true), nil
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
	_, err = p.RefreshTokenDescriptorsByTokenIdentifiers(ctx, ti)
	return err
}

// RefreshTokenDescriptorsByTokenIdentifiers will refresh the token descriptors for a token by its identifiers.
func (p *Provider) RefreshTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) (db.TokenDefinition, error) {
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
		return db.TokenDefinition{}, persist.ErrTokenNotFoundByTokenIdentifiers{Token: ti}
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
		return db.TokenDefinition{}, err
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
func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts, existingContracts []db.Contract, canOverwriteOwnerAddress bool) (currentContractState []db.Contract, newContracts []db.Contract, err error) {
	contractsToUpsert := chainContractsToUpsertableContracts(contractsFromProviders, existingContracts)
	newUpsertedContracts, err := d.Repos.ContractRepository.BulkUpsert(ctx, contractsToUpsert, canOverwriteOwnerAddress)
	if err != nil {
		return nil, nil, err
	}

	return util.DedupeWithTranslate(append(newUpsertedContracts, existingContracts...), false, func(c db.Contract) persist.DBID { return c.ID }), newUpsertedContracts, nil
}

// chainTokensToUpsertableTokenDefinitions returns a slice of token definitions that are ready to be upserted into the database from a slice of chainTokens.
func chainTokensToUpsertableTokenDefinitions(chainTokens []chainTokens, existingContracts []db.Contract) []db.TokenDefinition {
	definitions := make(map[persist.TokenIdentifiers]db.TokenDefinition)

	// Create a lookup of contracts to their IDs
	contractLookup := make(map[persist.ContractIdentifiers]persist.DBID)
	for _, contract := range existingContracts {
		contractIdentifiers := persist.NewContractIdentifiers(contract.Address, contract.Chain)
		contractLookup[contractIdentifiers] = contract.ID
	}

	for _, chainToken := range chainTokens {
		for _, token := range chainToken.tokens {
			tokenIdentifiers := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chainToken.chain)
			contractIdentifiers := persist.NewContractIdentifiers(token.ContractAddress, chainToken.chain)
			contractID, ok := contractLookup[contractIdentifiers]
			if !ok {
				panic(fmt.Sprintf("contract %s should have already been upserted", contractIdentifiers))
			}
			// Got a new token, add it to the list of token definitions
			if definition, ok := definitions[tokenIdentifiers]; !ok {
				definitions[tokenIdentifiers] = db.TokenDefinition{
					Name:                 util.ToNullString(token.Descriptors.Name, true),
					Description:          util.ToNullString(token.Descriptors.Description, true),
					TokenID:              token.TokenID,
					TokenType:            token.TokenType,
					ExternalUrl:          util.ToNullString(token.ExternalURL, true),
					Chain:                chainToken.chain,
					IsProviderMarkedSpam: util.GetOptionalValue(token.IsSpam, false),
					Metadata:             token.TokenMetadata,
					FallbackMedia:        token.FallbackMedia,
					ContractAddress:      token.ContractAddress,
					ContractID:           contractID,
					TokenMediaID:         "", // Upsert will set this if the definition already exists
				}
			} else {
				// Merge the token definition with the existing one. The fields that aren't merged below use the data of the first write.
				name := util.FirstNonEmptyString(definition.Name.String, token.Descriptors.Name)
				description := util.FirstNonEmptyString(definition.Description.String, token.Descriptors.Description)
				externalURL := util.FirstNonEmptyString(definition.ExternalUrl.String, token.ExternalURL)
				fallbackMedia, _ := util.FindFirst([]persist.FallbackMedia{definition.FallbackMedia, token.FallbackMedia}, func(m persist.FallbackMedia) bool { return m.IsServable() })
				metadata, _ := util.FindFirst([]persist.TokenMetadata{definition.Metadata, token.TokenMetadata}, func(m persist.TokenMetadata) bool { return len(m) > 0 })

				definition.Name = util.ToNullString(name, true)
				definition.Description = util.ToNullString(description, true)
				definition.ExternalUrl = util.ToNullString(externalURL, true)
				definition.FallbackMedia = fallbackMedia
				definition.Metadata = metadata
				definition.IsProviderMarkedSpam = definition.IsProviderMarkedSpam || util.GetOptionalValue(token.IsSpam, false)
				definitions[tokenIdentifiers] = definition
			}
		}
	}

	tokenDefinitions := make([]db.TokenDefinition, 0, len(definitions))
	for _, definition := range definitions {
		tokenDefinitions = append(tokenDefinitions, definition)
	}

	return tokenDefinitions
}

// chainTokensToUpsertableTokens returns a unique slice of tokens that are ready to be upserted into the database.
func chainTokensToUpsertableTokens(tokens []chainTokens, existingContracts []db.Contract, ownerUser persist.User) []db.Token {
	addressToContract := make(map[string]db.Contract)

	util.Map(existingContracts, func(c db.Contract) (any, error) {
		addressToContract[c.Chain.NormalizeAddress(c.Address)] = c
		return nil, nil
	})

	seenTokens := make(map[persist.TokenIdentifiers]db.Token)

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

			contractAddress := chainToken.chain.NormalizeAddress(token.ContractAddress)
			contract := addressToContract[contractAddress]

			// Last write wins
			seenTokens[ti] = db.Token{
				Chain:          chainToken.chain,
				TokenID:        token.TokenID,
				OwnerUserID:    ownerUser.ID,
				ContractID:     contract.ID,
				BlockNumber:    sql.NullInt64{Int64: token.BlockNumber.BigInt().Int64(), Valid: true},
				IsCreatorToken: createdContracts[persist.Address(contractAddress)],
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
			finalSeenToken.OwnedByWallets = util.MapWithoutError(seenWallets[ti], func(w persist.Wallet) persist.DBID { return w.ID })
			finalSeenToken.Quantity = seenQuantities[ti]
			seenTokens[ti] = finalSeenToken
		}
	}

	res := make([]db.Token, len(seenTokens))

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

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("%s-%s", t.ContractAddress, t.TokenID)
}

func (e errWithPriority) Error() string {
	return fmt.Sprintf("error with priority %d: %s", e.priority, e.err)
}

func dedupeWallets(wallets []persist.Wallet) []persist.Wallet {
	return util.DedupeWithTranslate(wallets, false, func(w persist.Wallet) string {
		return fmt.Sprintf("%d-%s", w.Chain, w.Address)
	})
}

// dedupeTokenDefinitions returns a slice of deduped token definitions. This is necessary because the upsert cannot handle duplicates in input.
// Since the same set of providers are used for each user, we assume that the definitions for a token are the same so we can write them in any order.
func dedupeTokenDefinitions(tokenDefs []db.TokenDefinition) (uniqueDefs []db.TokenDefinition) {
	return util.DedupeWithTranslate(tokenDefs, false, func(t db.TokenDefinition) string {
		return fmt.Sprintf("%d-%s-%s", t.Chain, t.ContractID, t.TokenID)
	})
}

// dedupeTokens returns a slice of deduped tokens. This is necessary because the upsert cannot handle duplicates in input.
func dedupeTokens(tokens []db.Token) (uniqueTokens []db.Token) {
	return util.DedupeWithTranslate(tokens, false, func(t db.Token) string {
		return fmt.Sprintf("%d-%s-%s-%s", t.Chain, t.ContractID, t.TokenID, t.OwnerUserID)
	})
}
