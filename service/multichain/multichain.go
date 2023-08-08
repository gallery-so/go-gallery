package multichain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc"
	"github.com/sourcegraph/conc/pool"

	"github.com/gammazero/workerpool"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("TOKEN_PROCESSING_URL", "required")
}

const staleCommunityTime = time.Minute * 30

const maxCommunitySize = 10_000

// SendTokens is called to process a user's batch of tokens
type SendTokens func(context.Context, task.TokenProcessingUserMessage) error

type Provider struct {
	Repos   *postgres.Repositories
	Queries *db.Queries
	Cache   *redis.Cache
	Chains  map[persist.Chain][]any

	// some chains use the addresses of other chains, this will map of chain we want tokens from => chain that's address will be used for lookup
	WalletOverrides WalletOverrideMap
	SendTokens      SendTokens
}

// BlockchainInfo retrieves blockchain info from all chains
type BlockchainInfo struct {
	Chain      persist.Chain `json:"chain_name"`
	ChainID    int           `json:"chain_id"`
	ProviderID int           `json:"provider_id"`
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
	GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokenByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

type TokensContractFetcher interface {
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

type ContractsFetcher interface {
	GetContractByAddress(ctx context.Context, contract persist.Address) (ChainAgnosticContract, error)
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
	seen := map[int]bool{}
	for _, p := range providers {
		if it, ok := p.(Configurer); ok && seen[it.GetBlockchainInfo().ProviderID] {
			continue
		} else if ok {
			seen[it.GetBlockchainInfo().ProviderID] = true
		}
		if it, ok := p.(T); ok {
			matches = append(matches, it)
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
						tokens, contracts, err := fetcher.GetTokensByWalletAddress(ctx, addr, 0, 0)
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
		logger.For(ctx).Infof("syncing %d chain %d tokens for user %s", len(t), c, user.Username)
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
			logger.For(ctx).Errorf("error while syncing tokens for user %s: %s", user.Username, err)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 && len(tokensFromProviders) == 0 {
		return nil, util.MultiErr(errs)
	}
	if !util.AllEqual(util.MapValues(discrepencyLog)) {
		logger.For(ctx).Debugf("discrepency: %+v", discrepencyLog)
	}

	persistedContracts, err := p.processContracts(ctx, contractsFromProviders, false)
	if err != nil {
		return nil, err
	}

	var newTokens []persist.TokenGallery
	if replace {
		_, newTokens, err = p.ReplaceHolderTokensForUser(ctx, tokensFromProviders, persistedContracts, user, chains)
	} else {
		_, newTokens, err = p.AddHolderTokensToUser(ctx, tokensFromProviders, persistedContracts, user, chains)
	}
	if err != nil {
		return nil, err
	}

	return newTokens, nil
}

// SyncCreatedTokens updates tokens created by a user
func (p *Provider) SyncCreatedTokens(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"user_id": userID, "chains": chains})

	err := p.SyncContractsOwnedByUser(ctx, userID, chains)
	if err != nil {
		return err
	}

	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	chainInts, _ := util.Map(chains, func(c persist.Chain) (int32, error) { return int32(c), nil })
	rows, err := p.Queries.GetCreatedContractsByUserID(ctx, db.GetCreatedContractsByUserIDParams{
		UserID: userID,
		Chains: chainInts,
	})

	if err != nil {
		return err
	}

	contracts, _ := util.Map(rows, func(row db.GetCreatedContractsByUserIDRow) (db.Contract, error) { return row.Contract, nil })

	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)
	chainsToAddresses := make(map[persist.Chain][]persist.Address)

	for _, chain := range chains {
		for _, contract := range contracts {
			if contract.Chain == chain {
				chainsToAddresses[chain] = append(chainsToAddresses[chain], contract.Address)
			}
		}
	}

	wg := conc.WaitGroup{}
	for c, a := range chainsToAddresses {
		logger.For(ctx).Infof("syncing chain %d creator tokens for user %s contracts %s", c, user.Username, a)
		chain := c
		addresses := a
		for _, addr := range addresses {
			addr := addr
			chain := chain
			wg.Go(func() {
				tokenFetchers := matchingProvidersForChain[TokensContractFetcher](p.Chains, chain)
				subWg := &conc.WaitGroup{}
				for i, f := range tokenFetchers {
					priority := i
					fetcher := f

					subWg.Go(func() {
						tokens, contract, err := fetcher.GetTokensByContractAddress(ctx, addr, 0, 0)
						if err != nil {
							errChan <- errWithPriority{err: err, priority: priority}
							return
						}

						incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
						incomingContracts <- chainContracts{chain: chain, contracts: []ChainAgnosticContract{contract}, priority: priority}
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

	tokensFromProviders := make([]chainTokens, 0, len(contracts))
	contractsFromProviders := make([]chainContracts, 0, len(contracts))

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
			logger.For(ctx).Errorf("error while syncing tokens for user %s: %s", user.Username, err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 && len(tokensFromProviders) == 0 {
		return util.MultiErr(errs)
	}
	if !util.AllEqual(util.MapValues(discrepencyLog)) {
		logger.For(ctx).Debugf("discrepency: %+v", discrepencyLog)
	}

	persistedContracts, err := p.processContracts(ctx, contractsFromProviders, false)
	if err != nil {
		return err
	}

	_, _, err = p.ReplaceCreatorTokensForUser(ctx, tokensFromProviders, persistedContracts, user, chains)
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
	return contractsToNewDedupedContracts(combined)
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
	providerPool := pool.NewWithResults[ProviderChildContractResult]().WithContext(ctx).WithCancelOnError()

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

func (p *Provider) prepTokensForTokenProcessing(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User) ([]persist.TokenGallery, map[persist.TokenIdentifiers]bool, error) {
	providerTokens, _ := tokensToNewDedupedTokens(tokensFromProviders, contracts, user)

	currentTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, 0, 0)
	if err != nil {
		return nil, nil, err
	}

	tokenLookup := make(map[persist.TokenIdentifiers]persist.TokenGallery)
	for _, token := range currentTokens {
		tokenLookup[token.TokenIdentifiers()] = token
	}

	newTokens := make(map[persist.TokenIdentifiers]bool)

	for i, token := range providerTokens {
		existingToken, exists := tokenLookup[token.TokenIdentifiers()]

		// Add already existing media to the provider token if it exists so that
		// we can display media for a token while it gets handled by tokenprocessing
		if !token.Media.IsServable() && existingToken.Media.IsServable() {
			// TODO remove
			providerTokens[i].Media = existingToken.Media
		}

		if !token.FallbackMedia.IsServable() && existingToken.FallbackMedia.IsServable() {
			providerTokens[i].FallbackMedia = existingToken.FallbackMedia
		}

		// There's no available media for the token at this point, so set the state to syncing
		// so we can show the loading state instead of a broken token while tokenprocessing handles it.
		if !exists && !token.Media.IsServable() {
			// TODO remove
			providerTokens[i].Media = persist.Media{MediaType: persist.MediaTypeSyncing}
		}

		if !exists || existingToken.TokenMediaID == "" {
			newTokens[token.TokenIdentifiers()] = true
		}
	}

	return providerTokens, newTokens, nil
}

func (p *Provider) processTokensForOwnersOfContract(ctx context.Context, contractID persist.DBID, chain persist.Chain, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens, contracts []persist.ContractGallery) error {
	tokensToUpsert := make([]persist.TokenGallery, 0, len(chainTokensForUsers)*3)
	userTokenOffsets := make(map[persist.DBID][2]int)
	newUserTokens := make(map[persist.DBID]map[persist.TokenIdentifiers]bool)

	for userID, user := range users {
		tokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, chainTokensForUsers[userID], contracts, user)
		if err != nil {
			return err
		}

		start := len(tokensToUpsert)
		tokensToUpsert = append(tokensToUpsert, tokens...)
		userTokenOffsets[userID] = [2]int{start, start + len(tokens)}
		newUserTokens[userID] = newTokens
	}

	persistedTokens, err := p.Repos.TokenRepository.BulkUpsertTokensOfContract(ctx, contractID, tokensToUpsert, postgres.UpsertOptions{
		SetHolderFields:  true,
		SetCreatorFields: false,
		SkipDelete:       false,
	})
	if err != nil {
		return err
	}

	// Invariant to make sure that its safe to index persistedTokens
	if len(tokensToUpsert) != len(persistedTokens) {
		panic("expected the length of tokens inserted to match the input length")
	}

	errors := make([]error, 0)
	for userID, offset := range userTokenOffsets {
		start, end := offset[0], offset[1]
		userTokenIDs := make([]persist.DBID, 0, end-start)

		for _, token := range persistedTokens[start:end] {
			if newUserTokens[userID][token.TokenIdentifiers()] {
				userTokenIDs = append(userTokenIDs, token.ID)
			}
		}

		err = p.sendTokensToTokenProcessing(ctx, userID, userTokenIDs, []persist.Chain{chain})
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 1 {
		return errors[0]
	}

	return nil
}

// AddHolderTokensToUser will append to a user's existing holder tokens
func (p *Provider) AddHolderTokensToUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, tokensFromProviders, contracts, user, chains, postgres.UpsertOptions{
		SetHolderFields:  true,
		SetCreatorFields: false,
		SkipDelete:       true,
	})
}

// ReplaceCreatorTokensForUser will replace a user's existing creator tokens with the new tokens
func (p *Provider) ReplaceCreatorTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, tokensFromProviders, contracts, user, chains, postgres.UpsertOptions{
		SetHolderFields:  false,
		SetCreatorFields: true,
		SkipDelete:       false,
	})
}

// ReplaceHolderTokensForUser will replace a user's existing holder tokens with the new tokens
func (p *Provider) ReplaceHolderTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, tokensFromProviders, contracts, user, chains, postgres.UpsertOptions{
		SetHolderFields:  true,
		SetCreatorFields: false,
		SkipDelete:       false,
	})
}

func (p *Provider) processTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain, options postgres.UpsertOptions) ([]persist.TokenGallery, []persist.TokenGallery, error) {
	dedupedTokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, tokensFromProviders, contracts, user)
	if err != nil {
		return nil, nil, err
	}

	logger.For(ctx).Infof("%d new tokens and %d deduped tokens for user %s", len(newTokens), len(dedupedTokens), user.ID)

	persistedTokens, err := p.Repos.TokenRepository.BulkUpsertByOwnerUserID(ctx, user.ID, chains, dedupedTokens, options)
	if err != nil {
		return nil, nil, err
	}

	logger.For(ctx).Infof("%d persisted tokens for user %s", len(persistedTokens), user.ID)

	tokenIDs := make([]persist.DBID, 0, len(newTokens))
	for _, token := range persistedTokens {
		if newTokens[token.TokenIdentifiers()] {
			tokenIDs = append(tokenIDs, token.ID)
		}
	}

	err = p.sendTokensToTokenProcessing(ctx, user.ID, tokenIDs, chains)

	newGalleryTokens := util.Filter(persistedTokens, func(t persist.TokenGallery) bool {
		return newTokens[t.TokenIdentifiers()]
	}, false)
	return persistedTokens, newGalleryTokens, err
}

func (p *Provider) sendTokensToTokenProcessing(ctx context.Context, userID persist.DBID, tokens []persist.DBID, chains []persist.Chain) error {
	if len(tokens) == 0 {
		return nil
	}
	return p.SendTokens(ctx, task.TokenProcessingUserMessage{UserID: userID, TokenIDs: tokens, Chains: chains})
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

	persistedContracts, err := p.processContracts(ctx, contracts, false)
	if err != nil {
		return nil, err
	}

	allTokens, _, err := p.AddHolderTokensToUser(ctx, tokensFromProviders, persistedContracts, user, []persist.Chain{wallet.Chain()})
	if err != nil {
		return nil, err
	}
	return allTokens, nil
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

	wp := pool.New().WithMaxGoroutines(len(metadataFetchers)).WithContext(ctx).WithCancelOnError()
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

	tokenFetchers := matchingProvidersForChain[TokenDescriptorsFetcher](p.Chains, ti.Chain)

	finalTokenDescriptors := ChainAgnosticTokenDescriptors{}
	finalContractDescriptors := ChainAgnosticContractDescriptors{}
	for _, tokenFetcher := range tokenFetchers {

		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}

		token, contract, err := tokenFetcher.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
		if err == nil {
			// token
			if token.Name != "" && finalContractDescriptors.Name == "" {
				finalTokenDescriptors.Name = token.Name
			}
			if token.Description != "" && finalContractDescriptors.Description == "" {
				finalTokenDescriptors.Description = token.Description
			}

			// contract
			if contract.Name != "" && finalContractDescriptors.Name == "" {
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
		} else {
			logger.For(ctx).Infof("token %s-%s-%d not found for refresh (err: %s)", ti.TokenID, ti.ContractAddress, ti.Chain, err)
		}

	}

	if err := p.Queries.UpdateTokenMetadataFieldsByTokenIdentifiers(ctx, db.UpdateTokenMetadataFieldsByTokenIdentifiersParams{
		Name:            util.ToNullString(finalTokenDescriptors.Name, true),
		Description:     util.ToNullString(finalTokenDescriptors.Description, true),
		TokenID:         ti.TokenID,
		ContractAddress: ti.ContractAddress,
	}); err != nil {
		return err
	}

	if err := p.Repos.ContractRepository.UpsertByAddress(ctx, ti.ContractAddress, ti.Chain, persist.ContractGallery{
		Chain:        ti.Chain,
		Address:      persist.Address(ti.Chain.NormalizeAddress(ti.ContractAddress)),
		Symbol:       persist.NullString(finalContractDescriptors.Symbol),
		Name:         persist.NullString(finalContractDescriptors.Name),
		OwnerAddress: finalContractDescriptors.CreatorAddress,
	}); err != nil {
		return err
	}
	return nil
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	contractRefreshers := matchingProvidersForChain[ContractRefresher](p.Chains, ci.Chain)
	for _, refresher := range contractRefreshers {
		if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	return nil
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

	persistedContracts, err := p.processContracts(ctx, contractsFromProviders, false)
	if err != nil {
		return err
	}

	contract, err := p.Queries.GetContractByChainAddress(ctx, db.GetContractByChainAddressParams{
		Address: ci.ContractAddress,
		Chain:   ci.Chain,
	})
	if err != nil {
		return err
	}

	return p.processTokensForOwnersOfContract(ctx, contract.ID, ci.Chain, users, chainTokensForUsers, persistedContracts)
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

	contractFetchers := matchingProvidersByChains[ContractsFetcher](p.Chains, chains...)
	searchAddresses := p.matchingWallets(user.Wallets, chains)
	providerPool := pool.NewWithResults[ContractOwnerResult]().WithContext(ctx).WithCancelOnError()

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

	_, err = p.processContracts(ctx, contractsFromProviders, true)
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

// processContracts deduplicates contracts and upserts them into the database. If canOverwriteOwnerAddress is true, then
// the owner address of an existing contract will be overwritten if the new contract provides a non-empty owner address.
// An empty owner address will never overwrite an existing address, even if canOverwriteOwnerAddress is true.
func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts, canOverwriteOwnerAddress bool) ([]persist.ContractGallery, error) {
	newContracts := contractsToNewDedupedContracts(contractsFromProviders)
	return d.Repos.ContractRepository.BulkUpsert(ctx, newContracts, canOverwriteOwnerAddress)
}

func tokensToNewDedupedTokens(tokens []chainTokens, contracts []persist.ContractGallery, ownerUser persist.User) ([]persist.TokenGallery, map[persist.DBID]persist.Address) {
	addressToDBID := make(map[string]persist.DBID)

	util.Map(contracts, func(c persist.ContractGallery) (any, error) {
		addressToDBID[c.Chain.NormalizeAddress(c.Address)] = c.ID
		return nil, nil
	})

	seenTokens := make(map[persist.TokenIdentifiers]persist.TokenGallery)

	seenWallets := make(map[persist.TokenIdentifiers][]persist.Wallet)
	seenQuantities := make(map[persist.TokenIdentifiers]persist.HexString)
	addressToWallets := make(map[string]persist.Wallet)
	tokenDBIDToAddress := make(map[persist.DBID]persist.Address)
	createdContracts := make(map[persist.Address]bool)

	for _, wallet := range ownerUser.Wallets {
		// could a normalized address ever overlap with the normalized address of another chain?
		normalizedAddress := wallet.Chain.NormalizeAddress(wallet.Address)
		addressToWallets[normalizedAddress] = wallet
	}

	sort.SliceStable(tokens, func(i int, j int) bool {
		return tokens[i].priority < tokens[j].priority
	})

	for _, contract := range contracts {
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
			existingToken, seen := seenTokens[ti]

			contractAddress := chainToken.chain.NormalizeAddress(token.ContractAddress)
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
				Contract:             addressToDBID[contractAddress],
				ExternalURL:          persist.NullString(token.ExternalURL),
				BlockNumber:          token.BlockNumber,
				IsProviderMarkedSpam: token.IsSpam,
				IsCreatorToken:       createdContracts[persist.Address(contractAddress)],
			}

			// If we've never seen the incoming token before, then add it.
			if !seen {
				seenTokens[ti] = candidateToken
			} else if len(existingToken.TokenMetadata) < len(candidateToken.TokenMetadata) {
				if existingToken.FallbackMedia.IsServable() && !candidateToken.FallbackMedia.IsServable() {
					candidateToken.FallbackMedia = existingToken.FallbackMedia
				}
				seenTokens[ti] = candidateToken
			} else {
				if !existingToken.FallbackMedia.IsServable() && candidateToken.FallbackMedia.IsServable() {
					existingToken.FallbackMedia = candidateToken.FallbackMedia
					seenTokens[ti] = existingToken
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

			seenToken := seenTokens[ti]
			ownership := fromMultichainToAddressAtBlock(token.OwnershipHistory)
			seenToken.OwnershipHistory = ownership
			seenToken.OwnedByWallets = seenWallets[ti]
			seenToken.Quantity = seenQuantities[ti]
			seenTokens[ti] = seenToken
			tokenDBIDToAddress[seenTokens[ti].ID] = ti.ContractAddress
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
	return res, tokenDBIDToAddress
}

type contractMetadata struct {
	Symbol          string
	Name            string
	OwnerAddress    persist.Address
	ProfileImageURL string
	Description     string
	IsSpam          bool
}

func contractsToNewDedupedContracts(contracts []chainContracts) []persist.ContractGallery {

	sort.SliceStable(contracts, func(i, j int) bool {
		return contracts[i].priority < contracts[j].priority
	})

	contractMetadatas := map[persist.ChainAddress]contractMetadata{}
	for _, chainContract := range contracts {
		for _, contract := range chainContract.contracts {

			meta := contractMetadatas[persist.NewChainAddress(contract.Address, chainContract.chain)]
			if contract.Descriptors.Symbol != "" {
				meta.Symbol = contract.Descriptors.Symbol
			}
			if contract.Descriptors.Name != "" {
				meta.Name = contract.Descriptors.Name
			}
			if contract.Descriptors.CreatorAddress != "" {
				meta.OwnerAddress = contract.Descriptors.CreatorAddress
			}
			if contract.Descriptors.Description != "" {
				meta.Description = contract.Descriptors.Description
			}
			if contract.Descriptors.ProfileImageURL != "" {
				meta.ProfileImageURL = contract.Descriptors.ProfileImageURL
			}
			if contract.IsSpam != nil && *contract.IsSpam {
				// only one provider needs to mark it as spam for it to be spam
				meta.IsSpam = true
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
