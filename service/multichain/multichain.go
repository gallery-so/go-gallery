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

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const staleCommunityTime = time.Hour * 2

const maxCommunitySize = 10_000

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	Repos  *persist.Repositories
	Cache  memstore.Cache
	Chains map[persist.Chain][]ChainProvider
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

// ChainProvider is an interface for retrieving data from a chain
type ChainProvider interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
	GetTokensByWalletAddress(context.Context, persist.Address, int, int) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByContractAddress(context.Context, persist.Address, int, int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers, int, int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
	GetOwnedTokensByContract(context.Context, persist.Address, persist.Address, int, int) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetContractByAddress(context.Context, persist.Address) (ChainAgnosticContract, error)
	GetCommunityOwners(context.Context, persist.Address, int, int) ([]ChainAgnosticCommunityOwner, error)
	GetDisplayNameByAddress(context.Context, persist.Address) string
	RefreshToken(context.Context, ChainAgnosticIdentifiers, persist.Address) error
	RefreshContract(context.Context, persist.Address) error
	// bool is whether or not to update all media content, including the tokens that already have media content
	UpdateMediaForWallet(context.Context, persist.Address, bool) error
	// do we want to return the tokens we validate?
	// bool is whether or not to update all of the tokens regardless of whether we know they exist already
	ValidateTokensForWallet(context.Context, persist.Address, bool) error
	// ctx, address, chain, wallet type, nonce, sig
	VerifySignature(context.Context, persist.PubKey, persist.WalletType, string, string) (bool, error)
}

type ChainOverrideMap = map[persist.Chain]*persist.Chain

