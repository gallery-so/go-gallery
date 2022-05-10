package multichain

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	TokenRepo    persist.TokenGalleryRepository
	ContractRepo persist.ContractGalleryRepository
	UserRepo     persist.UserRepository
	AddressRepo  persist.AddressRepository
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
	OwnerAddress     persist.AddressValue          `json:"owner_address"`
	OwnershipHistory []ChainAgnosticAddressAtBlock `json:"previous_owners"`
	TokenMetadata    persist.TokenMetadata         `json:"metadata"`
	ContractAddress  persist.AddressValue          `json:"contract_address"`
	CollectionName   string                        `json:"collection_name"`

	ExternalURL string `json:"external_url"`

	BlockNumber persist.BlockNumber `json:"block_number"`
}

// ChainAgnosticAddressAtBlock is an address at a block
type ChainAgnosticAddressAtBlock struct {
	Address persist.AddressValue `json:"address"`
	Block   persist.BlockNumber  `json:"block"`
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Address        persist.AddressValue `json:"address"`
	Symbol         string               `json:"symbol"`
	Name           string               `json:"name"`
	CreatorAddress persist.AddressValue `json:"creator_address"`

	LatestBlock persist.BlockNumber `json:"latest_block"`
}

// ErrChainNotFound is an error that occurs when a chain provider for a given chain is not registered in the MultichainProvider
type ErrChainNotFound struct {
	Chain persist.Chain
}

// ChainProvider is an interface for retrieving data from a chain
type ChainProvider interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
	GetTokensByWalletAddress(context.Context, persist.AddressValue) ([]ChainAgnosticToken, error)
	GetTokensByContractAddress(context.Context, persist.AddressValue) ([]ChainAgnosticToken, error)
	GetTokensByTokenIdentifiers(context.Context, persist.TokenIdentifiers) ([]ChainAgnosticToken, error)
	GetContractByAddress(context.Context, persist.AddressValue) (ChainAgnosticContract, error)
	// bool is whether or not to update all media content, including the tokens that already have media content
	UpdateMediaForWallet(context.Context, persist.AddressValue, bool) error
	// do we want to return the tokens we validate?
	// bool is whether or not to update all of the tokens regardless of whether we know they exist already
	ValidateTokensForWallet(context.Context, persist.AddressValue, bool) error
	// ctx, address, chain, wallet type, nonce, sig
	VerifySignature(context.Context, persist.AddressValue, persist.WalletType, string, string) (bool, error)
}

// NewMultiChainDataRetriever creates a new MultiChainDataRetriever
func NewMultiChainDataRetriever(ctx context.Context, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractGalleryRepository, userRepo persist.UserRepository, addressRepo persist.AddressRepository, chains ...ChainProvider) *Provider {
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
		AddressRepo:  addressRepo,
		Chains:       c,
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
	addresses := make([]persist.Address, len(user.Wallets))
	for i, wallet := range user.Wallets {
		addresses[i] = wallet.Address
	}
	for _, addr := range addresses {
		logrus.Infof("updating media for user %s wallet %s", user.Username, addr.AddressValue)
		go func(addr persist.Address) {
			provider, ok := d.Chains[addr.Chain]
			if !ok {
				errChan <- ErrChainNotFound{Chain: addr.Chain}
				return
			}
			tokens, err := provider.GetTokensByWalletAddress(ctx, addr.AddressValue)
			if err != nil {
				errChan <- err
				return
			}

			newTokens, err := tokensToTokens(ctx, tokens, addr.Chain, userID, addresses, d.AddressRepo)
			if err != nil {
				errChan <- err
				return
			}
			errChan <- d.TokenRepo.BulkUpsert(ctx, newTokens)
		}(addr)
	}
	for i := 0; i < len(addresses); i++ {
		err := <-errChan
		if err != nil {
			return err
		}
		logrus.Infof("updated tokens for wallet %s", user.Wallets[i].Address.AddressValue)
	}
	return nil
}

// VerifySignature verifies a signature for a wallet address
func (d *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pAddress persist.AddressValue, pChain persist.Chain, pWalletType persist.WalletType) (bool, error) {
	provider, ok := d.Chains[pChain]
	if !ok {
		return false, ErrChainNotFound{Chain: pChain}
	}
	return provider.VerifySignature(ctx, pAddress, pWalletType, pNonce, pSig)
}

func tokensToTokens(ctx context.Context, tokens []ChainAgnosticToken, chain persist.Chain, ownerUserID persist.DBID, ownerAddresses []persist.Address, addressRepo persist.AddressRepository) ([]persist.TokenGallery, error) {
	res := make([]persist.TokenGallery, len(tokens))
	for i, token := range tokens {
		ownership, err := addressAtBlockToAddressAtBlock(ctx, token.OwnershipHistory, chain, addressRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to get ownership history for token: %s", err)
		}
		contractAddress, err := addressRepo.GetByDetails(ctx, token.ContractAddress, chain)
		if err != nil {
			if _, ok := err.(persist.ErrAddressNotFoundByDetails); !ok {
				return nil, fmt.Errorf("failed to get contract address for token: %s", err)
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
			OwnerUserID:      ownerUserID,
			OwnerAddresses:   ownerAddresses,
			OwnershipHistory: ownership,
			TokenMetadata:    token.TokenMetadata,
			ContractAddress:  contractAddress,
			ExternalURL:      persist.NullString(token.ExternalURL),
			BlockNumber:      token.BlockNumber,
		}
	}
	return res, nil

}

func addressAtBlockToAddressAtBlock(ctx context.Context, addresses []ChainAgnosticAddressAtBlock, chain persist.Chain, addressRepo persist.AddressRepository) ([]persist.AddressAtBlock, error) {
	res := make([]persist.AddressAtBlock, len(addresses))
	for i, addr := range addresses {
		dbAddr, err := addressRepo.GetByDetails(ctx, addr.Address, chain)
		if err != nil {
			if _, ok := err.(persist.ErrAddressNotFoundByDetails); !ok {
				return nil, err
			}
		}
		res[i] = persist.AddressAtBlock{
			Address: dbAddr,
			Block:   addr.Block,
		}
	}
	return res, nil
}

func (e ErrChainNotFound) Error() string {
	return fmt.Sprintf("chain provider not found for chain: %d", e.Chain)
}
