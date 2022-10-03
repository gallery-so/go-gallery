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

var (
	TezImageKeywords     = []string{"displayUri", "image", "thumbnailUri", "artifactUri", "uri"}
	TezAnimationKeywords = []string{"artifactUri", "displayUri", "uri", "image"}
)

type keywordsForChain struct {
	imageKeywords     []string
	animationKeywords []string
}

var allKeywordsForChain = map[persist.Chain]keywordsForChain{
	persist.ChainTezos: {
		imageKeywords:     TezImageKeywords,
		animationKeywords: TezAnimationKeywords,
	},
}

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
	UserID        persist.DBID   `json:"user_id"`
	DisplayName   string         `json:"display_name"`
	WalletIDs     []persist.DBID `json:"wallet_ids"`
	PreviewTokens []string       `json:"preview_tokens"`
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
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
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
func NewMultiChainDataRetriever(ctx context.Context, repos *persist.Repositories, cache memstore.Cache, taskClient *cloudtasks.Client, chainOverrides ChainOverrideMap, chains ...ChainProvider) *Provider {
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
						tokens, contracts, err := provider.GetTokensByWalletAddress(ctx, addr)
						if err != nil {
							errChan <- errWithPriority{err: err, priority: priority}
							return
						}

						logger.For(ctx).Infof("got %d tokens and %d contracts for address %s on chain %d for priority %d", len(tokens), len(contracts), addr, chain, priority)

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

	newContracts, err := contractsToNewDedupedContracts(ctx, allContracts)
	if err := p.Repos.ContractRepository.BulkUpsert(ctx, newContracts); err != nil {
		return fmt.Errorf("error upserting contracts: %s", err)
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
		newContracts, err := p.Repos.ContractRepository.GetByAddresses(ctx, addresses, chain)
		if err != nil {
			return err
		}
		for _, c := range newContracts {
			addressesToContracts[c.Chain.NormalizeAddress(c.Address)] = c.ID
		}
	}

	allUsersNFTs, err := p.Repos.TokenRepository.GetByUserID(ctx, userID, 0, 0)
	if err != nil {
		return err
	}

	newTokens, err := tokensToNewDedupedTokens(ctx, allTokens, addressesToContracts, user)
	if err != nil {
		return err
	}
	newTokens = addExistingMedia(ctx, newTokens, allUsersNFTs)

	logger.For(ctx).Debugf("upserting %d tokens for user %s", len(newTokens), user.Username)
	if err := p.Repos.TokenRepository.BulkUpsert(ctx, newTokens); err != nil {
		return fmt.Errorf("error upserting tokens: %s", err)
	}

	for _, chain := range chains {
		if keywords, ok := allKeywordsForChain[chain]; ok {
			err = p.processMedialessTokens(ctx, userID, chain, keywords.imageKeywords, keywords.animationKeywords)
			if err != nil {
				logger.For(ctx).Errorf("error processing medialess tokens for user %s: %s", user.Username, err)
				return err
			}
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
	processMediaInput := task.MediaProcessingMessage{
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/process/token", viper.GetString("MEDIA_PROCESSING_URL")), bytes.NewBuffer(asJSON))
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
	providers, ok := d.Chains[communityIdentifiers.Chain()]
	if !ok {
		return nil, ErrChainNotFound{Chain: communityIdentifiers.Chain()}
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
			if !seenAddresses[persist.Address(holder.DisplayName)] {
				h := holder
				wp.Submit(func() {
					innerCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
					defer cancel()
					timeHere := time.Now()
					tokensForContract, _, err := providers[0].GetOwnedTokensByContract(innerCtx, communityIdentifiers.Address(), persist.Address(h.DisplayName))
					if err != nil {
						logger.For(ctx).Errorf("error getting tokens for contract %s and address %s: %s", communityIdentifiers.Address(), h.DisplayName, err)
						return
					}
					if len(tokensForContract) > 0 {
						h.PreviewTokens = []string{tokensForContract[0].Media.MediaURL.String()}
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
	providers, ok := d.Chains[ti.Chain]
	if !ok {
		return ErrChainNotFound{Chain: ti.Chain}
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
				if !token.Media.IsServable() {
					if keywords, ok := allKeywordsForChain[ti.Chain]; ok {
						err = d.processMedialessToken(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, token.OwnerAddress, keywords.imageKeywords, keywords.animationKeywords)
						if err != nil {
							return err
						}
					}
				}

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
	providers, ok := d.Chains[ci.Chain]
	if !ok {
		return ErrChainNotFound{Chain: ci.Chain}
	}
	for _, provider := range providers {
		if err := provider.RefreshContract(ctx, ci.ContractAddress); err != nil {
			return err
		}
	}
	return nil
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
				DisplayName: owner.Address.String(),
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
