package wrapper

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

// SyncPipelineWrapper makes a best effort to fetch tokens requested by a sync.
// Specifically, SyncPipelineWrapper searches every applicable provider to find tokens and
// fills missing token fields with data from a supplemental provider.
type SyncPipelineWrapper struct {
	Chain                            persist.Chain
	TokenIdentifierOwnerFetcher      multichain.TokenIdentifierOwnerFetcher
	TokensIncrementalOwnerFetcher    multichain.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetcher multichain.TokensIncrementalContractFetcher
	TokenMetadataBatcher             multichain.TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher  multichain.TokensByTokenIdentifiersFetcher
	TokensByContractWalletFetcher    multichain.TokensByContractWalletFetcher
	CustomMetadataWrapper            *multichain.CustomMetadataHandlers
}

func (w SyncPipelineWrapper) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, address persist.Address) (t multichain.ChainAgnosticToken, c multichain.ChainAgnosticContract, err error) {
	t, c, err = w.TokenIdentifierOwnerFetcher.GetTokenByTokenIdentifiersAndOwner(ctx, ti, address)
	t = w.CustomMetadataWrapper.AddToToken(ctx, w.Chain, t)
	return t, c, err
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalOwnerFetcher.GetTokensIncrementallyByWalletAddress(ctx, address)
	recCh, errCh = w.CustomMetadataWrapper.AddToPage(ctx, w.Chain, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalContractFetcher.GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	recCh, errCh = w.CustomMetadataWrapper.AddToPage(ctx, w.Chain, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	t, c, err := w.TokensByTokenIdentifiersFetcher.GetTokensByTokenIdentifiers(ctx, ti)
	t = w.CustomMetadataWrapper.LoadAll(ctx, w.Chain, t)
	return t, c, err
}

func (w SyncPipelineWrapper) GetTokensByContractWallet(ctx context.Context, c persist.ChainAddress, wallet persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (w SyncPipelineWrapper) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	metadata, err := w.GetTokenMetadataByTokenIdentifiersBatch(ctx, []multichain.ChainAgnosticIdentifiers{ti})
	return metadata[0], err
}

func (w SyncPipelineWrapper) GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []multichain.ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error) {
	ret := make([]persist.TokenMetadata, len(tIDs))
	noCustomHandlerBatch := make([]multichain.ChainAgnosticIdentifiers, 0, len(tIDs))
	noCustomHandlerResultIdxToInputIdx := make(map[int]int)

	// Separate tokens that have custom metadata handlers from those that don't
	for i, tID := range tIDs {
		t := multichain.ChainAgnosticIdentifiers{ContractAddress: tID.ContractAddress, TokenID: tID.TokenID}
		metadata := w.CustomMetadataWrapper.Load(ctx, w.Chain, t)
		if len(metadata) > 0 {
			ret[i] = metadata
			continue
		}
		pos := len(noCustomHandlerBatch)
		noCustomHandlerBatch = append(noCustomHandlerBatch, tID)
		noCustomHandlerResultIdxToInputIdx[pos] = i
	}

	// Fetch metadata for tokens that don't have custom metadata handlers
	if len(noCustomHandlerBatch) > 0 {
		batchMetadata, err := w.TokenMetadataBatcher.GetTokenMetadataByTokenIdentifiersBatch(ctx, noCustomHandlerBatch)
		if err != nil {
			logger.For(ctx).Errorf("error fetching metadata for batch: %s", err)
			sentryutil.ReportError(ctx, err)
		} else {
			if len(batchMetadata) != len(noCustomHandlerBatch) {
				panic(fmt.Sprintf("expected length to the the same; expected=%d; got=%d", len(noCustomHandlerBatch), len(batchMetadata)))
			}
			for i := range batchMetadata {
				ret[noCustomHandlerResultIdxToInputIdx[i]] = batchMetadata[i]
			}
		}
	}

	return ret, nil
}
