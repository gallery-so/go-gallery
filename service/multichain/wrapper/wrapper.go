package wrapper

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
)

var (
	MultiProviderWapperOptions MultiProviderWrapperOpts
)

// SyncPipelineWrapper makes a best effort to fetch tokens requested by a sync.
// Specifically, it searches every configured provider to find tokens and enriches
// the token data with a supplemental provider.
type SyncPipelineWrapper struct {
	TokenIdentifierOwnerFetcher      multichain.TokenIdentifierOwnerFetcher
	TokensIncrementalOwnerFetcher    multichain.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetcher multichain.TokensIncrementalContractFetcher
	TokenMetadataBatcher             multichain.TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher  multichain.TokensByTokenIdentifiersFetcher
	FillInWrapper                    *FillInWrapper
}

func NewSyncPipelineWrapper(
	ctx context.Context,
	tokenIdentifierOwnerFetcher multichain.TokenIdentifierOwnerFetcher,
	tokensIncrementalOwnerFetcher multichain.TokensIncrementalOwnerFetcher,
	tokensIncrementalContractFetcher multichain.TokensIncrementalContractFetcher,
	tokenMetadataBatcher multichain.TokenMetadataBatcher,
	fillInWrapper *FillInWrapper,
) *SyncPipelineWrapper {
	return &SyncPipelineWrapper{
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             tokenMetadataBatcher,
		FillInWrapper:                    fillInWrapper,
	}
}

func (w SyncPipelineWrapper) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, address persist.Address) (t multichain.ChainAgnosticToken, c multichain.ChainAgnosticContract, err error) {
	t, c, err = w.TokenIdentifierOwnerFetcher.GetTokenByTokenIdentifiersAndOwner(ctx, ti, address)
	t = w.FillInWrapper.AddToToken(ctx, t)
	return t, c, err
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalOwnerFetcher.GetTokensIncrementallyByWalletAddress(ctx, address)
	recCh, errCh = w.FillInWrapper.AddToPage(ctx, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh, errCh := w.TokensIncrementalContractFetcher.GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	recCh, errCh = w.FillInWrapper.AddToPage(ctx, recCh, errCh)
	return recCh, errCh
}

func (w SyncPipelineWrapper) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	t, c, err := w.TokensByTokenIdentifiersFetcher.GetTokensByTokenIdentifiers(ctx, ti)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	t = w.FillInWrapper.LoadAll(t)
	return t, c, nil
}

func (w SyncPipelineWrapper) GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []persist.TokenIdentifiers) ([]persist.TokenMetadata, error) {
	metadatas, err := w.TokenMetadataBatcher.GetTokenMetadataByTokenIdentifiersBatch(ctx, tIDs)
	if err != nil {
		logger.For(ctx).Errorf("error fetching metadata for batch: %s", err)
		sentryutil.ReportError(ctx, err)
	}

	if len(metadatas) != len(tIDs) {
		panic(fmt.Sprintf("expected length to the the same; expected=%d; got=%d", len(tIDs), len(metadatas)))
	}

	// Convert metadatas to tokens so they can be passed to FillInWrapper
	asTokens := make([]multichain.ChainAgnosticToken, len(metadatas))
	for i, m := range metadatas {
		asTokens[i] = multichain.ChainAgnosticToken{
			TokenID:         tIDs[i].TokenID,
			ContractAddress: tIDs[i].ContractAddress,
			TokenMetadata:   m,
		}
	}

	metadatas = w.FillInWrapper.LoadMetadataAll(asTokens)
	return metadatas, nil
}

// MultiProviderWrapperOpts configures options for the MultiProviderWrapper
type MultiProviderWrapperOpts struct{}

func (o MultiProviderWrapperOpts) WithTokensIncrementalOwnerFetchers(a, b multichain.TokensIncrementalOwnerFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) {
		m.TokensIncrementalOwnerFetchers = [2]multichain.TokensIncrementalOwnerFetcher{a, b}
	}
}

func (o MultiProviderWrapperOpts) WithTokenIdentifierOwnerFetchers(a, b multichain.TokenIdentifierOwnerFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) {
		m.TokenIdentifierOwnerFetchers = [2]multichain.TokenIdentifierOwnerFetcher{a, b}
	}
}

func (o MultiProviderWrapperOpts) WithContractFetchers(a, b multichain.ContractFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) { m.ContractFetchers = [2]multichain.ContractFetcher{a, b} }
}

