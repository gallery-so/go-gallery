package multichain

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// Provider is an interface for retrieving data from multiple chains
type Provider struct {
	TokenRepo    persist.TokenGalleryRepository
	ContractRepo persist.ContractRepository
	UserRepo     persist.UserRepository
	Chains       []ChainProvider
}

// BlockchainInfo retrieves blockchain info from all chains
type BlockchainInfo struct {
	ChainName persist.Chain `json:"chain_name"`
	ChainID   int           `json:"chain_id"`
}

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

	ExternalURL string `json:"external_url"`

	BlockNumber persist.BlockNumber `json:"block_number"`
}

type ChainAgnosticAddressAtBlock struct {
	Address persist.AddressValue `json:"address"`
	Block   persist.BlockNumber  `json:"block"`
}

type ChainAgnosticContract struct {
	Address        persist.AddressValue `json:"address"`
	Symbol         string               `json:"symbol"`
	Name           string               `json:"name"`
	CreatorAddress persist.AddressValue `json:"creator_address"`

	LatestBlock persist.BlockNumber `json:"latest_block"`
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
func NewMultiChainDataRetriever(chains ...ChainProvider) *Provider {
	return &Provider{
		Chains: chains,
	}
}

func (d *Provider) UpdateTokensForUser(ctx context.Context, userID persist.DBID) error {
	// user, err := d.UserRepo.GetByID(ctx, userID)
	// if err != nil {
	// 	return err
	// }

	// errChan := make(chan error)

	// for _, addr := range user.Addresses {

	// }
	return nil
}

// VerifySignature verifies a signature for a wallet address
func (d *Provider) VerifySignature(ctx context.Context, pSig string, pNonce string, pAddress persist.AddressValue, pChain persist.Chain, pWalletType persist.WalletType) (bool, error) {
	// user, err := d.UserRepo.GetByID(ctx, userID)
	// if err != nil {
	// 	return err
	// }

	// errChan := make(chan error)

	// for _, addr := range user.Addresses {

	// }
	return true, nil
}
