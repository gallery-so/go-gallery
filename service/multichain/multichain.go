package multichain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/sourcegraph/conc/pool"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
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
	ChainAddressOverrides ChainOverrideMap
	SendTokens            SendTokens
}

// BlockchainInfo retrieves blockchain info from all chains
type BlockchainInfo struct {
	Chain   persist.Chain `json:"chain_name"`
	ChainID int           `json:"chain_id"`
}

// ChainAgnosticToken is a token that is agnostic to the chain it is on
type ChainAgnosticToken struct {
	Media persist.Media `json:"media"`

	TokenType persist.TokenType `json:"token_type"`

	Name        string `json:"name"`
	Description string `json:"description"`

	TokenURI         persist.TokenURI              `json:"token_uri"`
	TokenID          persist.TokenID               `json:"token_id"`
	Quantity         persist.HexString             `json:"quantity"`
	OwnerAddress     persist.Address               `json:"owner_address"`
	OwnershipHistory []ChainAgnosticAddressAtBlock `json:"previous_owners"`
	TokenMetadata    persist.TokenMetadata         `json:"metadata"`
	ContractAddress  persist.Address               `json:"contract_address"`

	ExternalURL string `json:"external_url"`

	BlockNumber persist.BlockNumber `json:"block_number"`
	IsSpam      *bool               `json:"is_spam"`
}

func (c ChainAgnosticToken) hasMetadata() bool {
	return len(c.TokenMetadata) > 0
}

// ChainAgnosticAddressAtBlock is an address at a block
type ChainAgnosticAddressAtBlock struct {
	Address persist.Address     `json:"address"`
	Block   persist.BlockNumber `json:"block"`
}

