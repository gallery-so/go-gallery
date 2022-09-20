package multichain

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

const staleCommunityTime = time.Hour * 2

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	Repos  *persist.Repositories
	Cache  memstore.Cache
	Chains map[persist.Chain][]ChainProvider
	// some chains use the addresses of other chains, this will map of chain we want tokens from => chain that's address will be used for lookup
	ChainAddressOverrides ChainOverrideMap
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
	GetTokensByWalletAddress(context.Context, persist.Address) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByContractAddress(context.Context, persist.Address) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetOwnedTokensByContract(context.Context, persist.Address, persist.Address) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetContractByAddress(context.Context, persist.Address) (ChainAgnosticContract, error)
	GetCommunityOwners(context.Context, persist.Address) ([]ChainAgnosticCommunityOwner, error)
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

// NewMultiChainDataRetriever creates a new MultiChainDataRetriever
func NewMultiChainDataRetriever(ctx context.Context, repos *persist.Repositories, cache memstore.Cache, chainOverrides ChainOverrideMap, chains ...ChainProvider) *Provider {
	c := map[persist.Chain][]ChainProvider{}
	for _, chain := range chains {
		info, err := chain.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		c[info.Chain] = append(c[info.Chain], chain)
	}
	return &Provider{
		Repos: repos,
		Cache: cache,

		Chains:                c,
		ChainAddressOverrides: chainOverrides,
	}
}

