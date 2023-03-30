package multichain

import (
	"context"
	"sync"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// EvalFallbackProvider will call its fallback if the primary Provider's token
// response is unsuitable based on Eval
type EvalFallbackProvider struct {
	Primary interface {
		configurer
		tokensOwnerFetcher
		tokensContractFetcher
	}
	Fallback tokensOwnerFetcher
	Eval     func(context.Context, ChainAgnosticToken) bool
}

type FailureFallbackProvider struct {
	Primary  tokensOwnerFetcher
	Fallback tokensOwnerFetcher
}

func (f EvalFallbackProvider) GetBlockchainInfo(ctx context.Context) (BlockchainInfo, error) {
	return f.Primary.GetBlockchainInfo(ctx)
}

func (f EvalFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address, limit, offset)
	if err != nil {
		return nil, nil, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contracts, nil
}

func (f EvalFallbackProvider) GetTokensByContractAddress(ctx context.Context, contract persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	tokens, agnosticContract, err := f.Primary.GetTokensByContractAddress(ctx, contract, limit, offset)
	if err != nil {
		return nil, ChainAgnosticContract{}, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, agnosticContract, nil
}

func (f EvalFallbackProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, id ChainAgnosticIdentifiers, address persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error) {
	token, contract, err := f.Primary.GetTokensByTokenIdentifiersAndOwner(ctx, id, address)
	if err != nil {
		return ChainAgnosticToken{}, ChainAgnosticContract{}, err
	}
	if !f.Eval(ctx, token) {
		token.TokenMetadata = f.callFallback(ctx, token).TokenMetadata
	}
	return token, contract, nil
}

func (f EvalFallbackProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner persist.Address, contractAddress persist.Address, limit int, offset int) ([]ChainAgnosticToken, ChainAgnosticContract, error) {
	tokens, contract, err := f.Primary.GetTokensByContractAddressAndOwner(ctx, owner, contractAddress, limit, offset)
	if err != nil {
		return nil, ChainAgnosticContract{}, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contract, err
}

func (f EvalFallbackProvider) resolveTokens(ctx context.Context, tokens []ChainAgnosticToken) []ChainAgnosticToken {
	usableTokens := make([]ChainAgnosticToken, len(tokens))
	var wg sync.WaitGroup

	for i, token := range tokens {
		wg.Add(1)
		go func(i int, token ChainAgnosticToken) {
			defer wg.Done()
			usableTokens[i] = token
			if !f.Eval(ctx, token) {
				usableTokens[i].TokenMetadata = f.callFallback(ctx, token).TokenMetadata
			}
		}(i, token)
	}

	wg.Wait()

	return usableTokens
}

func (f *EvalFallbackProvider) callFallback(ctx context.Context, primary ChainAgnosticToken) ChainAgnosticToken {
	id := ChainAgnosticIdentifiers{primary.ContractAddress, primary.TokenID}
	backup, _, err := f.Fallback.GetTokensByTokenIdentifiersAndOwner(ctx, id, primary.OwnerAddress)
	if err == nil && f.Eval(ctx, backup) {
		return backup
	}
	logger.For(ctx).WithError(err).Warn("failed to call fallback")
	return primary
}

func (f *EvalFallbackProvider) GetSubproviders() []any {
	return []any{f.Primary, f.Fallback}
}

func (f FailureFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address, limit, offset)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get tokens by wallet address from primary in failure fallback")
		return f.Fallback.GetTokensByWalletAddress(ctx, address, limit, offset)
	}

	return tokens, contracts, nil
}

func (f FailureFallbackProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, id ChainAgnosticIdentifiers, address persist.Address) (ChainAgnosticToken, ChainAgnosticContract, error) {
	token, contract, err := f.Primary.GetTokensByTokenIdentifiersAndOwner(ctx, id, address)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get token by token identifiers and owner from primary in failure fallback")
		return f.Fallback.GetTokensByTokenIdentifiersAndOwner(ctx, id, address)
	}
	return token, contract, nil
}

func (f *FailureFallbackProvider) GetSubproviders() []any {
	return []any{f.Primary, f.Fallback}
}