func (o MultiProviderWrapperOpts) WithTokenDescriptorsFetchers(a, b multichain.TokenDescriptorsFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) {
		m.TokenDescriptorsFetchers = [2]multichain.TokenDescriptorsFetcher{a, b}
	}
}

func (o MultiProviderWrapperOpts) WithTokenMetadataFetchers(a, b multichain.TokenMetadataFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) { m.TokenMetadataFetchers = [2]multichain.TokenMetadataFetcher{a, b} }
}

func (o MultiProviderWrapperOpts) WithTokensIncrementalContractFetchers(a, b multichain.TokensIncrementalContractFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) {
		m.TokensIncrementalContractFetchers = [2]multichain.TokensIncrementalContractFetcher{a, b}
	}
}

func (o MultiProviderWrapperOpts) WithTokenByTokenIdentifiersFetchers(a, b multichain.TokensByTokenIdentifiersFetcher) func(*MultiProviderWrapper) {
	return func(m *MultiProviderWrapper) {
		m.TokensByTokenIdentifiersFetchers = [2]multichain.TokensByTokenIdentifiersFetcher{a, b}
	}
}

// MultiProviderWrapper handles calling into multiple providers. Depending on the calling context, providers are called in parallel or in series.
// In some cases, the first provider to return a result is used, in others, the results are combined.
type MultiProviderWrapper struct {
	TokensIncrementalOwnerFetchers    [2]multichain.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetchers [2]multichain.TokensIncrementalContractFetcher
	TokenDescriptorsFetchers          [2]multichain.TokenDescriptorsFetcher
	TokenMetadataFetchers             [2]multichain.TokenMetadataFetcher
	ContractFetchers                  [2]multichain.ContractFetcher
	TokenIdentifierOwnerFetchers      [2]multichain.TokenIdentifierOwnerFetcher
	TokensByTokenIdentifiersFetchers  [2]multichain.TokensByTokenIdentifiersFetcher
}

func NewMultiProviderWrapper(opts ...func(*MultiProviderWrapper)) *MultiProviderWrapper {
	m := &MultiProviderWrapper{}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m MultiProviderWrapper) GetContractByAddress(ctx context.Context, address persist.Address) (c multichain.ChainAgnosticContract, err error) {
	for _, f := range m.ContractFetchers {
		if c, err = f.GetContractByAddress(ctx, address); err == nil {
			return
		}
	}
	return
}

func (m MultiProviderWrapper) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, address persist.Address) (t multichain.ChainAgnosticToken, c multichain.ChainAgnosticContract, err error) {
	for _, f := range m.TokenIdentifierOwnerFetchers {
		if t, c, err = f.GetTokenByTokenIdentifiersAndOwner(ctx, ti, address); err == nil {
			return
		}
	}
	return
}

func (m MultiProviderWrapper) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (t multichain.ChainAgnosticTokenDescriptors, c multichain.ChainAgnosticContractDescriptors, err error) {
	for _, f := range m.TokenDescriptorsFetchers {
		if t, c, err = f.GetTokenDescriptorsByTokenIdentifiers(ctx, ti); err == nil {
			return
		}
	}
	return
}

func (m MultiProviderWrapper) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (tm persist.TokenMetadata, err error) {
	for _, f := range m.TokenMetadataFetchers {
		if tm, err = f.GetTokenMetadataByTokenIdentifiers(ctx, ti); err == nil {
			return
		}
	}
	return
}

func (m MultiProviderWrapper) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (t []multichain.ChainAgnosticToken, c multichain.ChainAgnosticContract, err error) {
	for _, f := range m.TokensByTokenIdentifiersFetchers {
		if t, c, err = f.GetTokensByTokenIdentifiers(ctx, ti); err == nil {
			return
		}
	}
	return
}

func (m MultiProviderWrapper) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, 2*10)
	errCh := make(chan error, 2)
	resultA, errA := m.TokensIncrementalContractFetchers[0].GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	resultB, errB := m.TokensIncrementalContractFetchers[1].GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	go func() { fanIn(ctx, recCh, errCh, resultA, resultB, errA, errB) }()
	return recCh, errCh
}

func (m MultiProviderWrapper) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error, 2)
	resultA, errA := m.TokensIncrementalOwnerFetchers[0].GetTokensIncrementallyByWalletAddress(ctx, address)
	resultB, errB := m.TokensIncrementalOwnerFetchers[1].GetTokensIncrementallyByWalletAddress(ctx, address)
	go func() { fanIn(ctx, recCh, errCh, resultA, resultB, errA, errB) }()
	return recCh, errCh
}

