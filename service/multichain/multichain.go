package multichain

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	TokenRepo    persist.TokenGalleryRepository
	ContractRepo persist.ContractGalleryRepository
	UserRepo     persist.UserRepository
	Chains       map[persist.Chain]ChainProvider
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
	chain  persist.Chain
	tokens []ChainAgnosticToken
}

type chainContracts struct {
	chain     persist.Chain
	contracts []ChainAgnosticContract
}

type tokenIdentifiers struct {
	chain    persist.Chain
	tokenID  persist.TokenID
	contract persist.DBID
}

// ChainProvider is an interface for retrieving data from a chain
type ChainProvider interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
	GetTokensByWalletAddress(context.Context, persist.Address) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetTokensByContractAddress(context.Context, persist.Address) ([]ChainAgnosticToken, ChainAgnosticContract, error)
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) ([]ChainAgnosticToken, []ChainAgnosticContract, error)
	GetContractByAddress(context.Context, persist.Address) (ChainAgnosticContract, error)
	RefreshToken(context.Context, ChainAgnosticIdentifiers) error
	RefreshContract(context.Context, persist.Address) error
	// bool is whether or not to update all media content, including the tokens that already have media content
	UpdateMediaForWallet(context.Context, persist.Address, bool) error
	// do we want to return the tokens we validate?
	// bool is whether or not to update all of the tokens regardless of whether we know they exist already
	ValidateTokensForWallet(context.Context, persist.Address, bool) error
	// ctx, address, chain, wallet type, nonce, sig
	VerifySignature(context.Context, persist.Address, persist.WalletType, string, string) (bool, error)
}

// NewMultiChainDataRetriever creates a new MultiChainDataRetriever
func NewMultiChainDataRetriever(ctx context.Context, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, userRepo persist.UserRepository, chains ...ChainProvider) *Provider {
	c := map[persist.Chain]ChainProvider{}
	for _, chain := range chains {
		info, err := chain.GetBlockchainInfo(ctx)
		if err != nil {
			panic(err)
		}
		if _, ok := c[info.Chain]; ok {
			panic("chain provider already exists for chain " + fmt.Sprint(info.Chain))
		}
		c[info.Chain] = chain
	}
	return &Provider{
		TokenRepo:    tokenRepo,
		ContractRepo: contractRepo,
		UserRepo:     userRepo,

		Chains: c,
	}
}

