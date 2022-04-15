package multichain

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

// DataRetriever is an interface for retrieving data from multiple chains
type DataRetriever struct {
	TokenRepo    persist.TokenGalleryRepository
	ContractRepo persist.ContractRepository
	UserRepo     persist.UserRepository
	Chains       []ChainDataRetriever
}

// BlockchainInfo retrieves blockchain info from all chains
type BlockchainInfo struct {
	ChainName persist.Chain `json:"chain_name"`
	ChainID   int           `json:"chain_id"`
}

// ChainDataRetriever is an interface for retrieving data from a chain
type ChainDataRetriever interface {
	GetBlockchainInfo(context.Context) (BlockchainInfo, error)
	GetTokensByWalletAddress(context.Context, persist.Wallet) ([]persist.TokenGallery, error)
	GetTokensByContractAddress(context.Context, persist.Wallet) ([]persist.TokenGallery, error)
	GetTokensByTokenIdentifiers(context.Context, persist.TokenIdentifiers) ([]persist.TokenGallery, error)
	GetContractByAddress(context.Context, persist.Wallet) (persist.ContractGallery, error)
	// bool is whether or not to update all media content, including the tokens that already have media content
	UpdateMediaForWallet(context.Context, persist.Wallet, bool) error
	// do we want to return the tokens we validate?
	// bool is whether or not to update all of the tokens regardless of whether we know they exist already
	ValidateTokensForWallet(context.Context, persist.Wallet, bool) error
	VerifySignature(context.Context, persist.Wallet, string, int) (bool, error)
}

// NewMultiChainDataRetriever creates a new MultiChainDataRetriever
func NewMultiChainDataRetriever(chains ...ChainDataRetriever) *DataRetriever {
	return &DataRetriever{
		Chains: chains,
	}
}

func (d *DataRetriever) UpdateTokensForUser(ctx context.Context, userID persist.DBID) error {
	// user, err := d.UserRepo.GetByID(ctx, userID)
	// if err != nil {
	// 	return err
	// }

	// errChan := make(chan error)

	// for _, addr := range user.Addresses {

	// }
	return nil
}