func fanIn(ctx context.Context, recCh chan<- multichain.ChainAgnosticTokensAndContracts, errCh chan<- error, resultA, resultB <-chan multichain.ChainAgnosticTokensAndContracts, errA, errB <-chan error) {
	defer close(recCh)
	defer close(errCh)

	var closingA bool
	var closingB bool

	// It's possible for one provider to not have a contract that the other does. We won't
	// stop pulling data unless neither provider has the contract.
	missing := make(map[persist.ContractIdentifiers]bool)

	for {
		select {
		case page, ok := <-resultA:
			if ok {
				recCh <- page
				continue
			}
			if closingB {
				return
			}
			closingA = true
		case page, ok := <-resultB:
			if ok {
				recCh <- page
				continue
			}
			if closingA {
				return
			}
			closingB = true
		case err, ok := <-errA:
			if !ok {
				continue
			}

			if err, ok := util.ErrorAs[multichain.ErrProviderContractNotFound](err); ok {
				logger.For(ctx).Warnf("primary provider could not find contract: %s", err)
				c := persist.NewContractIdentifiers(err.Contract, err.Chain)
				if missing[c] {
					errCh <- err
				} else {
					missing[c] = true
				}
				continue
			}

			errCh <- err
		case err, ok := <-errB:
			if !ok {
				continue
			}

			if err, ok := util.ErrorAs[multichain.ErrProviderContractNotFound](err); ok {
				logger.For(ctx).Warnf("secondary provider could not find contract: %s", err)
				c := persist.NewContractIdentifiers(err.Contract, err.Chain)
				if missing[c] {
					errCh <- err
				} else {
					missing[c] = true
				}
				continue
			}

			errCh <- err
		}
	}
}

// FillInWrapper is a service for adding missing data to tokens.
// Batching pattern adapted from dataloaden (https://github.com/vektah/dataloaden)
type FillInWrapper struct {
	chain             persist.Chain
	reservoirProvider *reservoir.Provider
	ctx               context.Context
	mu                sync.Mutex
	batch             *batch
	wait              time.Duration
	maxBatch          int
	resultCache       sync.Map
}

func NewFillInWrapper(ctx context.Context, httpClient *http.Client, chain persist.Chain) *FillInWrapper {
	return &FillInWrapper{
		chain:             chain,
		reservoirProvider: reservoir.NewProvider(httpClient, chain),
		ctx:               ctx,
		wait:              250 * time.Millisecond,
		maxBatch:          10,
	}
}

// AddToToken adds missing data to a token.
func (w *FillInWrapper) AddToToken(ctx context.Context, t multichain.ChainAgnosticToken) multichain.ChainAgnosticToken {
	return w.addToken(t)()
}

// AddToPage adds missing data to each token of a provider page.
func (w *FillInWrapper) AddToPage(ctx context.Context, recCh <-chan multichain.ChainAgnosticTokensAndContracts, errIn <-chan error) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	outCh := make(chan multichain.ChainAgnosticTokensAndContracts, 2*10)
	errOut := make(chan error)
	w.resultCache = sync.Map{}
	go func() {
		defer close(outCh)
		for {
			select {
			case page, ok := <-recCh:
				if !ok {
					return
				}
				outCh <- w.addPage(page)()
			case err, ok := <-errIn:
				if ok {
					errOut <- err
					return
				}
			case <-ctx.Done():
				errOut <- ctx.Err()
				return
			}
		}
	}()
	logger.For(ctx).Info("closing out channel")
	return outCh, errOut
}

// LoaddAll fills in missing data for a slice of tokens.
func (w *FillInWrapper) LoadAll(tokens []multichain.ChainAgnosticToken) []multichain.ChainAgnosticToken {
	thunks := make([]func() multichain.ChainAgnosticToken, len(tokens))
	for i, t := range tokens {
		t := t
		thunks[i] = w.addTokenToBatch(t)
	}
	result := make([]multichain.ChainAgnosticToken, len(tokens))
	for i, thunk := range thunks {
		result[i] = thunk()
	}
	return result
}

// LoadMetadataAll returns missing metadata for a slice of tokens.
func (w *FillInWrapper) LoadMetadataAll(tokens []multichain.ChainAgnosticToken) []persist.TokenMetadata {
	thunks := make([]func() multichain.ChainAgnosticToken, len(tokens))
	for i, t := range tokens {
		t := t
		if hasMediaURLs(t.TokenMetadata, w.chain) {
			thunks[i] = func() multichain.ChainAgnosticToken { return t }
		} else {
			thunks[i] = w.addTokenToBatch(t)
		}
	}
	result := make([]persist.TokenMetadata, len(tokens))
	for i, thunk := range thunks {
		r := thunk()
		result[i] = r.TokenMetadata
	}
	return result
}