// UpdateTokensForUser updates the media for all tokens for a user
// TODO consider updating contracts as well
func (d *Provider) UpdateTokensForUser(ctx context.Context, userID persist.DBID) error {
	user, err := d.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	errChan := make(chan error)
	incomingTokens := make(chan chainTokens)
	incomingContracts := make(chan chainContracts)
	chainsToAddresses := make(map[persist.Chain][]persist.Address)
	for _, wallet := range user.Wallets {
		if it, ok := chainsToAddresses[wallet.Chain]; ok {
			it = append(it, wallet.Address)
			chainsToAddresses[wallet.Chain] = it
		} else {
			chainsToAddresses[wallet.Chain] = []persist.Address{wallet.Address}
		}
	}
	for c, a := range chainsToAddresses {
		logrus.Infof("updating media for user %s wallets %s", user.Username, a)
		chain := c
		addresses := a
		for _, addr := range addresses {
			go func(addr persist.Address, chain persist.Chain) {
				start := time.Now()
				provider, ok := d.Chains[chain]
				if !ok {
					errChan <- ErrChainNotFound{Chain: chain}
					return
				}
				tokens, contracts, err := provider.GetTokensByWalletAddress(ctx, addr)
				if err != nil {
					errChan <- err
					return
				}

				incomingTokens <- chainTokens{chain: chain, tokens: tokens}
				incomingContracts <- chainContracts{chain: chain, contracts: contracts}
				logrus.Debugf("updated media for user %s wallet %s in %s: tokens %d", user.Username, addr, time.Since(start), len(tokens))
			}(addr, chain)
		}
	}
	allTokens := make([]chainTokens, 0, len(user.Wallets))
	allContracts := make([]chainContracts, 0, len(user.Wallets))
	// ensure all tokens have been upserted
	for i := 0; i < (len(user.Wallets) * 2); i++ {
		select {
		case incomingTokens := <-incomingTokens:
			allTokens = append(allTokens, incomingTokens)
		case incomingContracts := <-incomingContracts:
			allContracts = append(allContracts, incomingContracts)
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			return err
		}
	}
	newContracts, err := contractsToContracts(ctx, allContracts)
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

	newTokens, err := tokensToTokens(ctx, allTokens, addressesToContracts, user)
	if err := d.TokenRepo.BulkUpsert(ctx, newTokens); err != nil {
		return fmt.Errorf("error upserting tokens: %s", err)
	}

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
		if !ownedTokens[tokenIdentifiers{chain: nft.Chain, tokenID: nft.TokenID, contract: nft.Contract}] {
			err := d.TokenRepo.DeleteByID(ctx, nft.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// VerifySignature verifies a signature for a wallet address
func (d *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pChainAddress persist.ChainAddress, pWalletType persist.WalletType) (bool, error) {
	provider, ok := d.Chains[pChainAddress.Chain()]
	if !ok {
		return false, ErrChainNotFound{Chain: pChainAddress.Chain()}
	}
	return provider.VerifySignature(ctx, pChainAddress.Address(), pWalletType, pNonce, pSig)
}

// RefreshToken refreshes a token on the given chain using the chain provider for that chain
func (d *Provider) RefreshToken(ctx context.Context, ti persist.TokenIdentifiers) error {
	provider, ok := d.Chains[ti.Chain]
	if !ok {
		return ErrChainNotFound{Chain: ti.Chain}
	}
	return provider.RefreshToken(ctx, ChainAgnosticIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID})
}

// RefreshContract refreshes a contract on the given chain using the chain provider for that chain
func (d *Provider) RefreshContract(ctx context.Context, ci persist.ContractIdentifiers) error {
	provider, ok := d.Chains[ci.Chain]
	if !ok {
		return ErrChainNotFound{Chain: ci.Chain}
	}
	return provider.RefreshContract(ctx, ci.ContractAddress)
}

func tokensToTokens(ctx context.Context, chaintokens []chainTokens, contractAddressIDs map[string]persist.DBID, ownerUser persist.User) ([]persist.TokenGallery, error) {
	res := make([]persist.TokenGallery, 0, len(chaintokens))
	seenWallets := make(map[persist.TokenIdentifiers][]persist.Wallet)
	seenQuantities := make(map[persist.TokenIdentifiers]persist.HexString)
	addressToWallets := make(map[string]persist.Wallet)
	for _, wallet := range ownerUser.Wallets {
		// could a normalized address ever overlap with the normalized address of another chain?
		normalizedAddress := wallet.Chain.NormalizeAddress(wallet.Address)
		addressToWallets[normalizedAddress] = wallet
	}
	for _, chainToken := range chaintokens {
		for _, token := range chainToken.tokens {
			ownership, err := addressAtBlockToAddressAtBlock(ctx, token.OwnershipHistory, chainToken.chain)
			if err != nil {
				return nil, fmt.Errorf("failed to get ownership history for token: %s", err)
			}

			ti := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chainToken.chain)

			if w, ok := addressToWallets[chainToken.chain.NormalizeAddress(token.OwnerAddress)]; ok {
				seenWallets[ti] = append(seenWallets[ti], w)
			}

			if q, ok := seenQuantities[ti]; ok {
				seenQuantities[ti] = q.Add(token.Quantity)
			} else {
				seenQuantities[ti] = token.Quantity
			}

			res = append(res, persist.TokenGallery{
				Media:            token.Media,
				TokenType:        token.TokenType,
				Chain:            chainToken.chain,
				Name:             persist.NullString(token.Name),
				Description:      persist.NullString(token.Description),
				TokenURI:         token.TokenURI,
				TokenID:          token.TokenID,
				Quantity:         seenQuantities[ti],
				OwnerUserID:      ownerUser.ID,
				OwnedByWallets:   seenWallets[ti],
				OwnershipHistory: ownership,
				TokenMetadata:    token.TokenMetadata,
				Contract:         contractAddressIDs[chainToken.chain.NormalizeAddress(token.ContractAddress)],
				ExternalURL:      persist.NullString(token.ExternalURL),
				BlockNumber:      token.BlockNumber,
			})
		}
	}
	return res, nil

}

func contractsToContracts(ctx context.Context, contracts []chainContracts) ([]persist.ContractGallery, error) {
	res := make([]persist.ContractGallery, 0, len(contracts))
	seen := make(map[persist.ChainAddress]bool)
	for _, chainContract := range contracts {
		for _, contract := range chainContract.contracts {
			if _, ok := seen[persist.NewChainAddress(contract.Address, chainContract.chain)]; !ok {
				res = append(res, persist.ContractGallery{
					Chain:          chainContract.chain,
					Address:        contract.Address,
					Symbol:         persist.NullString(contract.Symbol),
					Name:           persist.NullString(contract.Name),
					CreatorAddress: contract.CreatorAddress,
				})
				seen[persist.NewChainAddress(contract.Address, chainContract.chain)] = true
			}
		}

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

func (e ErrChainNotFound) Error() string {
	return fmt.Sprintf("chain provider not found for chain: %d", e.Chain)
}
