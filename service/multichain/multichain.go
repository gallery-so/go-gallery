package multichain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	TokenRepo    persist.TokenGalleryRepository
	ContractRepo persist.ContractGalleryRepository
	UserRepo     persist.UserRepository
	Chains       map[persist.Chain][]ChainProvider
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
	CreatorAddress persist.Address `json:"creator_address"`

	LatestBlock persist.BlockNumber `json:"latest_block"`
}

// ChainAgnosticIdentifiers identify tokens despite their chain
type ChainAgnosticIdentifiers struct {
	ContractAddress persist.Address `json:"contract_address"`
	TokenID         persist.TokenID `json:"token_id"`
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
	GetContractByAddress(context.Context, persist.Address) (ChainAgnosticContract, error)
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

// NewMultiChainDataRetriever creates a new MultiChainDataRetriever
func NewMultiChainDataRetriever(ctx context.Context, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, userRepo persist.UserRepository, chains ...ChainProvider) *Provider {
	c := map[persist.Chain][]ChainProvider{}
	for _, chain := range chains {
		info, err := chain.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		c[info.Chain] = append(c[info.Chain], chain)
	}
	return &Provider{
		TokenRepo:    tokenRepo,
		ContractRepo: contractRepo,
		UserRepo:     userRepo,

		Chains: c,
	}
}

// SyncTokens updates the media for all tokens for a user
// TODO consider updating contracts as well
func (d *Provider) SyncTokens(ctx context.Context, userID persist.DBID, chains []persist.Chain) error {
	user, err := d.UserRepo.GetByID(ctx, userID)
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

	for _, wallet := range user.Wallets {
		if validChainsLookup[wallet.Chain] {
			chainsToAddresses[wallet.Chain] = append(chainsToAddresses[wallet.Chain], wallet.Address)
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
					return err.err
				}
				logger.For(ctx).Errorf("error updating fallback media for user %s: %s", user.Username, err.err)
			} else {
				return err
			}
		}
	}

	newContracts, err := contractsToNewDedupedContracts(ctx, allContracts)
	if err := d.ContractRepo.BulkUpsert(ctx, newContracts); err != nil {
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
		newContracts, err := d.ContractRepo.GetByAddresses(ctx, addresses, chain)
		if err != nil {
			return err
		}
		for _, c := range newContracts {
			addressesToContracts[c.Chain.NormalizeAddress(c.Address)] = c.ID
		}
	}

	newTokens, err := tokensToNewDedupedTokens(ctx, allTokens, addressesToContracts, user)
	if err := d.TokenRepo.BulkUpsert(ctx, newTokens); err != nil {
		return fmt.Errorf("error upserting tokens: %s", err)
	}

	logger.For(ctx).Warn("preparing to delete old tokens")

	// ensure all old tokens are deleted
	ownedTokens := make(map[tokenIdentifiers]bool)
	for _, t := range newTokens {
		ownedTokens[tokenIdentifiers{chain: t.Chain, tokenID: t.TokenID, contract: t.Contract}] = true
	}

	allUsersNFTs, err := d.TokenRepo.GetByUserID(ctx, userID, 0, 0)
	if err != nil {
		return err
	}

	for _, nft := range allUsersNFTs {
		if !validChainsLookup[nft.Chain] {
			continue
		}
		if !ownedTokens[tokenIdentifiers{chain: nft.Chain, tokenID: nft.TokenID, contract: nft.Contract}] {
			logger.For(ctx).Warnf("deleting nft %s-%s-%s", nft.Chain, nft.TokenID, nft.Contract)
			err := d.TokenRepo.DeleteByID(ctx, nft.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
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

		tokens, contracts, err := provider.GetTokensByTokenIdentifiers(ctx, id)
		if err != nil {
			return err
		}
		if i == 0 {
			for _, token := range tokens {
				if err := d.TokenRepo.UpdateByTokenIdentifiersUnsafe(ctx, ti.TokenID, ti.ContractAddress, ti.Chain, persist.TokenUpdateMediaInput{
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
				if err := d.ContractRepo.UpsertByAddress(ctx, ti.ContractAddress, ti.Chain, persist.ContractGallery{
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

			if it, ok := seenTokens[ti]; ok {
				if !(it.Media.MediaType == persist.MediaTypeVideo && it.Media.ThumbnailURL == "") {
					if it.Media.MediaURL != "" && it.Name != "" {
						continue
					}
				} else {
					if it.Media.MediaURL != "" {
						token.Media.MediaURL = it.Media.MediaURL
					}
				}
				logger.For(ctx).Debugf("updating token %s because current version is invalid", ti)
			} else {
				if w, ok := addressToWallets[chainToken.chain.NormalizeAddress(token.OwnerAddress)]; ok {
					seenWallets[ti] = append(seenWallets[ti], w)
				}
				if q, ok := seenQuantities[ti]; ok {
					if token.Name == "aspiring chad" {
						asJSON, _ := json.MarshalIndent(token, "", "  ")
						logger.For(ctx).Warnf("t quantity again %s | %s", token.Quantity, string(asJSON))
					}
					seenQuantities[ti] = q.Add(token.Quantity)
				} else {
					if token.Name == "aspiring chad" {
						asJSON, _ := json.MarshalIndent(token, "", "  ")
						logger.For(ctx).Warnf("t quantity %s | %s", token.Quantity, string(asJSON))
					}
					seenQuantities[ti] = token.Quantity
				}
			}

			ownership, err := addressAtBlockToAddressAtBlock(ctx, token.OwnershipHistory, chainToken.chain)
			if err != nil {
				return nil, fmt.Errorf("failed to get ownership history for token: %s", err)
			}

			t := persist.TokenGallery{
				Media:                token.Media,
				TokenType:            token.TokenType,
				Chain:                chainToken.chain,
				Name:                 persist.NullString(token.Name),
				Description:          persist.NullString(token.Description),
				TokenURI:             token.TokenURI,
				TokenID:              token.TokenID,
				Quantity:             seenQuantities[ti],
				OwnerUserID:          ownerUser.ID,
				OwnedByWallets:       seenWallets[ti],
				OwnershipHistory:     ownership,
				TokenMetadata:        token.TokenMetadata,
				Contract:             contractAddressIDs[chainToken.chain.NormalizeAddress(token.ContractAddress)],
				ExternalURL:          persist.NullString(token.ExternalURL),
				BlockNumber:          token.BlockNumber,
				IsProviderMarkedSpam: token.IsSpam,
			}
			seenTokens[ti] = t
		}
	}

	res := make([]persist.TokenGallery, 0, len(seenTokens))
	for _, t := range seenTokens {
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
	return e.err.Error()
}
