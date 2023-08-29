package multichain

import (
	"context"
	"sync"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// SyncWithContractEvalFallbackProvider will call its fallback if the primary Provider's token
// response is unsuitable based on Eval
type SyncWithContractEvalFallbackProvider struct {
	Primary  SyncWithContractEvalPrimary
	Fallback SyncWithContractEvalSecondary
	Eval     func(context.Context, ChainAgnosticToken) bool
}

type SyncWithContractEvalPrimary interface {
	Configurer
	TokensOwnerFetcher
	TokensContractFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
}

type SyncWithContractEvalSecondary interface {
	TokensOwnerFetcher
}

// SyncFailureFallbackProvider will call its fallback if the primary Provider's token
// response fails (returns an error)
type SyncFailureFallbackProvider struct {
	Primary  SyncFailurePrimary
	Fallback SyncFailureSecondary
}

type SyncFailurePrimary interface {
	Configurer
	TokensOwnerFetcher
	TokensIncrementalOwnerFetcher
	TokenDescriptorsFetcher
	TokensContractFetcher
	ContractsFetcher
}

type SyncFailureSecondary interface {
	TokensOwnerFetcher
	TokenDescriptorsFetcher
}

func (f SyncWithContractEvalFallbackProvider) GetBlockchainInfo() BlockchainInfo {
	return f.Primary.GetBlockchainInfo()
}

func (f SyncWithContractEvalFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contracts, nil
}

func (f SyncWithContractEvalFallbackProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	tokens, agnosticContract, err := f.Primary.GetTokensByContractAddress(ctx, contract, limit, offset)
	if err != nil {
		return nil, ChainAgnosticContract{}, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, agnosticContract, nil
}

func (f SyncWithContractEvalFallbackProvider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, id ChainAgnosticIdentifiers, address persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error) {
	token, contract, err := f.Primary.GetTokenByTokenIdentifiersAndOwner(ctx, id, address)
	if err != nil {
		return ChainAgnosticToken{}, ChainAgnosticContract{}, err
	}
	if !f.Eval(ctx, token) {
		token.TokenMetadata = f.callFallbackIdentifiers(ctx, token).TokenMetadata
	}
	return token, contract, nil
}

func (f SyncWithContractEvalFallbackProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contractAddress persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	tokens, contract, err := f.Primary.GetTokensByContractAddressAndOwner(ctx, owner, contractAddress, limit, offset)
	if err != nil {
		return nil, ChainAgnosticContract{}, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contract, err
}

func (f SyncWithContractEvalFallbackProvider) resolveTokens(ctx context.Context, tokens []ChainAgnosticToken) []ChainAgnosticToken {
	usableTokens := make([]ChainAgnosticToken, len(tokens))
	var wg sync.WaitGroup

	for i, token := range tokens {
		wg.Add(1)
		go func(i int, token ChainAgnosticToken) {
			defer wg.Done()
			usableTokens[i] = token
			if !f.Eval(ctx, token) {
				usableTokens[i].TokenMetadata = f.callFallbackIdentifiers(ctx, token).TokenMetadata
			}
		}(i, token)
	}

	wg.Wait()

	return usableTokens
}

func (f *SyncWithContractEvalFallbackProvider) callFallbackIdentifiers(ctx context.Context, primary ChainAgnosticToken) ChainAgnosticToken {
	id := ChainAgnosticIdentifiers{primary.ContractAddress, primary.TokenID}
	backup, _, err := f.Fallback.GetTokenByTokenIdentifiersAndOwner(ctx, id, primary.OwnerAddress)
	if err == nil && f.Eval(ctx, backup) {
		return backup
	}
	logger.For(ctx).WithError(err).Warn("failed to call fallback")
	return primary
}

func (f SyncWithContractEvalFallbackProvider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, id ChainAgnosticIdentifiers) (ChainAgnosticTokenDescriptors, ChainAgnosticContractDescriptors, error) {
	return f.Primary.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
}

func (f SyncWithContractEvalFallbackProvider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, id ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	return f.Primary.GetTokenMetadataByTokenIdentifiers(ctx, id)
}

func (f SyncWithContractEvalFallbackProvider) GetSubproviders() []any {
	return []any{f.Primary, f.Fallback}
}

func (f SyncFailureFallbackProvider) GetBlockchainInfo() BlockchainInfo {
	return f.Primary.GetBlockchainInfo()
}

func (f SyncFailureFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address, limit, offset)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get tokens by wallet address from primary in failure fallback")
		return f.Fallback.GetTokensByWalletAddress(ctx, address, limit, offset)
	}

	return tokens, contracts, nil
}

func (f SyncFailureFallbackProvider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address, rec chan<- ChainAgnosticTokensAndContracts, errChan chan<- error) {

	if tiof, ok := f.Fallback.(TokensIncrementalOwnerFetcher); ok {
		subErrChan := make(chan error)
		subRec := make(chan ChainAgnosticTokensAndContracts)
		f.Primary.GetTokensIncrementallyByWalletAddress(ctx, address, subRec, subErrChan)
		for {
			select {
			case <-subErrChan:
				tiof.GetTokensIncrementallyByWalletAddress(ctx, address, rec, errChan)
				return
			case tokens, ok := <-subRec:
				if !ok {
					close(rec)
					return
				}
				rec <- tokens
			}
		}
	} else {
		f.Primary.GetTokensIncrementallyByWalletAddress(ctx, address, rec, errChan)
	}
}

func (f SyncFailureFallbackProvider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, id ChainAgnosticIdentifiers, address persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error) {
	token, contract, err := f.Primary.GetTokenByTokenIdentifiersAndOwner(ctx, id, address)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
		return f.Fallback.GetTokenByTokenIdentifiersAndOwner(ctx, id, address)
	}
	return token, contract, nil
}

func (f SyncFailureFallbackProvider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, id ChainAgnosticIdentifiers) (ChainAgnosticTokenDescriptors, ChainAgnosticContractDescriptors, error) {
	token, contract, err := f.Primary.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
		return f.Fallback.GetTokenDescriptorsByTokenIdentifiers(ctx, id)
	}
	return token, contract, nil
}

func (f SyncFailureFallbackProvider) GetContractByAddress(ctx context.Context, addr persist.Address) (ChainAgnosticContract, error) {
	contract, err := f.Primary.GetContractByAddress(ctx, addr)
	if err != nil {
		if tcf, ok := f.Fallback.(ContractsFetcher); ok {
			logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
			return tcf.GetContractByAddress(ctx, addr)
		}
		return ChainAgnosticContract{}, err
	}
	return contract, nil
}

func (f SyncFailureFallbackProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	ts, c, err := f.Primary.GetTokensByContractAddress(ctx, contract, limit, offset)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
		if tcf, ok := f.Fallback.(TokensContractFetcher); ok {
			return tcf.GetTokensByContractAddress(ctx, contract, limit, offset)
		}
		return nil, ChainAgnosticContract{}, err
	}
	return ts, c, nil
}
func (f SyncFailureFallbackProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	ts, c, err := f.Primary.GetTokensByContractAddressAndOwner(ctx, owner, contract, limit, offset)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
		if tcf, ok := f.Fallback.(TokensContractFetcher); ok {
			return tcf.GetTokensByContractAddressAndOwner(ctx, owner, contract, limit, offset)
		}
		return nil, ChainAgnosticContract{}, err
	}
	return ts, c, nil
}

func (f SyncFailureFallbackProvider) GetSubproviders() []any {
	return []any{f.Primary, f.Fallback}
}
