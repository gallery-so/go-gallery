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
	Eval     func(ChainAgnosticToken) bool
}

type SyncWithContractEvalPrimary interface {
	Configurer
	TokensOwnerFetcher
	TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetcher
	TokensContractFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	ContractsOwnerFetcher
}

type SyncWithContractEvalSecondary interface {
	TokensOwnerFetcher
	TokensIncrementalOwnerFetcher
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
	TokensIncrementalContractFetcher
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

func (f SyncWithContractEvalFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address)
	if err != nil {
		return nil, nil, err
	}
	tokens = f.resolveTokens(ctx, tokens)
	return tokens, contracts, nil
}

func (f SyncWithContractEvalFallbackProvider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan ChainAgnosticTokensAndContracts, <-chan error) {
	return getTokensIncrementallyByWalletAddressWithFallback(ctx, address, f.Primary, f.Fallback, f.resolveTokens, nil)
}

func (f SyncWithContractEvalFallbackProvider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan ChainAgnosticTokensAndContracts, <-chan error) {
	return f.Primary.GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
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
	if !f.Eval(token) {
		token.TokenMetadata = f.callFallbackIdentifiers(ctx, token).TokenMetadata
	}
	return token, contract, nil
}

func (f SyncWithContractEvalFallbackProvider) GetContractsByOwnerAddress(ctx context.Context, address persist.Address) ([]ChainAgnosticContract, error) {
	return f.Primary.GetContractsByOwnerAddress(ctx, address)
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
			if !f.Eval(token) {
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
	if err == nil && f.Eval(backup) {
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

func (f SyncFailureFallbackProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]ChainAgnosticToken, []ChainAgnosticContract, error) {
	tokens, contracts, err := f.Primary.GetTokensByWalletAddress(ctx, address)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("failed to get tokens by wallet address from primary in failure fallback")
		return f.Fallback.GetTokensByWalletAddress(ctx, address)
	}

	return tokens, contracts, nil
}

func (f SyncFailureFallbackProvider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan ChainAgnosticTokensAndContracts, <-chan error) {
	return getTokensIncrementallyByWalletAddressWithFallback(ctx, address, f.Primary, f.Fallback, nil, nil)
}

func (f SyncFailureFallbackProvider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan ChainAgnosticTokensAndContracts, <-chan error) {
	return f.Primary.GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
}

func getTokensIncrementallyByWalletAddressWithFallback(ctx context.Context, address persist.Address, primary TokensIncrementalOwnerFetcher, fallback any, processTokens func(context.Context, []ChainAgnosticToken) []ChainAgnosticToken, processContracts func(context.Context, []ChainAgnosticContract) []ChainAgnosticContract) (<-chan ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan ChainAgnosticTokensAndContracts)
	errChan := make(chan error)

	go func() {
		defer close(rec)
		// create sub channels so that we can separately keep track of the results and errors and handle them here as opposed to the original receiving channels
		subRec, subErrChan := primary.GetTokensIncrementallyByWalletAddress(ctx, address)
		for {
			select {
			case err := <-subErrChan:
				// FIXIT maybe we return an error from the primary that specifies what tokens failed so we don't have to go through the whole process again on a fallback
				if tiof, ok := fallback.(TokensIncrementalOwnerFetcher); ok {
					logger.For(ctx).Warnf("failed to get tokens incrementally by wallet address from primary in failure fallback: %s", err)
					fallbackRec, fallbackErrChan := tiof.GetTokensIncrementallyByWalletAddress(ctx, address)
					for {
						select {
						case err := <-fallbackErrChan:
							logger.For(ctx).Warnf("failed to get tokens incrementally by wallet address from fallback in failure fallback: %s", err)
							errChan <- err
							return
						case tokens, ok := <-fallbackRec:
							if !ok {
								return
							}
							if processTokens != nil {
								tokens.Tokens = processTokens(ctx, tokens.Tokens)
							}
							if processContracts != nil {
								tokens.Contracts = processContracts(ctx, tokens.Contracts)
							}
							rec <- tokens
						}
					}
				}
				errChan <- err
				return

			case tokens, ok := <-subRec:
				if !ok {
					return
				}
				if processTokens != nil {
					tokens.Tokens = processTokens(ctx, tokens.Tokens)
				}
				if processContracts != nil {
					tokens.Contracts = processContracts(ctx, tokens.Contracts)
				}
				rec <- tokens
			}
		}
	}()
	return rec, errChan
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