func (c ChainAgnosticAddressAtBlock) ToAddressAtBlock() persist.AddressAtBlock {
	return persist.AddressAtBlock{Address: c.Address, Block: c.Block}
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Address        persist.Address     `json:"address"`
	Symbol         string              `json:"symbol"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	CreatorAddress persist.Address     `json:"creator_address"`
	LatestBlock    persist.BlockNumber `json:"latest_block"`
}

// ChildContract represents a subset of tokens within a contract, identified by a slug
type ChildContract struct {
	ChildID        string // Uniquely identifies a child contract within a parent contract
	Name           string
	Description    string
	CreatorAddress persist.Address
	ParentContract ChainAgnosticContract
	Tokens         []ChainAgnosticToken
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

type tokenIdentifiers struct {
	chain    persist.Chain
	tokenID  persist.TokenID
	contract persist.DBID
}

type errWithPriority struct {
	err      error
	priority int
}

// configurer maintains provider settings
type configurer interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
}

// nameResolver is able to resolve an address to a friendly display name
type nameResolver interface {
	GetDisplayNameByAddress(context.Context, persist.Address) string
}

// verifier can verify that a signature is signed by a given key
type verifier interface {
	VerifySignature(ctx context.Context, pubKey persist.PubKey, walletType persist.WalletType, nonce string, sig string) (bool, error)
}

type walletHooker interface {
	// WalletCreated is called when a wallet is created
	WalletCreated(context.Context, persist.DBID, persist.Address, persist.WalletType) error
}

// tokensFetcher supports fetching tokens for syncing
type tokensFetcher interface {
	GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

// childContractFetcher supports fetching tokens created by an address
type childContractFetcher interface {
	GetContractsCreatedOnSharedContract(ctx context.Context, address persist.Address) ([]ChildContract, error)
}

// tokenRefresher supports refreshes of a token
type tokenRefresher interface {
	RefreshToken(context.Context, ChainAgnosticIdentifiers, persist.Address) error
}

// tokenFetcherRefresher is the interface that combines the tokenFetcher and tokenRefresher interface
type tokenFetcherRefresher interface {
	tokensFetcher
	tokenRefresher
}

// contractRefresher supports refreshes of a contract
type contractRefresher interface {
	RefreshContract(context.Context, persist.Address) error
}

// tokenMetadataFetcher supports fetching token metadata
type tokenMetadataFetcher interface {
	GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers, ownerAddress persist.Address) (persist.TokenMetadata, error)
}

type ChainOverrideMap = map[persist.Chain]*persist.Chain

// NewProvider creates a new MultiChainDataRetriever
func NewProvider(ctx context.Context, repos *postgres.Repositories, queries *db.Queries, cache *redis.Cache, taskClient *cloudtasks.Client, chainOverrides ChainOverrideMap, providers ...interface{}) *Provider {
	return &Provider{
		Repos:                 repos,
		Cache:                 cache,
		Queries:               queries,
		Chains:                validateProviders(ctx, providers),
		ChainAddressOverrides: chainOverrides,
		SendTokens: func(ctx context.Context, t task.TokenProcessingUserMessage) error {
			return task.CreateTaskForTokenProcessing(ctx, taskClient, t)
		},
	}
}

var chainValidation map[persist.Chain]validation = map[persist.Chain]validation{
	persist.ChainETH: {
		NameResolver:          true,
		Verifier:              true,
		TokensFetcher:         true,
		TokenRefresher:        true,
		TokenFetcherRefresher: true,
		ContractRefresher:     true,
		TokenMetadataFetcher:  true,
		ChildContractFetcher:  true,
	},
	persist.ChainTezos: {
		TokensFetcher:         true,
		TokenFetcherRefresher: true,
	},
	persist.ChainPOAP: {
		NameResolver:  true,
		TokensFetcher: true,
	},
}

type validation struct {
	NameResolver          bool `json:"nameResolver"`
	Verifier              bool `json:"verifier"`
	TokensFetcher         bool `json:"tokensFetcher"`
	TokenRefresher        bool `json:"tokenRefresher"`
	TokenFetcherRefresher bool `json:"tokenFetcherRefresher"`
	TokenMetadataFetcher  bool `json:"tokenMetadataFetcher"`
	ContractRefresher     bool `json:"contractRefresher"`
	ChildContractFetcher  bool `json:"childContractFetcher"`
}

// validateProviders verifies that the input providers match the expected multichain spec and panics otherwise
func validateProviders(ctx context.Context, providers []any) map[persist.Chain][]any {
	provided := make(map[persist.Chain][]any)
	for _, p := range providers {
		cfg := p.(configurer)
		info, err := cfg.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		provided[info.Chain] = append(provided[info.Chain], cfg)
	}

	passesValidation := true
	errorMsg := "\n[ multichain validation ]\n"

	validateChain := func(chain persist.Chain, expected, actual map[string]bool) {
		fail := false
		header := fmt.Sprintf("validation results for chain: %d\n", chain)
		for s := range expected {
			if expected[s] && !actual[s] {
				if !fail {
					fail = true
					errorMsg += header
				}
				errorMsg += fmt.Sprintf("\t* missing implemntation of %s\n", s)
			}
		}
		passesValidation = passesValidation && !fail
	}

	for chain, spec := range chainValidation {
		providers := provided[chain]

		expected := make(map[string]bool)
		byt, _ := json.Marshal(spec)
		json.Unmarshal(byt, &expected)

		actual := make(map[string]bool)

		markValid := func(i string) {
			actual[i] = true
			expected[i] = true
		}

		for _, p := range providers {
			if _, ok := p.(nameResolver); ok {
				markValid("nameResolver")
			}
			if _, ok := p.(verifier); ok {
				markValid("verifier")
			}
			if _, ok := p.(tokensFetcher); ok {
				markValid("tokensFetcher")
			}
			if _, ok := p.(tokenRefresher); ok {
				markValid("tokenRefresher")
			}
			if _, ok := p.(tokenFetcherRefresher); ok {
				markValid("tokenFetcherRefresher")
			}
			if _, ok := p.(contractRefresher); ok {
				markValid("contractRefresher")
			}
			if _, ok := p.(tokenMetadataFetcher); ok {
				markValid("tokenMetadataFetcher")
			}
			if _, ok := p.(childContractFetcher); ok {
				markValid("childContractFetcher")
			}
		}
		validateChain(chain, expected, actual)
	}

	if !passesValidation {
		panic(errorMsg)
	}

	return provided
}

// providersMatchingInterface returns providers that adhere to the given interface
func providersMatchingInterface[T any](providers []any) []T {
	matches := make([]T, 0)
	for _, p := range providers {
		if it, ok := p.(T); ok {
			matches = append(matches, it)
		}
	}
	return matches
}

// matchingProviders returns providers that adhere to the given interface per chain
func matchingProviders[T any](availableProviders map[persist.Chain][]any, requestedChains ...persist.Chain) map[persist.Chain][]T {
	matches := make(map[persist.Chain][]T)
	for availableChain, providers := range availableProviders {
		for _, requestChain := range requestedChains {
			if availableChain == requestChain {
				matches[requestChain] = providersMatchingInterface[T](providers)
			}
		}
	}
	return matches
}

func matchingProvidersForChain[T any](availableProviders map[persist.Chain][]any, chain persist.Chain) []T {
	return matchingProviders[T](availableProviders, chain)[chain]
}

// matchingAddresses returns wallet addresses that belong to any of the passed chains
func matchingAddresses(wallets []persist.Wallet, chains []persist.Chain) map[persist.Chain][]persist.Address {
	matches := make(map[persist.Chain][]persist.Address)
	for _, chain := range chains {
		for _, wallet := range wallets {
			if wallet.Chain == chain {
				matches[chain] = append(matches[chain], wallet.Address)
			}
		}
	}
	return matches
}

// contractAddressToDBID maps contract addresses to their DBIDs
func contractAddressToDBID(contracts []persist.ContractGallery) map[string]persist.DBID {
	m := make(map[string]persist.DBID)
	for _, contract := range contracts {
		m[contract.Chain.NormalizeAddress(contract.Address)] = contract.ID
	}
	return m
}

// tokenIDstoDBID maps tokens to their DBIDs
func tokenIDstoDBID(tokens []persist.TokenGallery) map[persist.TokenIdentifiers]persist.DBID {
	m := make(map[persist.TokenIdentifiers]persist.DBID)
	for _, token := range tokens {
		m[token.TokenIdentifiers()] = token.ID
	}
	return m
}

// SyncTokens updates the media for all tokens for a user
// TODO consider updating contracts as well
func (p *Provider) SyncTokens(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)
	chainsToAddresses := make(map[persist.Chain][]persist.Address)

	for _, chain := range chains {
		for _, wallet := range user.Wallets {
			override := p.ChainAddressOverrides[chain]

			if wallet.Chain == chain || (override != nil && *override == wallet.Chain) {
				chainsToAddresses[chain] = append(chainsToAddresses[chain], wallet.Address)
			}
		}
	}

	wg := sync.WaitGroup{}
	for c, a := range chainsToAddresses {
		logger.For(ctx).Infof("syncing tokens for user %s wallets %s", user.Username, a)
		chain := c
		addresses := a
		wg.Add(len(addresses))
		for _, addr := range addresses {
			go func(addr persist.Address, chain persist.Chain) {
				defer wg.Done()
				start := time.Now()
				fetchers := matchingProvidersForChain[tokensFetcher](p.Chains, chain)
				subWg := &sync.WaitGroup{}
				subWg.Add(len(fetchers))
				for i, fetcher := range fetchers {
					go func(fetcher tokensFetcher, priority int) {
						defer subWg.Done()
						tokens, contracts, err := fetcher.GetTokensByWalletAddress(ctx, addr, 0, 0)
						if err != nil {
							errChan <- errWithPriority{err: err, priority: priority}
							return
						}

						incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
						incomingContracts <- chainContracts{chain: chain, contracts: contracts, priority: priority}
					}(fetcher, i)
				}
				subWg.Wait()
				logger.For(ctx).Debugf("updated media for user %s wallet %s in %s", user.Username, addr, time.Since(start))
			}(addr, chain)
		}
	}

	go func() {
		defer close(incomingTokens)
		defer close(incomingContracts)
		wg.Wait()
	}()

	tokensFromProviders := make([]chainTokens, 0, len(user.Wallets))
	contractsFromProviders := make([]chainContracts, 0, len(user.Wallets))

outer:
	for {
		select {
		case incomingTokens := <-incomingTokens:
			tokensFromProviders = append(tokensFromProviders, incomingTokens)
		case incomingContracts, ok := <-incomingContracts:
			if !ok {
				break outer
			}
			contractsFromProviders = append(contractsFromProviders, incomingContracts)
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			if err, ok := err.(errWithPriority); ok {
				if err.priority == 0 {
					return err
				}
				logger.For(ctx).Errorf("error updating fallback media for user %s: %s", user.Username, err)
			} else {
				return err
			}
		}
	}

	persistedContracts, err := p.processContracts(ctx, contractsFromProviders)
	if err != nil {
		return err
	}

	_, err = p.ReplaceTokensForUser(ctx, tokensFromProviders, persistedContracts, user, chains)
	return err
}

type ChildContractResult struct {
	Priority       int
	Chain          persist.Chain
	ChildContracts []ChildContract
}

// ParentContracts returns all parent contracts
func (u ChildContractResult) ParentContracts() chainContracts {
	contracts := make([]ChainAgnosticContract, 0)
	for _, child := range u.ChildContracts {
		contracts = append(contracts, child.ParentContract)
	}
	return chainContracts{
		priority:  u.Priority,
		chain:     u.Chain,
		contracts: contracts,
	}
}

// Tokens returns all tokens that the user has created across all contracts
func (u ChildContractResult) Tokens() chainTokens {
	tokens := make([]ChainAgnosticToken, 0)
	for _, child := range u.ChildContracts {
		for _, token := range child.Tokens {
			tokens = append(tokens, token)
		}
	}
	return chainTokens{
		priority: u.Priority,
		chain:    u.Chain,
		tokens:   tokens,
	}
}

// combinedChildContractResults is a helper type for combining results from multiple providers
type combinedChildContractResults []ChildContractResult

// ParentContracts returns all umbrella contracts across all providers
func (c combinedChildContractResults) ParentContracts() []persist.ContractGallery {
	contracts := make([]chainContracts, 0, len(c))
	for i, p := range c {
		contracts[i] = p.ParentContracts()
	}
	return contractsToNewDedupedContracts(contracts)
}

// ChildContracts returns all child contracts across all providers
func (c combinedChildContractResults) ChildContracts() []ChildContract {
	contracts := make([]ChildContract, 0)
	for _, p := range c {
		contracts = append(contracts, p.ChildContracts...)
	}
	// TODO: We may want to dedupe across providers here, but its unlikely
	// that the same umbrella contract will be returned by different providers
	return contracts
}

// Tokens combines all tokens from all providers
func (c combinedChildContractResults) Tokens() []chainTokens {
	tokens := make([]chainTokens, 0, len(c))
	for i, p := range c {
		tokens[i] = p.Tokens()
	}
	return tokens
}

type userIDToUser map[persist.DBID]persist.User

// AddressToUser returns a map of user IDs to users
func addressToUser(c []userIDToUser) map[persist.Address]persist.User {
	addressToUser := make(map[persist.Address]persist.User)
	for _, result := range c {
		for _, user := range result {
			for _, wallet := range user.Wallets {
				addressToUser[wallet.Address] = user
			}
		}
	}
	return addressToUser
}

// SyncTokensCreatedOnSharedContracts queries each provider to identify contracts created by the given user.
func (p *Provider) SyncTokensCreatedOnSharedContracts(ctx context.Context, userID persist.DBID, chains []persist.Chain, includeAll bool) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if includeAll {
		chains = make([]persist.Chain, 0)
		for chain := range p.Chains {
			chains = append(chains, chain)
		}
	}

	fetchers := matchingProviders[childContractFetcher](p.Chains, chains...)
	searchAddresses := matchingAddresses(user.Wallets, chains)
	providerPool := pool.NewWithResults[ChildContractResult]().WithContext(ctx).WithCancelOnError()

	// Fetch all tokens created by the user
	for chain, addresses := range searchAddresses {
		for priority, fetcher := range fetchers[chain] {
			for _, address := range addresses {
				c := chain
				p := priority
				f := fetcher
				a := address
				providerPool.Go(func(ctx context.Context) (ChildContractResult, error) {
					contracts, err := f.GetContractsCreatedOnSharedContract(ctx, a)
					if err != nil {
						return ChildContractResult{}, err
					}
					return ChildContractResult{
						Priority:       p,
						Chain:          c,
						ChildContracts: contracts,
					}, nil
				})
			}
		}
	}

	pResult, err := providerPool.Wait()
	if err != nil {
		return err
	}

	combinedResult := combinedChildContractResults(pResult)

	// Create universal users for owners that aren't users yet
	createPool := pool.NewWithResults[userIDToUser]().WithContext(ctx).WithCancelOnError()

	for _, result := range combinedResult {
		createPool.Go(func(ctx context.Context) (userIDToUser, error) {
			_, newUsers, err := p.createUsersForTokens(ctx, combinedResult.Tokens(), result.Chain)
			return newUsers, err
		})
	}

	createUserResult, err := createPool.Wait()
	if err != nil {
		return err
	}

	addressToChain := make(map[persist.Address]persist.Chain)
	userLookup := addressToUser(createUserResult)
	params := db.UpsertCreatedTokensParams{}
	now := time.Now()

	var errors []error

	for _, contract := range combinedResult.ParentContracts() {
		params.ParentContractID = append(params.ParentContractID, persist.GenerateID().String())
		params.ParentContractDeleted = append(params.ParentContractDeleted, false)
		params.ParentContractCreatedAt = append(params.ParentContractCreatedAt, now)
		params.ParentContractName = append(params.ParentContractName, contract.Name.String())
		params.ParentContractSymbol = append(params.ParentContractSymbol, contract.Symbol.String())
		params.ParentContractAddress = append(params.ParentContractAddress, contract.Address.String())
		params.ParentContractCreatorAddress = append(params.ParentContractCreatorAddress, contract.CreatorAddress.String())
		params.ParentContractChain = append(params.ParentContractChain, int32(contract.Chain))
		params.ParentContractDescription = append(params.ParentContractDescription, contract.Description.String())
		addressToChain[contract.Address] = contract.Chain
	}
	for _, child := range combinedResult.ChildContracts() {
		chain := addressToChain[child.ParentContract.Address]
		params.ChildContractID = append(params.ChildContractID, persist.GenerateID().String())
		params.ChildContractDeleted = append(params.ChildContractDeleted, false)
		params.ChildContractCreatedAt = append(params.ChildContractCreatedAt, now)
		params.ChildContractName = append(params.ChildContractName, child.Name)
		params.ChildContractAddress = append(params.ChildContractAddress, child.ChildID)
		params.ChildContractCreatorAddress = append(params.ChildContractCreatorAddress, child.CreatorAddress.String())
		params.ChildContractChain = append(params.ChildContractChain, int32(chain))
		params.ChildContractDescription = append(params.ChildContractDescription, child.Description)
		params.ChildContractParentAddress = append(params.ChildContractParentAddress, child.ParentContract.Address.String())
	}
	for _, result := range combinedResult.Tokens() {
		for _, token := range result.tokens {
			owner := userLookup[token.OwnerAddress]
			params.TokenID = append(params.TokenID, persist.GenerateID().String())
			params.TokenDeleted = append(params.TokenDeleted, false)
			params.TokenCreatedAt = append(params.TokenCreatedAt, now)
			params.TokenName = append(params.TokenName, token.Name)
			params.TokenDescription = append(params.TokenDescription, token.Description)
			params.TokenTokenType = append(params.TokenTokenType, token.TokenType.String())
			params.TokenTokenID = append(params.TokenTokenID, token.TokenID.String())
			params.TokenQuantity = append(params.TokenQuantity, token.Quantity.String())
			postgres.AppendAddressAtBlock(&params.TokenOwnershipHistory, fromMultichainToAddressAtBlock(token.OwnershipHistory), &params.TokenOwnershipHistoryStartIdx, &params.TokenOwnershipHistoryEndIdx, &errors)
			params.TokenExternalUrl = append(params.TokenExternalUrl, token.ExternalURL)
			params.TokenBlockNumber = append(params.TokenBlockNumber, token.BlockNumber.BigInt().Int64())
			params.TokenOwnerUserID = append(params.TokenOwnerUserID, owner.ID.String())
			postgres.AppendWalletList(&params.TokenOwnedByWallets, token.OwnedByWallets, &params.TokenOwnedByWalletsStartIdx, &params.TokenOwnedByWalletsEndIdx, &errors)
			params.TokenChain = append(params.TokenChain, int32(result.chain))
			params.TokenIsProviderMarkedSpam = append(params.TokenIsProviderMarkedSpam, util.GetOptionalValue(token.IsSpam, false))
			params.TokenLastSynced = append(params.TokenLastSynced, now)
			params.TokenContractAddress = append(params.TokenContractAddress, token.ContractAddress.String())
		}
	}

	_, err = p.Queries.UpsertCreatedTokens(ctx, params)

	return err
}

func (p *Provider) prepTokensForTokenProcessing(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User) ([]persist.TokenGallery, map[persist.TokenIdentifiers]bool, error) {
	providerTokens, err := tokensToNewDedupedTokens(ctx, tokensFromProviders, contracts, user)
	if err != nil {
		return nil, nil, err
	}

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
			providerTokens[i].Media = existingToken.Media
		}

		// There's no available media for the token at this point, so set the state to syncing
		// so we can show the loading state instead of a broken token while tokenprocessing handles it.
		if !exists && !token.Media.IsServable() {
			providerTokens[i].Media = persist.Media{MediaType: persist.MediaTypeSyncing}
		}

		if !exists {
			newTokens[token.TokenIdentifiers()] = true
		}
	}

	return providerTokens, newTokens, nil
}

func (p *Provider) processTokensForOwnersOfContract(ctx context.Context, contractID persist.DBID, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens, contracts []persist.ContractGallery) error {
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

	persistedTokens, err := p.Repos.TokenRepository.BulkUpsertTokensOfContract(ctx, contractID, tokensToUpsert, false)
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

		err = p.sendTokensToTokenProcessing(ctx, userID, userTokenIDs)
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 1 {
		return errors[0]
	}

	return nil
}

func (p *Provider) AddTokensToUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, tokensFromProviders, contracts, user, chains, false)
}

func (p *Provider) ReplaceTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, error) {
	return p.processTokensForUser(ctx, tokensFromProviders, contracts, user, chains, true)
}

func (p *Provider) processTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, contracts []persist.ContractGallery, user persist.User, chains []persist.Chain, skipDelete bool) ([]persist.TokenGallery, error) {
	dedupedTokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, tokensFromProviders, contracts, user)
	if err != nil {
		return nil, err
	}

	persistedTokens, err := p.Repos.TokenRepository.BulkUpsertByOwnerUserID(ctx, user.ID, chains, dedupedTokens, skipDelete)
	if err != nil {
		return nil, err
	}

	tokenIDs := make([]persist.DBID, 0, len(newTokens))
	for _, token := range persistedTokens {
		if newTokens[token.TokenIdentifiers()] {
			tokenIDs = append(tokenIDs, token.ID)
		}
	}

	err = p.sendTokensToTokenProcessing(ctx, user.ID, tokenIDs)
	return persistedTokens, err
}

func (p *Provider) sendTokensToTokenProcessing(ctx context.Context, userID persist.DBID, tokens []persist.DBID) error {
	if len(tokens) == 0 {
		return nil
	}
	return p.SendTokens(ctx, task.TokenProcessingUserMessage{UserID: userID, TokenIDs: tokens})
}

func (p *Provider) processTokenMedia(ctx context.Context, tokenID persist.TokenID, contractAddress persist.Address, chain persist.Chain, ownerAddress persist.Address, imageKeywords, animationKeywords []string) error {
	input := map[string]interface{}{
		"token_id":           tokenID,
		"contract_address":   contractAddress,
		"chain":              chain,
		"owner_address":      ownerAddress,
		"image_keywords":     imageKeywords,
		"animation_keywords": animationKeywords,
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
		return util.GetErrFromResp(resp)
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

	fetchers := matchingProvidersForChain[tokensFetcher](p.Chains, wallet.Chain())
	tokensFromProviders := make([]chainTokens, 0, len(fetchers))
	contracts := make([]chainContracts, 0, len(fetchers))

	for i, fetcher := range fetchers {
		tokensOfOwner, contract, err := fetcher.GetTokensByContractAddressAndOwner(ctx, wallet.Address(), contractAddress, limit, offset)
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

	persistedContracts, err := p.processContracts(ctx, contracts)
	if err != nil {
		return nil, err
	}

	return p.AddTokensToUser(ctx, tokensFromProviders, persistedContracts, user, []persist.Chain{wallet.Chain()})
}

// GetTokenMetadataByTokenIdentifiers will get the metadata for a given token identifier
func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, ownerAddress persist.Address, chain persist.Chain) (persist.TokenMetadata, error) {
	var metadata persist.TokenMetadata
	var err error

	fetchers := matchingProvidersForChain[tokenMetadataFetcher](d.Chains, chain)

	for _, fetcher := range fetchers {
		metadata, err = fetcher.GetTokenMetadataByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{ContractAddress: contractAddress, TokenID: tokenID}, ownerAddress)
		if err == nil && len(metadata) > 0 {
			return metadata, nil
		}
	}

	return metadata, err
}

// RunWalletCreationHooks runs hooks for when a wallet is created
func (d *Provider) RunWalletCreationHooks(ctx context.Context, userID persist.DBID, walletAddress persist.Address, walletType persist.WalletType, chain persist.Chain) error {
	// User doesn't exist
	_, err := d.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// TODO check if user wallets contains wallet using new util.Contains in other PR
	hookers := matchingProvidersForChain[walletHooker](d.Chains, chain)

	for _, hooker := range hookers {
		if err := hooker.WalletCreated(ctx, userID, walletAddress, walletType); err != nil {
			return err
		}
	}

	return nil
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	verifiers := matchingProvidersForChain[verifier](p.Chains, pChainAddress.Chain())

	for _, verifier := range verifiers {
		if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (p *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers, ownerAddresses []persist.Address) error {
	refreshers := matchingProvidersForChain[tokenFetcherRefresher](p.Chains, ti.Chain)
	for i, refresher := range refreshers {
		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
		for _, ownerAddress := range ownerAddresses {
			if err := refresher.RefreshToken(ctx, id, ownerAddress); err != nil {
				return err
			}
		}

		if i == 0 {
			for _, ownerAddress := range ownerAddresses {
				refreshedToken, contract, err := refresher.GetTokensByTokenIdentifiersAndOwner(ctx, id, ownerAddress)
				if err != nil {
					return err
				}

				currentTokenState, err := p.Repos.TokenRepository.GetByTokenIdentifiers(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, 0, 0)
				if err != nil {
					return err
				}

				// Add existing media to the token if it already exists so theres
				// something to display for when no providers had media for it
				for i := 0; !refreshedToken.Media.IsServable() && i < len(currentTokenState); i++ {
					if !refreshedToken.Media.IsServable() && currentTokenState[i].Media.IsServable() {
						refreshedToken.Media = currentTokenState[i].Media
					}
				}

				if err := p.Repos.TokenRepository.UpdateByTokenIdentifiersUnsafe(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, persist.TokenUpdateAllMetadataFieldsInput{
					Metadata:    refreshedToken.TokenMetadata,
					Name:        persist.NullString(refreshedToken.Name),
					LastUpdated: persist.LastUpdatedTime{},
					TokenURI:    refreshedToken.TokenURI,
					Description: persist.NullString(refreshedToken.Description),
				}); err != nil {
					return err
				}

				image, anim := ti.Chain.BaseKeywords()
				err = p.processTokenMedia(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, refreshedToken.OwnerAddress, image, anim)
				if err != nil {
					return err
				}

				if err := p.Repos.ContractRepository.UpsertByAddress(ctx, ti.ContractAddress, ti.Chain, persist.ContractGallery{
					Chain:          ti.Chain,
					Address:        persist.Address(ti.Chain.NormalizeAddress(ti.ContractAddress)),
					Symbol:         persist.NullString(contract.Symbol),
					Name:           persist.NullString(contract.Name),
					CreatorAddress: contract.CreatorAddress,
				}); err != nil {
					return err
				}
			}

		}
	}
	return nil
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	refreshers := matchingProvidersForChain[contractRefresher](p.Chains, ci.Chain)
	for _, refresher := range refreshers {
		if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	return nil
}

// RefreshTokensForContract refreshes all tokens in a given contract
func (p *Provider) RefreshTokensForContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	tokensFromProviders := []chainTokens{}
	contractsFromProviders := []chainContracts{}
	tokensReceive := make(chan chainTokens)
	contractsReceive := make(chan chainContracts)
	errChan := make(chan errWithPriority)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	fetchers := matchingProvidersForChain[tokensFetcher](p.Chains, ci.Chain)
	for i, fetcher := range fetchers {
		wg.Add(1)
		go func(priority int, p tokensFetcher) {
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

	persistedContracts, err := p.processContracts(ctx, contractsFromProviders)
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

	return p.processTokensForOwnersOfContract(ctx, contract.ID, users, chainTokensForUsers, persistedContracts)
}

type tokenUniqueIdentifiers struct {
	chain        persist.Chain
	contract     persist.Address
	tokenID      persist.TokenID
	ownerAddress persist.Address
}

type tokenForUser struct {
	userID   persist.DBID
	token    ChainAgnosticToken
	chain    persist.Chain
	priority int
}

// this function returns a map of user IDs to their new tokens as well as a map of user IDs to the users themselves
func (p *Provider) createUsersForTokens(ctx context.Context, tokens []chainTokens, chain persist.Chain) (map[persist.DBID][]chainTokens, userIDToUser, error) {
	users := map[persist.DBID]persist.User{}
	userTokens := map[persist.DBID]map[int]chainTokens{}
	seenTokens := map[tokenUniqueIdentifiers]bool{}

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
			CreationTime:       persist.CreationTime(user.CreatedAt),
			Deleted:            persist.NullBool(user.Deleted),
			LastUpdated:        persist.LastUpdatedTime(user.LastUpdated),
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
		resolvers := matchingProvidersForChain[nameResolver](p.Chains, chainToken.chain)
		for _, agnosticToken := range chainToken.tokens {
			if agnosticToken.OwnerAddress == "" {
				continue
			}
			tid := tokenUniqueIdentifiers{chain: chainToken.chain, contract: agnosticToken.ContractAddress, tokenID: agnosticToken.TokenID, ownerAddress: agnosticToken.OwnerAddress}
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
							if display != "" {
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
						})
						if err != nil {
							if _, ok := err.(persist.ErrUsernameNotAvailable); ok {
								user, err = p.Repos.UserRepository.GetByUsername(ctx, username)
								if err != nil {
									errChan <- err
									return
								}
							} else if _, ok := err.(persist.ErrAddressOwnedByUser); ok {
								user, err = p.Repos.UserRepository.GetByChainAddress(ctx, persist.NewChainAddress(t.OwnerAddress, ct.chain))
								if err != nil {
									errChan <- err
									return
								}
							} else if _, ok := err.(persist.ErrWalletCreateFailed); ok {
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

func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts) ([]persist.ContractGallery, error) {
	newContracts := contractsToNewDedupedContracts(contractsFromProviders)
	return d.Repos.ContractRepository.BulkUpsert(ctx, newContracts)
}

func tokensToNewDedupedTokens(ctx context.Context, tokens []chainTokens, contracts []persist.ContractGallery, ownerUser persist.User) ([]persist.TokenGallery, error) {
	contractAddressDBIDs := contractAddressToDBID(contracts)
	seenTokens := make(map[persist.TokenIdentifiers]persist.TokenGallery)

	seenWallets := make(map[persist.TokenIdentifiers][]persist.Wallet)
	seenQuantities := make(map[persist.TokenIdentifiers]persist.HexString)
	addressToWallets := make(map[string]persist.Wallet)
	for _, wallet := range ownerUser.Wallets {
		// could a normalized address ever overlap with the normalized address of another chain?
		normalizedAddress := wallet.Chain.NormalizeAddress(wallet.Address)
		addressToWallets[normalizedAddress] = wallet
	}

	sort.SliceStable(tokens, func(i int, j int) bool {
		return tokens[i].priority < tokens[j].priority
	})

	for _, chainToken := range tokens {
		for _, token := range chainToken.tokens {

			if token.Quantity.BigInt().Cmp(big.NewInt(0)) == 0 {
				logger.For(ctx).Warnf("skipping token %s with quantity 0", token.Name)
				continue
			}

			ti := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chainToken.chain)
			existingToken, seen := seenTokens[ti]

			candidateToken := persist.TokenGallery{
				Media:                token.Media,
				TokenType:            token.TokenType,
				Chain:                chainToken.chain,
				Name:                 persist.NullString(token.Name),
				Description:          persist.NullString(token.Description),
				TokenURI:             "", // We don't save tokenURI information
				TokenID:              token.TokenID,
				OwnerUserID:          ownerUser.ID,
				TokenMetadata:        token.TokenMetadata,
				Contract:             contractAddressDBIDs[chainToken.chain.NormalizeAddress(token.ContractAddress)],
				ExternalURL:          persist.NullString(token.ExternalURL),
				BlockNumber:          token.BlockNumber,
				IsProviderMarkedSpam: token.IsSpam,
			}

			// If we've never seen the incoming token before, then add it.
			if !seen {
				seenTokens[ti] = candidateToken
			} else if len(existingToken.TokenMetadata) < len(candidateToken.TokenMetadata) {
				seenTokens[ti] = candidateToken
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

			ownership := fromMultichainToAddressAtBlock(token.OwnershipHistory)
			seenToken := seenTokens[ti]
			seenToken.OwnershipHistory = ownership
			seenToken.OwnedByWallets = seenWallets[ti]
			seenToken.Quantity = seenQuantities[ti]
			seenTokens[ti] = seenToken
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

	return res, nil
}

func contractsToNewDedupedContracts(contracts []chainContracts) []persist.ContractGallery {
	seen := make(map[persist.ChainAddress]persist.ContractGallery)

	sort.SliceStable(contracts, func(i, j int) bool {
		return contracts[i].priority < contracts[j].priority
	})

	for _, chainContract := range contracts {
		for _, contract := range chainContract.contracts {
			if it, ok := seen[persist.NewChainAddress(contract.Address, chainContract.chain)]; ok {
				if it.Name != "" {
					continue
				}
			}
			c := persist.ContractGallery{
				Chain:          chainContract.chain,
				Address:        contract.Address,
				Symbol:         persist.NullString(contract.Symbol),
				Name:           persist.NullString(contract.Name),
				CreatorAddress: contract.CreatorAddress,
			}
			seen[persist.NewChainAddress(contract.Address, chainContract.chain)] = c
		}
	}

	res := make([]persist.ContractGallery, 0, len(seen))
	for _, c := range seen {
		res = append(res, c)
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
		res[i] = addr.ToAddressAtBlock()
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
