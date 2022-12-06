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

	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const staleCommunityTime = time.Minute * 30

const maxCommunitySize = 10_000

type Provider struct {
	Repos   *postgres.Repositories
	Queries *coredb.Queries
	Cache   *redis.Cache
	Chains  map[persist.Chain][]configurer
	// some chains use the addresses of other chains, this will map of chain we want tokens from => chain that's address will be used for lookup
	ChainAddressOverrides ChainOverrideMap
	TasksClient           *cloudtasks.Client
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

// tokensFetcher supports fetching tokens for syncing
type tokensFetcher interface {
	GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
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

// deepRefresher supports deep refreshes
type deepRefresher interface {
	DeepRefresh(ctx context.Context, address persist.Address) error
}

type ChainOverrideMap = map[persist.Chain]*persist.Chain

// NewProvider creates a new MultiChainDataRetriever
func NewProvider(ctx context.Context, repos *postgres.Repositories, queries *coredb.Queries, cache *redis.Cache, taskClient *cloudtasks.Client, chainOverrides ChainOverrideMap, chains ...configurer) *Provider {
	c := map[persist.Chain][]configurer{}
	for _, chain := range chains {
		info, err := chain.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		c[info.Chain] = append(c[info.Chain], chain)
	}
	return &Provider{
		Repos:       repos,
		Cache:       cache,
		TasksClient: taskClient,
		Queries:     queries,

		Chains:                c,
		ChainAddressOverrides: chainOverrides,
	}
}

// SyncTokens updates the media for all tokens for a user
// TODO consider updating contracts as well
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
		logger.For(ctx).Infof("updating media for user %s wallets %s", user.Username, a)
		chain := c
		addresses := a
		wg.Add(len(addresses))
		for _, addr := range addresses {
			go func(addr persist.Address, chain persist.Chain) {
				defer wg.Done()
				start := time.Now()
				providers, ok := p.Chains[chain]
				if !ok {
					errChan <- ErrChainNotFound{Chain: chain}
					return
				}
				subWg := &sync.WaitGroup{}
				subWg.Add(len(providers))
				for i, p := range providers {
					if fetcher, ok := p.(tokensFetcher); ok {
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

	allTokens := make([]chainTokens, 0, len(user.Wallets))
	allContracts := make([]chainContracts, 0, len(user.Wallets))

outer:
	for {
		select {
		case incomingTokens := <-incomingTokens:
			allTokens = append(allTokens, incomingTokens)
		case incomingContracts, ok := <-incomingContracts:
			if !ok {
				break outer
			}
			allContracts = append(allContracts, incomingContracts)
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

	allUsersNFTs, err := p.Repos.TokenRepository.GetByUserID(ctx, userID, 0, 0)
	if err != nil {
		return err
	}

	addressToContract, err := p.upsertContracts(ctx, allContracts)
	if err != nil {
		return err
	}
	_, err = p.upsertTokens(ctx, allTokens, addressToContract, allUsersNFTs, user, chains)
	if err != nil {
		return err
	}

	for _, chain := range chains {
		image, anim := chain.BaseKeywords()
		err = p.processMedialessTokens(ctx, userID, chain, image, anim)
		if err != nil {
			logger.For(ctx).Errorf("error processing medialess tokens for user %s: %s", user.Username, err)
			return err
		}

	}

	return nil
}

func (p *Provider) processMedialessTokens(ctx context.Context, userID persist.DBID, chain persist.Chain, imageKeywords, animationKeywords []string) error {
	processMediaInput := task.TokenProcessingUserMessage{
		UserID:            userID,
		Chain:             chain,
		ImageKeywords:     imageKeywords,
		AnimationKeywords: animationKeywords,
	}
	return task.CreateTaskForMediaProcessing(ctx, processMediaInput, p.TasksClient)
}

func (p *Provider) processMedialessToken(ctx context.Context, tokenID persist.TokenID, contractAddress persist.Address, chain persist.Chain, ownerAddress persist.Address, imageKeywords, animationKeywords []string) error {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/media/process/token", viper.GetString("TOKEN_PROCESSING_URL")), bytes.NewBuffer(asJSON))
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

	for _, provider := range d.Chains[chain] {
		for _, wallet := range addresses {
			if refresher, ok := provider.(deepRefresher); ok {
				if err := refresher.DeepRefresh(ctx, wallet); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	providers, ok := p.Chains[pChainAddress.Chain()]
	if !ok {
		return false, ErrChainNotFound{Chain: pChainAddress.Chain()}
	}
	for _, provider := range providers {
		if verifier, ok := provider.(verifier); ok {
			if valid, err := verifier.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
				return false, err
			}
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
	for i, provider := range providers {
		refresher, ok := provider.(tokenFetcherRefresher)
		if !ok {
			continue
		}

		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
		for _, ownerAddress := range ownerAddresses {
			if err := refresher.RefreshToken(ctx, id, ownerAddress); err != nil {
				return err
			}
		}

		if i == 0 {
			for _, ownerAddress := range ownerAddresses {
				token, contract, err := refresher.GetTokensByTokenIdentifiersAndOwner(ctx, id, ownerAddress)
				if err != nil {
					return err
				}

				if err := p.Repos.TokenRepository.UpdateByTokenIdentifiersUnsafe(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, persist.TokenUpdateAllURIDerivedFieldsInput{
					Media:       token.Media,
					Metadata:    token.TokenMetadata,
					Name:        persist.NullString(token.Name),
					LastUpdated: persist.LastUpdatedTime{},
					TokenURI:    token.TokenURI,
					Description: persist.NullString(token.Description),
				}); err != nil {
					return err
				}
				if !token.Media.IsServable() {
					image, anim := ti.Chain.BaseKeywords()
					err = p.processMedialessToken(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, token.OwnerAddress, image, anim)
					if err != nil {
						return err
					}
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
	providers, err := p.getProvidersForChain(ci.Chain)
	if err != nil {
		return err
	}
	for _, provider := range providers {
		if refresher, ok := provider.(contractRefresher); ok {
			if err := refresher.RefreshContract(ctx, ci.ContractAddress); err != nil {
				return err
			}
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

	allTokens := []chainTokens{}
	allContracts := []chainContracts{}
	tokensReceive := make(chan chainTokens)
	contractsReceive := make(chan chainContracts)
	errChan := make(chan errWithPriority)
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(len(providers))
	for i, provider := range providers {
		if fetcher, ok := provider.(tokensFetcher); ok {
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
			allTokens = append(allTokens, tokens)
		case contract := <-contractsReceive:
			allContracts = append(allContracts, contract)
		case <-done:
			logger.For(ctx).Debug("done refreshing tokens for collection")
			break outer
		}
	}

	logger.For(ctx).Debug("creating users")

	chainTokensForUsers, users, err := p.createUsersForTokens(ctx, allTokens, ci.Chain)
	if err != nil {
		return err
	}

	logger.For(ctx).Debug("creating contracts")

	addressToContract, err := p.upsertContracts(ctx, allContracts)
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

	logger.For(ctx).Debug("creating tokens")
	now := time.Now()

	tokensToUpsert := make([]persist.TokenGallery, 0, len(chainTokensForUsers)*3)
	for userID, user := range users {
		allUserTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, userID, -1, 0)
		if err != nil {
			return err
		}

		logger.For(ctx).Debugf("preparing tokens for user %s", user.Username)
		tokens, err := p.prepareTokensOfContractForUser(ctx, chainTokensForUsers[userID], addressToContract, allUserTokens, user, now)
		if err != nil {
			return err
		}
		tokensToUpsert = append(tokensToUpsert, tokens...)

		logger.For(ctx).Debugf("preparing tokens for user %s done", user.Username)
	}

	if err := p.Repos.TokenRepository.BulkUpsertTokensOfContract(ctx, contract.ID, tokensToUpsert); err != nil {
		return fmt.Errorf("error deleting tokens: %s", err)
	}
	return nil
}

func (d *Provider) getProvidersForChain(chain persist.Chain) ([]configurer, error) {
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
					for _, provider := range providers {
						if resolver, ok := provider.(nameResolver); ok {
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

func (p *Provider) upsertTokens(ctx context.Context, allTokens []chainTokens, addressesToContracts map[string]persist.DBID, allUsersTokens []persist.TokenGallery, user persist.User, chains []persist.Chain) ([]persist.TokenGallery, error) {
	newTokens, err := dedupeAndPrepareTokensForUpsert(ctx, allTokens, addressesToContracts, user, allUsersTokens)
	if err != nil {
		return nil, err
	}

	if err := p.Repos.TokenRepository.BulkUpsertByOwnerUserID(ctx, user.ID, chains, newTokens); err != nil {
		return nil, fmt.Errorf("error upserting tokens: %s", err)
	}
	return newTokens, nil
}

func (p *Provider) prepareTokensOfContractForUser(ctx context.Context, allTokens []chainTokens, addressesToContracts map[string]persist.DBID, allUsersTokens []persist.TokenGallery, user persist.User, timeStamp time.Time) ([]persist.TokenGallery, error) {

	newTokens, err := dedupeAndPrepareTokensForUpsert(ctx, allTokens, addressesToContracts, user, allUsersTokens)
	if err != nil {
		return nil, err
	}

	return newTokens, nil
}

func dedupeAndPrepareTokensForUpsert(ctx context.Context, allTokens []chainTokens, addressesToContracts map[string]persist.DBID, user persist.User, allUsersTokens []persist.TokenGallery) ([]persist.TokenGallery, error) {
	newTokens, err := tokensToNewDedupedTokens(ctx, allTokens, addressesToContracts, user)
	if err != nil {
		return nil, err
	}

	newTokens = addExistingMedia(ctx, newTokens, allUsersTokens)
	return newTokens, nil
}

func (d *Provider) upsertContracts(ctx context.Context, allContracts []chainContracts) (map[string]persist.DBID, error) {
	newContracts, err := contractsToNewDedupedContracts(ctx, allContracts)
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

			// If we've never seen the incoming token before, then add it. If we have the token but its not servable
			// then we replace it entirely with the incoming token.
			if seen, ok := seenTokens[ti]; !ok || (ok && !seen.Media.IsServable() && token.Media.IsServable()) {
				seenTokens[ti] = persist.TokenGallery{
					Media:                token.Media,
					TokenType:            token.TokenType,
					Chain:                chainToken.chain,
					Name:                 persist.NullString(token.Name),
					Description:          persist.NullString(token.Description),
					TokenURI:             token.TokenURI,
					TokenID:              token.TokenID,
					OwnerUserID:          ownerUser.ID,
					TokenMetadata:        token.TokenMetadata,
					Contract:             contractAddressIDs[chainToken.chain.NormalizeAddress(token.ContractAddress)],
					ExternalURL:          persist.NullString(token.ExternalURL),
					BlockNumber:          token.BlockNumber,
					IsProviderMarkedSpam: token.IsSpam,
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

func addExistingMedia(ctx context.Context, providerTokens []persist.TokenGallery, dbTokens []persist.TokenGallery) []persist.TokenGallery {
	savedMap := make(map[persist.TokenIdentifiers]persist.TokenGallery)
	for _, token := range dbTokens {
		savedMap[token.TokenIdentifiers()] = token
	}
	res := make([]persist.TokenGallery, len(providerTokens))
	for i, t := range providerTokens {
		if !t.Media.IsServable() {
			if dbToken, ok := savedMap[t.TokenIdentifiers()]; ok && dbToken.Media.IsServable() {
				t.Media = dbToken.Media
			}
		}
		res[i] = t
	}
	return res
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

// FallbackProvider will call its fallback if the primary Provider's token
// response is unsuitable based on Eval
type FallbackProvider struct {
	Primary interface {
		configurer
		tokenFetcherRefresher
	}
	Fallback tokensFetcher
	Eval     func(context.Context, ChainAgnosticToken) bool
}

func (f FallbackProvider) GetBlockchainInfo(ctx context.Context) (BlockchainInfo, error) {
	return f.Primary.GetBlockchainInfo(ctx)
}

func (f FallbackProvider) RefreshToken(ctx context.Context, tokenIdentifiers ChainAgnosticIdentifiers, owner persist.Address) error {
	return f.Primary.RefreshToken(ctx, tokenIdentifiers, owner)
}

func (f FallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contracts, nil
}

func (f FallbackProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	tokens, agnosticContract, err := f.Primary.GetTokensByContractAddress(ctx, contract, limit, offset)
	if err != nil {
		return nil, ChainAgnosticContract{}, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, agnosticContract, nil
}

func (f FallbackProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, id ChainAgnosticIdentifiers, address persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error) {
	token, contract, err := f.Primary.GetTokensByTokenIdentifiersAndOwner(ctx, id, address)
	if err != nil {
		return ChainAgnosticToken{}, ChainAgnosticContract{}, err
	}
	if !f.Eval(ctx, token) {
		token.TokenMetadata = f.callFallback(ctx, token).TokenMetadata
	}
	return token, contract, nil
}

func (f FallbackProvider) resolveTokens(ctx context.Context, tokens []ChainAgnosticToken) []ChainAgnosticToken {
	usableTokens := make([]ChainAgnosticToken, len(tokens))
	var wg sync.WaitGroup

	for i, token := range tokens {
		wg.Add(1)
		go func(i int, token ChainAgnosticToken) {
			defer wg.Done()
			usableTokens[i] = token
			if !f.Eval(ctx, token) {
				usableTokens[i].TokenMetadata = f.callFallback(ctx, token).TokenMetadata
			}
		}(i, token)
	}

	wg.Wait()

	return usableTokens
}

func (f *FallbackProvider) callFallback(ctx context.Context, primary ChainAgnosticToken) ChainAgnosticToken {
	id := ChainAgnosticIdentifiers{primary.ContractAddress, primary.TokenID}
	backup, _, err := f.Fallback.GetTokensByTokenIdentifiersAndOwner(ctx, id, primary.OwnerAddress)
	if err == nil && f.Eval(ctx, backup) {
		return backup
	}
	logger.For(ctx).WithError(err).Warn("failed to call fallback")
	return primary
}

// ContainsTezosKeywords returns true if the token's metadata has at least one non-empty Tezos keyword.
func ContainsTezosKeywords(ctx context.Context, token ChainAgnosticToken) bool {
	imageKeywords, animationKeywords := persist.ChainTezos.BaseKeywords()
	for field, val := range token.TokenMetadata {

		for _, keyword := range imageKeywords {
			if field == keyword && (val != nil && val != "") {
				return true
			}
		}

		for _, keyword := range animationKeywords {
			if field == keyword && (val != nil && val != "") {
				return true
			}
		}
	}
	return false
}
