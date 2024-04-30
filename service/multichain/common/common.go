package common

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

// ChainAgnosticIdentifiers identify tokens despite their chain
type ChainAgnosticIdentifiers struct {
	ContractAddress persist.Address    `json:"contract_address"`
	TokenID         persist.HexTokenID `json:"token_id"`
}

func (t ChainAgnosticIdentifiers) String() string {
	return fmt.Sprintf("token(address=%s, tokenID=%s)", t.ContractAddress, t.TokenID.ToDecimalTokenID())
}

// Verifier can verify that a signature is signed by a given key
type Verifier interface {
	VerifySignature(ctx context.Context, pubKey persist.PubKey, walletType persist.WalletType, nonce string, sig string) (bool, error)
}

type TokenIdentifierOwnerFetcher interface {
	GetTokenByTokenIdentifiersAndOwner(context.Context, ChainAgnosticIdentifiers, persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error)
}

type TokensByContractWalletFetcher interface {
	GetTokensByContractWallet(ctx context.Context, contract persist.ChainAddress, wallet persist.Address) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

type TokensByTokenIdentifiersFetcher interface {
	GetTokensByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) ([]ChainAgnosticToken, ChainAgnosticContract, error)
}

// TokensIncrementalOwnerFetcher supports fetching tokens for syncing incrementally
type TokensIncrementalOwnerFetcher interface {
	// NOTE: implementation MUST close the rec channel
	GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan ChainAgnosticTokensAndContracts, <-chan error)
}

// TokensIncrementalContractFetcher supports fetching tokens by contract for syncing incrementally
type TokensIncrementalContractFetcher interface {
	// NOTE: implementations MUST close the rec channel
	// maxLimit is not for pagination, it is to make sure we don't fetch a bajilion tokens from an omnibus contract
	GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan ChainAgnosticTokensAndContracts, <-chan error)
}

type ContractFetcher interface {
	GetContractByAddress(ctx context.Context, contract persist.Address) (ChainAgnosticContract, error)
}

type ContractsCreatorFetcher interface {
	GetContractsByCreatorAddress(ctx context.Context, owner persist.Address) ([]ChainAgnosticContract, error)
}

// TokenMetadataFetcher supports fetching token metadata
type TokenMetadataFetcher interface {
	GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (persist.TokenMetadata, error)
}

type TokenMetadataBatcher interface {
	GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error)
}

type TokenDescriptorsFetcher interface {
	GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti ChainAgnosticIdentifiers) (ChainAgnosticTokenDescriptors, ChainAgnosticContractDescriptors, error)
}

// ChainAgnosticToken is a token that is agnostic to the chain it is on
type ChainAgnosticToken struct {
	Descriptors     ChainAgnosticTokenDescriptors `json:"descriptors"`
	TokenType       persist.TokenType             `json:"token_type"`
	TokenURI        persist.TokenURI              `json:"token_uri"`
	TokenID         persist.HexTokenID            `json:"token_id"`
	Quantity        persist.HexString             `json:"quantity"`
	OwnerAddress    persist.Address               `json:"owner_address"`
	TokenMetadata   persist.TokenMetadata         `json:"metadata"`
	ContractAddress persist.Address               `json:"contract_address"`
	FallbackMedia   persist.FallbackMedia         `json:"fallback_media"`
	ExternalURL     string                        `json:"external_url"`
	BlockNumber     persist.BlockNumber           `json:"block_number"`
	IsSpam          *bool                         `json:"is_spam"`
}

// ChainAgnosticContract is a contract that is agnostic to the chain it is on
type ChainAgnosticContract struct {
	Descriptors ChainAgnosticContractDescriptors `json:"descriptors"`
	Address     persist.Address                  `json:"address"`
	IsSpam      *bool                            `json:"is_spam"`
	LatestBlock persist.BlockNumber              `json:"latest_block"`
}

type ChainAgnosticTokensAndContracts struct {
	Tokens    []ChainAgnosticToken    `json:"tokens"`
	Contracts []ChainAgnosticContract `json:"contracts"`
}

// ChainAgnosticTokenDescriptors are the fields that describe a token but cannot be used to uniquely identify it
type ChainAgnosticTokenDescriptors struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ChainAgnosticContractDescriptors struct {
	Symbol          string          `json:"symbol"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ProfileImageURL string          `json:"profile_image_url"`
	OwnerAddress    persist.Address `json:"owner_address"`
}