// NewProvider creates a new MultiChainDataRetriever
func NewProvider(ctx context.Context, repos *persist.Repositories, cache memstore.Cache, taskClient *cloudtasks.Client, chainOverrides ChainOverrideMap, chains ...ChainProvider) *Provider {
	c := map[persist.Chain][]ChainProvider{}
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
					go func(provider ChainProvider, priority int) {
						defer subWg.Done()
						tokens, contracts, err := provider.GetTokensByWalletAddress(ctx, addr, 0, 0)
						if err != nil {
							errChan <- errWithPriority{err: err, priority: priority}
							return
						}

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
	newTokens, err := p.upsertTokens(ctx, allTokens, addressToContract, allUsersNFTs, user)
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

	logger.For(ctx).Warn("preparing to delete old tokens")

	// ensure all old tokens are deleted
	ownedTokens := make(map[tokenIdentifiers]bool)
	for _, t := range newTokens {
		ownedTokens[tokenIdentifiers{chain: t.Chain, tokenID: t.TokenID, contract: t.Contract}] = true
	}

	for _, nft := range allUsersNFTs {
		if !validChainsLookup[nft.Chain] {
			continue
		}
		if !ownedTokens[tokenIdentifiers{chain: nft.Chain, tokenID: nft.TokenID, contract: nft.Contract}] {
			logger.For(ctx).Warnf("deleting nft %d-%s-%s", nft.Chain, nft.TokenID, nft.Contract)
			err := p.Repos.TokenRepository.DeleteByID(ctx, nft.ID)
			if err != nil {
				return err
			}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/process/token", viper.GetString("TOKEN_PROCESSING_URL")), bytes.NewBuffer(asJSON))
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

// VerifySignature verifies a signature for a wallet address
func (p *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	providers, ok := p.Chains[pChainAddress.Chain()]
	if !ok {
		return false, ErrChainNotFound{Chain: pChainAddress.Chain()}
	}
	for _, provider := range providers {
		if valid, err := provider.VerifySignature(ctx, pChainAddress.PubKey(), pWalletType, pNonce, pSig); err != nil || !valid {
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
	for i, provider := range providers {
		id := ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
		for _, ownerAddress := range ownerAddresses {
			if err := provider.RefreshToken(ctx, id, ownerAddress); err != nil {
				return err
			}
		}

		if i == 0 {
			for _, ownerAddress := range ownerAddresses {
				token, contract, err := provider.GetTokensByTokenIdentifiersAndOwner(ctx, id, ownerAddress)
				if err != nil {
					return err
				}

				if err := p.Repos.TokenRepository.UpdateByTokenIdentifiersUnsafe(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, persist.TokenUpdateMediaInput{
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
		if err := provider.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	return nil
}

// RefreshTokensForCollection refreshes all tokens in a given collection
func (p *Provider) RefreshTokensForCollection(ctx context.Context, ci persist.ContractIdentifiers) error {
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
		go func(priority int, p ChainProvider) {
			defer wg.Done()
			tokens, contract, err := p.GetTokensByContractAddress(ctx, ci.ContractAddress, maxCommunitySize, 0)
			if err != nil {
				errChan <- errWithPriority{priority: priority, err: err}
				return
			}
			tokensReceive <- chainTokens{chain: ci.Chain, tokens: tokens, priority: priority}
			contractsReceive <- chainContracts{chain: ci.Chain, contracts: []ChainAgnosticContract{contract}, priority: priority}

		}(i, provider)
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

	chainTokensForUsers, users, err := p.createUsersForTokens(ctx, allTokens)
	if err != nil {
		return err
	}

	logger.For(ctx).Debug("creating contracts")

	addressToContract, err := p.upsertContracts(ctx, allContracts)
	if err != nil {
		return err
	}

	logger.For(ctx).Debug("creating tokens")

	for _, user := range users {
		allUserTokens, err := p.Repos.TokenRepository.GetByUserID(ctx, user.ID, -1, 0)
		if err != nil {
			return err
		}

		logger.For(ctx).Debugf("creating tokens for user %s", user.Username)
		_, err = p.upsertTokens(ctx, chainTokensForUsers[user.ID], addressToContract, allUserTokens, user)
		if err != nil {
			return err
		}

		logger.For(ctx).Debugf("creating tokens for user %s done", user.Username)
	}
	return nil
}

func (d *Provider) getProvidersForChain(chain persist.Chain) ([]ChainProvider, error) {
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
func (p *Provider) createUsersForTokens(ctx context.Context, tokens []chainTokens) (map[persist.DBID][]chainTokens, map[persist.DBID]persist.User, error) {
	users := map[persist.DBID]persist.User{}
	userTokens := map[persist.DBID]map[int]chainTokens{}
	seenTokens := map[tokenUniqueIdentifiers]bool{}

	userChan := make(chan persist.User)
	tokensForUserChan := make(chan tokenForUser)
	errChan := make(chan error)
	done := make(chan struct{})
	wp := workerpool.New(100)

	mu := &sync.Mutex{}

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
				user, err := p.Repos.UserRepository.GetByChainAddress(ctx, persist.NewChainAddress(t.OwnerAddress, ct.chain))
				if err != nil || user.ID == "" {
					username := t.OwnerAddress.String()
					for _, provider := range providers {
						doBreak := func() bool {
							displayCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
							defer cancel()
							display := provider.GetDisplayNameByAddress(displayCtx, t.OwnerAddress)
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

func (p *Provider) upsertTokens(ctx context.Context, allTokens []chainTokens, addressesToContracts map[string]persist.DBID, allUsersTokens []persist.TokenGallery, user persist.User) ([]persist.TokenGallery, error) {

	newTokens, err := tokensToNewDedupedTokens(ctx, allTokens, addressesToContracts, allUsersTokens, user)
	if err != nil {
		return nil, err
	}

	newTokens = addExistingMedia(ctx, newTokens, allUsersTokens)

	if err := p.Repos.TokenRepository.BulkUpsert(ctx, newTokens); err != nil {
		return nil, fmt.Errorf("error upserting tokens: %s", err)
	}
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
			return nil, fmt.Errorf("error upserting tokens: %s", err)
		}
		for _, c := range newContracts {
			addressesToContracts[c.Chain.NormalizeAddress(c.Address)] = c.ID
		}
	}
	return addressesToContracts, nil
}

func tokensToNewDedupedTokens(ctx context.Context, tokens []chainTokens, contractAddressIDs map[string]persist.DBID, dbTokens []persist.TokenGallery, ownerUser persist.User) ([]persist.TokenGallery, error) {
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
		logger.For(ctx).Debugf("token: %s", t.Name)
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

func tokenHoldersToTokenHolders(ctx context.Context, owners []persist.TokenHolder, userRepo persist.UserRepository) ([]TokenHolder, error) {
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
