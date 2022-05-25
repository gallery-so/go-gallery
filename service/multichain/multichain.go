package multichain

import (
	"context"
	"fmt"
	"strings"

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
	CollectionName   string                        `json:"collection_name"`

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

// ErrChainNotFound is an error that occurs when a chain provider for a given chain is not registered in the MultichainProvider
type ErrChainNotFound struct {
	Chain persist.Chain
}

// ChainProvider is an interface for retrieving data from a chain
type ChainProvider interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
	GetTokensByWalletAddress(context.Context, persist.Address) ([]ChainAgnosticToken, error)
	GetTokensByContractAddress(context.Context, persist.Address) ([]ChainAgnosticToken, error)
	GetTokensByTokenIdentifiers(context.Context, persist.TokenIdentifiers) ([]ChainAgnosticToken, error)
	GetContractByAddress(context.Context, persist.Address) (ChainAgnosticContract, error)
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
				provider, ok := d.Chains[chain]
				if !ok {
					errChan <- ErrChainNotFound{Chain: chain}
					return
				}
				tokens, err := provider.GetTokensByWalletAddress(ctx, addr)
				if err != nil {
					errChan <- err
					return
				}

				newTokens, err := tokensToTokens(ctx, tokens, chain, user, addresses)
				if err != nil {
					errChan <- err
					return
				}
				errChan <- d.TokenRepo.BulkUpsert(ctx, newTokens)
			}(addr, chain)
		}
	}
	for i := 0; i < len(user.Wallets); i++ {
		err := <-errChan
		if err != nil {
			return err
		}
		logrus.Infof("updated tokens for wallet %s", user.Wallets[i].Address)
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

func tokensToTokens(ctx context.Context, tokens []ChainAgnosticToken, chain persist.Chain, ownerUser persist.User, ownerAddresses []persist.Address) ([]persist.TokenGallery, error) {
	res := make([]persist.TokenGallery, len(tokens))
	seen := make(map[persist.TokenIdentifiers][]persist.Wallet)
	addressToWallets := make(map[string]persist.Wallet)
	for _, wallet := range ownerUser.Wallets {
		for _, addr := range ownerAddresses {
			if wallet.Address == addr {
				addressToWallets[strings.ToLower(addr.String())] = wallet
				break
			}
		}
	}
	for i, token := range tokens {
		ownership, err := addressAtBlockToAddressAtBlock(ctx, token.OwnershipHistory, chain)
		if err != nil {
			return nil, fmt.Errorf("failed to get ownership history for token: %s", err)
		}

		ti := persist.NewTokenIdentifiers(token.ContractAddress, token.TokenID, chain)

		if w, ok := addressToWallets[strings.ToLower(token.OwnerAddress.String())]; ok {
			if it, ok := seen[ti]; ok {
				it = append(it, w)
				seen[ti] = it

			} else {
				seen[ti] = []persist.Wallet{w}
			}
		}

		res[i] = persist.TokenGallery{
			Media:            token.Media,
			TokenType:        token.TokenType,
			Chain:            chain,
			Name:             persist.NullString(token.Name),
			Description:      persist.NullString(token.Description),
			TokenURI:         token.TokenURI,
			TokenID:          token.TokenID,
			Quantity:         token.Quantity,
			OwnerUserID:      ownerUser.ID,
			OwnerAddresses:   seen[ti],
			OwnershipHistory: ownership,
			TokenMetadata:    token.TokenMetadata,
			ContractAddress:  token.ContractAddress,
			ExternalURL:      persist.NullString(token.ExternalURL),
			BlockNumber:      token.BlockNumber,
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
