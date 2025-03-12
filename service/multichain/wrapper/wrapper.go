package wrapper

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/multichain/custom"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
)

// SyncPipelineWrapper makes a best effort to fetch tokens requested by a sync.
// Specifically, SyncPipelineWrapper searches every applicable provider to find tokens and
// fills missing token fields with data from a supplemental provider.
type SyncPipelineWrapper struct {
	Chain                            persist.Chain
	TokenIdentifierOwnerFetcher      common.TokenIdentifierOwnerFetcher
	TokensIncrementalOwnerFetcher    common.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetcher common.TokensIncrementalContractFetcher
	TokenMetadataBatcher             common.TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher  common.TokensByTokenIdentifiersFetcher
	CustomMetadataWrapper            *custom.CustomMetadataHandlers
}

func (w SyncPipelineWrapper) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti common.ChainAgnosticIdentifiers, address persist.Address) (t common.ChainAgnosticToken, c common.ChainAgnosticContract, err error) {
	t, c, err = w.TokenIdentifierOwnerFetcher.GetTokenByTokenIdentifiersAndOwner(ctx, ti, address)
	t = w.CustomMetadataWrapper.AddToToken(ctx, w.Chain, t)
	return t, c, err
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalOwnerFetcher.GetTokensIncrementallyByWalletAddress(ctx, address)
	recCh, errCh = w.CustomMetadataWrapper.AddToPage(ctx, w.Chain, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalContractFetcher.GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	recCh, errCh = w.CustomMetadataWrapper.AddToPage(ctx, w.Chain, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	t, c, err := w.TokensByTokenIdentifiersFetcher.GetTokensByTokenIdentifiers(ctx, ti)
	t = w.CustomMetadataWrapper.LoadAll(ctx, w.Chain, t)
	return t, c, err
}

func (w SyncPipelineWrapper) GetTokensByContractWallet(ctx context.Context, address persist.ChainAddress, wallet persist.Address) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	return []common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, fmt.Errorf("not implemented")
	//t, c, err := w.TokensByContractWalletFetcher.GetTokensByContractWallet(ctx, address, wallet)
	//t = w.CustomMetadataWrapper.LoadAll(ctx, w.Chain, t)
	//return t, c, err
}

func (w SyncPipelineWrapper) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	metadata, err := w.GetTokenMetadataByTokenIdentifiersBatch(ctx, []common.ChainAgnosticIdentifiers{ti})
	return metadata[0], err
}

func (w SyncPipelineWrapper) GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []common.ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error) {
	ret := make([]persist.TokenMetadata, len(tIDs))
	noCustomHandlerBatch := make([]common.ChainAgnosticIdentifiers, 0, len(tIDs))
	noCustomHandlerResultIdxToInputIdx := make(map[int]int)

	// Separate tokens that have custom metadata handlers from those that don't
	for i, tID := range tIDs {
		t := common.ChainAgnosticIdentifiers{ContractAddress: tID.ContractAddress, TokenID: tID.TokenID}
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