// SyncTokens updates the media for all tokens for a user
// TODO consider updating contracts as well
func (d *Provider) SyncTokens(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := d.Repos.UserRepository.GetByID(ctx, userID)
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
			override := d.ChainAddressOverrides[chain]

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
				providers, ok := d.Chains[chain]
				if !ok {
					errChan <- ErrChainNotFound{Chain: chain}
					return
				}
				subWg := &sync.WaitGroup{}
				subWg.Add(len(providers))
				for i, p := range providers {
					go func(provider ChainProvider, priority int) {
						defer subWg.Done()
						tokens, contracts, err := provider.GetTokensByWalletAddress(ctx, addr)
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
				logger.For(ctx).Errorf("error updating fallback media for user %s: %w", user.Username, err)
			} else {
				return err
			}
		}
	}

	allUsersNFTs, err := d.Repos.TokenRepository.GetByUserID(ctx, userID, 0, 0)
	if err != nil {
		return err
	}

	addressToContract, err := d.upsertContracts(ctx, allContracts)
	if err != nil {
		return err
	}

	newTokens, err := d.upsertTokens(ctx, allTokens, addressToContract, allUsersNFTs, user)
	if err != nil {
		return err
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
			logger.For(ctx).Warnf("deleting nft %s-%s-%s", nft.Chain, nft.TokenID, nft.Contract)
			err := d.Repos.TokenRepository.DeleteByID(ctx, nft.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Provider) GetCommunityOwners(ctx context.Context, communityIdentifiers persist.ChainAddress, onlyGalleryUsers bool, forceRefresh bool) ([]TokenHolder, error) {

	cacheKey := fmt.Sprintf("%s-%t", communityIdentifiers.String(), onlyGalleryUsers)
	if !forceRefresh {
		bs, err := d.Cache.Get(ctx, cacheKey)
		if err == nil && len(bs) > 0 {
			var owners []TokenHolder
			err = json.Unmarshal(bs, &owners)
			if err != nil {
				return nil, err
			}
			return owners, nil
		}
	}
	providers, err := d.getProvidersForChain(communityIdentifiers.Chain())
	if err != nil {
		return nil, err
	}

	dbHolders, err := d.Repos.ContractRepository.GetOwnersByAddress(ctx, communityIdentifiers.Address(), communityIdentifiers.Chain())
	if err != nil {
		return nil, err
	}

	holders, err := tokenHoldersToTokenHolders(ctx, dbHolders, d.Repos.UserRepository)

	if !onlyGalleryUsers {
		// look for other holders from the provider directly
		var nonGalleryOwners []ChainAgnosticCommunityOwner
		for _, p := range providers {
			owners, err := p.GetCommunityOwners(ctx, communityIdentifiers.Address())
			if err != nil {
				return nil, err
			}
			nonGalleryOwners = append(nonGalleryOwners, owners...)
		}
		asHolders := communityOwnersToTokenHolders(nonGalleryOwners)
		seenAddresses := map[persist.Address]bool{}
		for _, h := range holders {
			for _, w := range h.WalletIDs {
				wallet, err := d.Repos.WalletRepository.GetByID(ctx, w)
				if err == nil {
					seenAddresses[wallet.Address] = true
				}
			}
		}
		wp := workerpool.New(10)
		holderChan := make(chan TokenHolder)
		for _, holder := range asHolders {
			if !seenAddresses[holder.Address] {
				h := holder
				wp.Submit(func() {
					innerCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
					defer cancel()
					timeHere := time.Now()
					userID, err := d.Repos.UserRepository.Create(innerCtx, persist.CreateUserInput{
						// TODO how to get ENS here?
						Username:     h.Address.String(),
						ChainAddress: persist.NewChainAddress(h.Address, communityIdentifiers.Chain()),
						Universal:    true,
					})
					user, err := d.Repos.UserRepository.GetByID(innerCtx, userID)
					if err != nil {
						logger.For(ctx).Errorf("error getting user: %s", err)
						return
					}

					tokens, contracts, err := providers[0].GetOwnedTokensByContract(innerCtx, communityIdentifiers.Address(), persist.Address(h.Address))
					if err != nil {
						logger.For(ctx).Errorf("error getting tokens for contract %s and address %s: %s", communityIdentifiers.Address(), h.Address, err)
						return
					}
					cToken := chainTokens{chain: communityIdentifiers.Chain(), tokens: tokens, priority: 0}
					cContract := chainContracts{chain: communityIdentifiers.Chain(), contracts: []ChainAgnosticContract{contracts}, priority: 0}
					addressToContract, err := d.upsertContracts(innerCtx, []chainContracts{cContract})
					if err != nil {
						logger.For(ctx).Errorf("error upserting contracts: %s", err)
						return
					}
					_, err = d.upsertTokens(innerCtx, []chainTokens{cToken}, addressToContract, []persist.TokenGallery{}, user)
					if err != nil {
						logger.For(ctx).Errorf("error upserting tokens: %s", err)
						return
					}
					if len(tokens) > 0 {
						previews := make([]string, int(math.Min(3, float64(len(tokens)))))
						for i, t := range tokens {
							if i > 2 {
								break
							}
							previews[i] = t.Media.MediaURL.String()
						}
						h.PreviewTokens = previews
					}
					holderChan <- h
					logger.For(ctx).Infof("appended owner with previews in %s", time.Since(timeHere))
				})
			}
		}
		go func() {
			defer close(holderChan)
			wp.StopWait()
		}()
		for h := range holderChan {
			holders = append(holders, h)
		}
	}

	bs, err := json.Marshal(holders)
	if err != nil {
		return nil, err
	}
	err = d.Cache.Set(ctx, cacheKey, bs, staleCommunityTime)
	if err != nil {
		return nil, err
	}
	return holders, nil
}

// VerifySignature verifies a signature for a wallet address
func (d *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainPubKey, pWalletType persist.WalletType) (bool, error) {
	providers, ok := d.Chains[pChainAddress.Chain()]
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
func (d *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers, ownerAddresses []persist.Address) error {
	providers, err := d.getProvidersForChain(ti.Chain)
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
			tokens, contracts, err := provider.GetTokensByTokenIdentifiers(ctx, id)
			if err != nil {
				return err
			}
			for _, token := range tokens {
				if err := d.Repos.TokenRepository.UpdateByTokenIdentifiersUnsafe(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, persist.TokenUpdateMediaInput{
					Media:       token.Media,
					Metadata:    token.TokenMetadata,
					Name:        persist.NullString(token.Name),
					LastUpdated: persist.LastUpdatedTime{},
					TokenURI:    token.TokenURI,
					Description: persist.NullString(token.Description),
				}); err != nil {
					return err
				}
			}
			for _, contract := range contracts {
				if err := d.Repos.ContractRepository.UpsertByAddress(ctx, ti.ContractAddress, ti.Chain, persist.ContractGallery{
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
func (d *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	providers, err := d.getProvidersForChain(ci.Chain)
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
func (d *Provider) RefreshTokensForCollection(ctx context.Context, ci persist.ContractIdentifiers) error {
	providers, err := d.getProvidersForChain(ci.Chain)
	if err != nil {
		return err
	}

	allTokens := []chainTokens{}
	allContracts := []chainContracts{}
	tokensReceive := make(chan chainTokens)
	contractsReceive := make(chan chainContracts)
	errChan := make(chan errWithPriority)
	wg := &sync.WaitGroup{}
	wg.Add(len(providers))
	for i, provider := range providers {
		go func(priority int, p ChainProvider) {
			defer wg.Done()
			tokens, contract, err := p.GetTokensByContractAddress(ctx, ci.ContractAddress)
			if err != nil {
				errChan <- errWithPriority{priority: priority, err: err}
				return
			}
			tokensReceive <- chainTokens{chain: ci.Chain, tokens: tokens, priority: priority}
			contractsReceive <- chainContracts{chain: ci.Chain, contracts: []ChainAgnosticContract{contract}, priority: priority}
		}(i, provider)
	}
	go func() {
		defer close(tokensReceive)
		defer close(contractsReceive)
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
		case contract, ok := <-contractsReceive:
			if !ok {
				break outer
			}
			allContracts = append(allContracts, contract)
		}
	}

	addressToContract, err := d.upsertContracts(ctx, allContracts)
	if err != nil {
		return err
	}

	chainTokensForUsers, users, err := d.createUsersForTokens(ctx, allTokens)
	if err != nil {
		return err
	}
	for _, user := range users {
		allUserTokens, err := d.Repos.TokenRepository.GetByUserID(ctx, user.ID, -1, 0)
		if err != nil {
			return err
		}

		_, err = d.upsertTokens(ctx, chainTokensForUsers[user.ID], addressToContract, allUserTokens, user)
		if err != nil {
			return err
		}
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
	chain          persist.Chain
	contract       persist.Address
	tokenID        persist.TokenID
	ownerAddresses persist.Address
}

func (d *Provider) createUsersForTokens(ctx context.Context, tokens []chainTokens) (map[persist.DBID][]chainTokens, map[persist.DBID]persist.User, error) {
	users := map[persist.DBID]persist.User{}
	userTokens := map[persist.DBID]map[int]chainTokens{}
	seenTokens := map[tokenUniqueIdentifiers]bool{}
	seenAddresses := map[persist.Address]persist.User{}
	for _, token := range tokens {
		for _, t := range token.tokens {
			if seenTokens[tokenUniqueIdentifiers{chain: token.chain, contract: t.ContractAddress, tokenID: t.TokenID, ownerAddresses: t.OwnerAddress}] {
				continue
			}
			seenTokens[tokenUniqueIdentifiers{chain: token.chain, contract: t.ContractAddress, tokenID: t.TokenID, ownerAddresses: t.OwnerAddress}] = true
			user, ok := seenAddresses[t.OwnerAddress]
			if !ok {
				userID, err := d.Repos.UserRepository.Create(ctx, persist.CreateUserInput{
					Username:     t.OwnerAddress.String(),
					ChainAddress: persist.NewChainAddress(t.OwnerAddress, token.chain),
					Universal:    true,
				})
				if err != nil {
					if _, ok := err.(persist.ErrUsernameNotAvailable); ok {
						user, err = d.Repos.UserRepository.GetByUsername(ctx, t.OwnerAddress.String())
						if err != nil {
							return nil, nil, err
						}
					} else if _, ok := err.(persist.ErrAddressOwnedByUser); ok {
						user, err = d.Repos.UserRepository.GetByChainAddress(ctx, persist.NewChainAddress(t.OwnerAddress, token.chain))
						if err != nil {
							return nil, nil, err
						}
					} else {
						return nil, nil, err
					}
				} else {
					user, err = d.Repos.UserRepository.GetByID(ctx, userID)
					if err != nil {
						return nil, nil, err
					}
				}
				users[user.ID] = user
			}
			chainTokensForUser, ok := userTokens[user.ID]
			if !ok {
				chainTokensForUser = map[int]chainTokens{}
			}
			tokensInChainTokens, ok := chainTokensForUser[token.priority]
			if !ok {
				tokensInChainTokens = chainTokens{chain: token.chain, tokens: []ChainAgnosticToken{}, priority: token.priority}
			}
			tokensInChainTokens.tokens = append(tokensInChainTokens.tokens, t)
			chainTokensForUser[token.priority] = tokensInChainTokens
			userTokens[user.ID] = chainTokensForUser
		}
	}
	chainTokensForUser := map[persist.DBID][]chainTokens{}
	for userID, chainTokens := range userTokens {
		for _, chainToken := range chainTokens {
			chainTokensForUser[userID] = append(chainTokensForUser[userID], chainToken)
		}
	}
	return chainTokensForUser, users, nil
}

func (d *Provider) upsertTokens(ctx context.Context, allTokens []chainTokens, addressesToContracts map[string]persist.DBID, allUsersTokens []persist.TokenGallery, user persist.User) ([]persist.TokenGallery, error) {

	newTokens, err := tokensToNewDedupedTokens(ctx, allTokens, addressesToContracts, allUsersTokens, user)
	if err != nil {
		return nil, err
	}
	if err := d.Repos.TokenRepository.BulkUpsert(ctx, newTokens); err != nil {
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
		return nil, fmt.Errorf("error upserting tokens: %s", err)
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

	dbSeen := make(map[persist.TokenIdentifiers]persist.TokenGallery)
	for _, token := range dbTokens {
		dbSeen[persist.NewTokenIdentifiers(persist.Address(token.Contract), token.TokenID, token.Chain)] = token
	}

	res := make([]persist.TokenGallery, 0, len(seenTokens))
	for _, t := range seenTokens {
		if !t.Media.IsServable() {
			if dbToken, ok := dbSeen[persist.NewTokenIdentifiers(persist.Address(t.Contract), t.TokenID, t.Chain)]; ok && dbToken.Media.IsServable() {
				res = append(res, dbToken)
				continue
			}
		}
		res = append(res, t)
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