// LoadFallbackAll returns missing fallback media for a slice of tokens.
func (w *FillInWrapper) LoadFallbackAll(tokens []multichain.ChainAgnosticToken) []persist.FallbackMedia {
	thunks := make([]func() multichain.ChainAgnosticToken, len(tokens))
	for i, t := range tokens {
		t := t
		if t.FallbackMedia.IsServable() {
			thunks[i] = func() multichain.ChainAgnosticToken { return t }
		} else {
			thunks[i] = w.addTokenToBatch(t)
		}
	}
	result := make([]persist.FallbackMedia, len(tokens))
	for i, thunk := range thunks {
		r := thunk()
		result[i] = r.FallbackMedia
	}
	return result
}

func (w *FillInWrapper) addPage(p multichain.ChainAgnosticTokensAndContracts) func() multichain.ChainAgnosticTokensAndContracts {
	thunks := make([]func() multichain.ChainAgnosticToken, len(p.Tokens))
	for i, t := range p.Tokens {
		thunks[i] = w.addToken(t)
	}
	return func() multichain.ChainAgnosticTokensAndContracts {
		for i, thunk := range thunks {
			p.Tokens[i] = thunk()
		}
		return p
	}
}

func (w *FillInWrapper) addToken(t multichain.ChainAgnosticToken) func() multichain.ChainAgnosticToken {
	if hasMediaURLs(t.TokenMetadata, w.chain) && t.FallbackMedia.IsServable() {
		return func() multichain.ChainAgnosticToken { return t }
	}
	return w.addTokenToBatch(t)
}

func (w *FillInWrapper) addTokenToBatch(t multichain.ChainAgnosticToken) func() multichain.ChainAgnosticToken {
	ti := persist.TokenIdentifiers{
		TokenID:         t.TokenID,
		ContractAddress: t.ContractAddress,
		Chain:           w.chain,
	}

	if v, ok := w.resultCache.Load(ti); ok {
		return func() multichain.ChainAgnosticToken {
			f := v.(multichain.ChainAgnosticToken)
			if !t.FallbackMedia.IsServable() {
				t.FallbackMedia = f.FallbackMedia
			}
			if !hasMediaURLs(t.TokenMetadata, w.chain) {
				t.TokenMetadata = f.TokenMetadata
			}
			return t
		}
	}

	w.mu.Lock()

	if w.batch == nil {
		w.batch = &batch{done: make(chan struct{})}
	}
	b := w.batch
	pos := b.addToBatch(w, ti)

	w.mu.Unlock()

	return func() multichain.ChainAgnosticToken {
		<-b.done
		if b.errors[pos] != nil {
			return t
		}
		if !t.FallbackMedia.IsServable() {
			t.FallbackMedia = b.results[pos].FallbackMedia
		}
		if !hasMediaURLs(t.TokenMetadata, w.chain) {
			t.TokenMetadata = b.results[pos].TokenMetadata
		}
		return t
	}
}

func hasMediaURLs(metadata persist.TokenMetadata, chain persist.Chain) bool {
	_, _, err := media.FindMediaURLsChain(metadata, chain)
	return err == nil
}

type batch struct {
	tokens  []persist.TokenIdentifiers
	errors  []error
	results []multichain.ChainAgnosticToken
	closing bool
	done    chan struct{}
}

func (b *batch) addToBatch(w *FillInWrapper, t persist.TokenIdentifiers) int {
	pos := len(b.tokens)
	b.tokens = append(b.tokens, t)
	if pos == 0 {
		go b.startTimer(w)
	}

	if w.maxBatch != 0 && pos >= w.maxBatch-1 {
		if !b.closing {
			b.closing = true
			w.batch = nil
			go b.end(w)
		}
	}

	return pos
}

func (b *batch) startTimer(w *FillInWrapper) {
	time.Sleep(w.wait)
	w.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		w.mu.Unlock()
		return
	}

	w.batch = nil
	w.mu.Unlock()

	b.end(w)
}

func (b *batch) end(w *FillInWrapper) {
	ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
	defer cancel()
	b.results, b.errors = w.reservoirProvider.GetTokensByTokenIdentifiersBatch(ctx, b.tokens)
	for i := range b.results {
		if b.errors[i] == nil {
			w.resultCache.Store(b.tokens[i], b.results[i])
		}
	}
	close(b.done)
}
