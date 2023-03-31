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

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
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
	Queries *coredb.Queries
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

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Address        persist.Address `json:"address"`
	Symbol         string          `json:"symbol"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	CreatorAddress persist.Address `json:"creator_address"`

	LatestBlock persist.BlockNumber `json:"latest_block"`
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

// ErrChainNotFound is an error that occurs when a chain provider for a given chain is not registered in the MultichainProvider
type ErrChainNotFound struct {
	Chain persist.Chain
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

// tokensOwnerFetcher supports fetching tokens for syncing
type tokensOwnerFetcher interface {
	GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

type tokensContractFetcher interface {
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

// contractRefresher supports refreshes of a contract
type contractRefresher interface {
	RefreshContract(context.Context, persist.Address) error
}

// deepRefresher supports deep refreshes
type deepRefresher interface {
	DeepRefresh(ctx context.Context, address persist.Address) error
}

// tokenMetadataFetcher supports fetching token metadata
type tokenMetadataFetcher interface {
	GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers, ownerAddress persist.Address) (persist.TokenMetadata, error)
}

type subproviderProvider interface {
	GetSubproviders() []any
}

type ChainOverrideMap = map[persist.Chain]*persist.Chain

// NewProvider creates a new MultiChainDataRetriever
func NewProvider(ctx context.Context, repos *postgres.Repositories, queries *coredb.Queries, cache *redis.Cache, taskClient *cloudtasks.Client, chainOverrides ChainOverrideMap, providers ...any) *Provider {
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

func getChainProvidersForTask[T any](providers []any) []T {
	result := make([]T, 0, len(providers))
	for _, p := range providers {
		if provider, ok := p.(T); ok {
			result = append(result, provider)
		} else if subproviders, ok := p.(subproviderProvider); ok {
			for _, subprovider := range subproviders.GetSubproviders() {
				if provider, ok := subprovider.(T); ok {
					result = append(result, provider)
				}
			}
		}
	}
	return result
}

func hasProvidersForTask[T any](providers []any) bool {
	for _, p := range providers {
		if _, ok := p.(T); ok {
			return true
		} else if subproviders, ok := p.(subproviderProvider); ok {
			for _, subprovider := range subproviders.GetSubproviders() {
				if _, ok := subprovider.(T); ok {
					return true
				}
			}
		}
	}
	return false
}

var chainValidation map[persist.Chain]validation = map[persist.Chain]validation{
	persist.ChainETH: {
		nameResolver:          true,
		verifier:              true,
		tokensOwnerFetcher:    true,
		tokensContractFetcher: true,
		contractRefresher:     true,
		tokenMetadataFetcher:  true,
	},
	persist.ChainTezos: {
		tokensOwnerFetcher: true,
	},
	persist.ChainPOAP: {
		nameResolver:       true,
		tokensOwnerFetcher: true,
	},
}

type validation struct {
	nameResolver          bool
	verifier              bool
	tokensOwnerFetcher    bool
	tokensContractFetcher bool
	tokenMetadataFetcher  bool
	contractRefresher     bool
}

func validateProviders(ctx context.Context, providers []any) map[persist.Chain][]any {
	chains := map[persist.Chain][]any{}

	configurers := getChainProvidersForTask[configurer](providers)
	for _, cfg := range configurers {
		info, err := cfg.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		chains[info.Chain] = append(chains[info.Chain], cfg)
	}

	for chain, providers := range chains {
		requirements, ok := chainValidation[chain]
		if !ok {
			logger.For(ctx).Warnf("chain=%d has no provider validation", chain)
			continue
		}

		hasImplementor := validation{}

		if hasNameResolver := hasProvidersForTask[nameResolver](providers); hasNameResolver {
			hasImplementor.nameResolver = true
			requirements.nameResolver = true
		}

		if hasVerifier := hasProvidersForTask[verifier](providers); hasVerifier {
			hasImplementor.verifier = true
			requirements.verifier = true
		}

		if hasTokensOwnerFetcher := hasProvidersForTask[tokensOwnerFetcher](providers); hasTokensOwnerFetcher {
			hasImplementor.tokensOwnerFetcher = true
			requirements.tokensOwnerFetcher = true
		}

		if hasTokensContractFetcher := hasProvidersForTask[tokensContractFetcher](providers); hasTokensContractFetcher {
			hasImplementor.tokensContractFetcher = true
			requirements.tokensContractFetcher = true
		}

		if hasContractRefresher := hasProvidersForTask[contractRefresher](providers); hasContractRefresher {
			hasImplementor.contractRefresher = true
			requirements.contractRefresher = true
		}

		if hasTokenMetadataFetcher := hasProvidersForTask[tokenMetadataFetcher](providers); hasTokenMetadataFetcher {
			hasImplementor.tokenMetadataFetcher = true
			requirements.tokenMetadataFetcher = true
		}

		if hasImplementor != requirements {
			panic(fmt.Sprintf("chain=%d;got=%+v;want=%+v", chain, hasImplementor, requirements))
		}
	}

	return chains
}

// SyncTokens updates the media for all tokens for a user
func (p *Provider) SyncTokens(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := p.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	validChainsLookup := make(map[persist.Chain]bool)
	for _, chain := range chains {
		validChainsLookup[chain] = true
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
				providers, err := p.getProvidersForChain(chain)
				if err != nil {
					errChan <- err
					return
				}

				tokenFetchers := getChainProvidersForTask[tokensOwnerFetcher](providers)
				subWg := &sync.WaitGroup{}
				for i, p := range tokenFetchers {
					subWg.Add(1)
					go func(fetcher tokensOwnerFetcher, priority int) {
						defer subWg.Done()
						tokens, contracts, err := fetcher.GetTokensByWalletAddress(ctx, addr, 0, 0)
						if err != nil {
							errChan <- err
							return
						}

						logger.For(ctx).Debugf("got %d tokens and %d contracts from provider %d", len(tokens), len(contracts), priority)

						incomingTokens <- chainTokens{chain: chain, tokens: tokens, priority: priority}
						incomingContracts <- chainContracts{chain: chain, contracts: contracts, priority: priority}
					}(p, i)
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

	errs := []error{}
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
			logger.For(ctx).Errorf("error while syncing tokens for user %s: %s", user.Username, err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 && len(tokensFromProviders) == 0 {
		return util.MultiErr(errs)
	}

	addressToContract, err := p.processContracts(ctx, contractsFromProviders)
	if err != nil {
		return err
	}

	_, err = p.processTokensForUser(ctx, tokensFromProviders, addressToContract, user, chains, false)
	return err
}

func (p *Provider) prepTokensForTokenProcessing(ctx context.Context, tokensFromProviders []chainTokens, addressToContract map[string]persist.DBID, user persist.User) ([]persist.TokenGallery, map[persist.TokenIdentifiers]bool, error) {
	providerTokens, err := tokensToNewDedupedTokens(ctx, tokensFromProviders, addressToContract, user)
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

func (p *Provider) processTokensForOwnersOfContract(ctx context.Context, contractID persist.DBID, users map[persist.DBID]persist.User, chainTokensForUsers map[persist.DBID][]chainTokens, addressToContract map[string]persist.DBID) error {
	tokensToUpsert := make([]persist.TokenGallery, 0, len(chainTokensForUsers)*3)
	userTokenOffsets := make(map[persist.DBID][2]int)
	newUserTokens := make(map[persist.DBID]map[persist.TokenIdentifiers]bool)

	for userID, user := range users {
		tokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, chainTokensForUsers[userID], addressToContract, user)
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

func (p *Provider) processTokensForUser(ctx context.Context, tokensFromProviders []chainTokens, addressToContract map[string]persist.DBID, user persist.User, chains []persist.Chain, skipDelete bool) ([]persist.TokenGallery, error) {
	dedupedTokens, newTokens, err := p.prepTokensForTokenProcessing(ctx, tokensFromProviders, addressToContract, user)
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
	input := map[string]any{
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

	providers, err := p.getProvidersForChain(wallet.Chain())
	if err != nil {
		return nil, err
	}

	contractFetchers := getChainProvidersForTask[tokensContractFetcher](providers)

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

	addressToContract, err := p.processContracts(ctx, contracts)
	if err != nil {
		return nil, err
	}

	return p.processTokensForUser(ctx, tokensFromProviders, addressToContract, user, []persist.Chain{wallet.Chain()}, true)
}

// GetTokenMetadataByTokenIdentifiers will get the metadata for a given token identifier
func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, ownerAddress persist.Address, chain persist.Chain) (persist.TokenMetadata, error) {

	var metadata persist.TokenMetadata
	var err error

	metadataFetchers := getChainProvidersForTask[tokenMetadataFetcher](d.Chains[chain])

	for _, metadataFetcher := range metadataFetchers {
		metadata, err = metadataFetcher.GetTokenMetadataByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{ContractAddress: contractAddress, TokenID: tokenID}, ownerAddress)
		if err == nil && len(metadata) > 0 {
			return metadata, nil
		}
	}

	return metadata, err
}

// DeepRefresh re-indexes a user's wallets.
func (d *Provider) DeepRefreshByChain(ctx context.Context, userID persist.DBID, chain persist.Chain) error {
	if _, ok := d.Chains[chain]; !ok {
		return nil
	}

	// User doesn't exist
	user, err := d.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	addresses := make([]persist.Address, 0)
	for _, wallet := range user.Wallets {
		if wallet.Chain == chain {
			addresses = append(addresses, wallet.Address)
		}
	}

	deepRefreshers := getChainProvidersForTask[deepRefresher](d.Chains[chain])

	for _, refresher := range deepRefreshers {
		for _, wallet := range addresses {
			if err := refresher.DeepRefresh(ctx, wallet); err != nil {
				return err
			}
		}
	}

	return nil
}

// RunWalletCreationHooks runs hooks for when a wallet is created
func (d *Provider) RunWalletCreationHooks(ctx context.Context, userID persist.DBID, walletAddress persist.Address, walletType persist.WalletType, chain persist.Chain) error {

	// User doesn't exist
	_, err := d.Repos.UserRepository.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	walletHookers := getChainProvidersForTask[walletHooker](d.Chains[chain])

	for _, hooker := range walletHookers {
		if err := hooker.WalletCreated(ctx, userID, walletAddress, walletType); err != nil {
			return err
		}
	}

	return nil
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	providers, err := p.getProvidersForChain(pChainAddress.Chain())
	if err != nil {
		return false, err
	}
	verifiers := getChainProvidersForTask[verifier](providers)
	for _, verifier := range verifiers {
		if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
			return false, err
		}
	}
	return true, nil
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (p *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers, ownerAddresses []persist.Address) error {
	providers, err := p.getProvidersForChain(ti.Chain)
	if err != nil {
		return err
	}

	ownerFetchers := getChainProvidersForTask[tokensOwnerFetcher](providers)
outer:
	for _, ownerFetcher := range ownerFetchers {

		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}

		for i, ownerAddress := range ownerAddresses {
			refreshedToken, contract, err := ownerFetcher.GetTokensByTokenIdentifiersAndOwner(ctx, id, ownerAddress)
			if err != nil {
				return err
			}

			if !refreshedToken.hasMetadata() && i == 0 {
				continue outer
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
		return nil
	}
	return nil
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (p *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	providers, err := p.getProvidersForChain(ci.Chain)
	if err != nil {
		return err
	}

	contractRefreshers := getChainProvidersForTask[contractRefresher](providers)
	for _, refresher := range contractRefreshers {
		if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	return nil
}

// RefreshTokensForContract refreshes all tokens in a given contract
func (p *Provider) RefreshTokensForContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	providers, err := p.getProvidersForChain(ci.Chain)
	if err != nil {
		return err
	}

	contractRefreshers := getChainProvidersForTask[tokensContractFetcher](providers)

	tokensFromProviders := []chainTokens{}
	contractsFromProviders := []chainContracts{}
	tokensReceive := make(chan chainTokens)
	contractsReceive := make(chan chainContracts)
	errChan := make(chan errWithPriority)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}

	for i, fetcher := range contractRefreshers {

		wg.Add(1)
		go func(priority int, p tokensContractFetcher) {
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

	addressToContract, err := p.processContracts(ctx, contractsFromProviders)
	if err != nil {
		return err
	}

	contract, err := p.Queries.GetContractByChainAddress(ctx, coredb.GetContractByChainAddressParams{
		Address: ci.ContractAddress,
		Chain:   ci.Chain,
	})
	if err != nil {
		return err
	}

	return p.processTokensForOwnersOfContract(ctx, contract.ID, users, chainTokensForUsers, addressToContract)
}

func (d *Provider) getProvidersForChain(chain persist.Chain) ([]any, error) {
	providers, ok := d.Chains[chain]
	if !ok {
		return nil, ErrChainNotFound{Chain: chain}
	}

	return providers, nil
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
func (p *Provider) createUsersForTokens(ctx context.Context, tokens []chainTokens, chain persist.Chain) (map[persist.DBID][]chainTokens, map[persist.DBID]persist.User, error) {
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

	allCurrentUsers, err := p.Queries.GetUsersByChainAddresses(ctx, coredb.GetUsersByChainAddressesParams{
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
		providers, err := p.getProvidersForChain(chainToken.chain)
		if err != nil {
			return nil, nil, err
		}

		nameResolvers := getChainProvidersForTask[nameResolver](providers)

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
					for _, resolver := range nameResolvers {
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

func (d *Provider) processContracts(ctx context.Context, contractsFromProviders []chainContracts) (map[string]persist.DBID, error) {
	newContracts, err := contractsToNewDedupedContracts(ctx, contractsFromProviders)
	if err != nil {
		return nil, err
	}
	if err := d.Repos.ContractRepository.BulkUpsert(ctx, newContracts); err != nil {
		return nil, fmt.Errorf("error upserting contracts: %s", err)
	}
	contractsForChain := map[persist.Chain][]persist.Address{}
	for _, c := range newContracts {
		if _, ok := contractsForChain[c.Chain]; !ok {
			contractsForChain[c.Chain] = []persist.Address{}
		}
		contractsForChain[c.Chain] = append(contractsForChain[c.Chain], c.Address)
	}

	addressesToContracts := map[string]persist.DBID{}
	for chain, addresses := range contractsForChain {
		newContracts, err := d.Repos.ContractRepository.GetByAddresses(ctx, addresses, chain)
		if err != nil {
			return nil, fmt.Errorf("error fetching contracts: %s", err)
		}
		for _, c := range newContracts {
			addressesToContracts[c.Chain.NormalizeAddress(c.Address)] = c.ID
		}
	}
	return addressesToContracts, nil
}

func tokensToNewDedupedTokens(ctx context.Context, tokens []chainTokens, contractAddressIDs map[string]persist.DBID, ownerUser persist.User) ([]persist.TokenGallery, error) {
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
				TokenType:            token.TokenType,
				Chain:                chainToken.chain,
				Name:                 persist.NullString(token.Name),
				Description:          persist.NullString(token.Description),
				TokenURI:             "", // We don't save tokenURI information
				TokenID:              token.TokenID,
				OwnerUserID:          ownerUser.ID,
				TokenMetadata:        token.TokenMetadata,
				Contract:             contractAddressIDs[chainToken.chain.NormalizeAddress(token.ContractAddress)],
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

			ownership, err := addressAtBlockToAddressAtBlock(ctx, token.OwnershipHistory, chainToken.chain)
			if err != nil {
				return nil, fmt.Errorf("failed to get ownership history for token: %s", err)
			}

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

func contractsToNewDedupedContracts(ctx context.Context, contracts []chainContracts) ([]persist.ContractGallery, error) {
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
	return res, nil

}

func communityOwnersToTokenHolders(owners []ChainAgnosticCommunityOwner) []TokenHolder {
	seen := make(map[persist.Address]TokenHolder)
	for _, owner := range owners {
		if _, ok := seen[owner.Address]; !ok {
			seen[owner.Address] = TokenHolder{
				Address: owner.Address,
			}
		}
	}

	res := make([]TokenHolder, 0, len(seen))
	for _, t := range seen {
		res = append(res, t)
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

func addressAtBlockToAddressAtBlock(ctx context.Context, addresses []ChainAgnosticAddressAtBlock, chain persist.Chain) ([]persist.AddressAtBlock, error) {
	res := make([]persist.AddressAtBlock, len(addresses))
	for i, addr := range addresses {

		res[i] = persist.AddressAtBlock{
			Address: addr.Address,
			Block:   addr.Block,
		}
	}
	return res, nil
}

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("%s-%s", t.ContractAddress, t.TokenID)
}

func (e ErrChainNotFound) Error() string {
	return fmt.Sprintf("chain provider not found for chain: %d", e.Chain)
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
