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
	"github.com/mikeydub/go-gallery/util"
)

var (
	MultiProviderWapperOptions MultiProviderWrapperOpts
)

type SyncPipelineWrapper struct {
	TokenIdentifierOwnerFetcher      multichain.TokenIdentifierOwnerFetcher
	TokensIncrementalOwnerFetcher    multichain.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetcher multichain.TokensIncrementalContractFetcher
	FillInWrapper                    *FillInWrapper
}

func NewSyncPipelineWrapper(
	ctx context.Context,
	tokenIdentifierOwnerFetcher multichain.TokenIdentifierOwnerFetcher,
	tokensIncrementalOwnerFetcher multichain.TokensIncrementalOwnerFetcher,
	tokensIncrementalContractFetcher multichain.TokensIncrementalContractFetcher,
	fillInWrapper *FillInWrapper,
) *SyncPipelineWrapper {
	return &SyncPipelineWrapper{
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
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

// MultiProviderWrapper handles calling into multiple providers. Depending on the calling context, providers are called in parallel or in series.
// In some cases, the first provider to return a result is used, in others, the results are combined.
type MultiProviderWrapper struct {
	TokensIncrementalOwnerFetchers    [2]multichain.TokensIncrementalOwnerFetcher
	TokensIncrementalContractFetchers [2]multichain.TokensIncrementalContractFetcher
	TokenDescriptorsFetchers          [2]multichain.TokenDescriptorsFetcher
	TokenMetadataFetchers             [2]multichain.TokenMetadataFetcher
	ContractFetchers                  [2]multichain.ContractFetcher
	TokenIdentifierOwnerFetchers      [2]multichain.TokenIdentifierOwnerFetcher
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

func (m MultiProviderWrapper) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, 2*10)
	errCh := make(chan error, 2)
	resultA, errA := m.TokensIncrementalContractFetchers[0].GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	resultB, errB := m.TokensIncrementalContractFetchers[1].GetTokensIncrementallyByContractAddress(ctx, address, maxLimit)
	go func() { fanIn(ctx, recCh, errCh, resultA, resultB, errA, errB) }()
	return recCh, errCh
}

func (m MultiProviderWrapper) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, 2*10)
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

func (w *FillInWrapper) AddToToken(ctx context.Context, t multichain.ChainAgnosticToken) multichain.ChainAgnosticToken {
	f, err := w.addToken(t)()
	if err != nil {
		return t
	}
	if !t.FallbackMedia.IsServable() {
		fmt.Println("filling in fallback media!")
		t.FallbackMedia = f.FallbackMedia
	}
	if _, _, err := media.FindMediaURLsChain(t.TokenMetadata, w.chain); err != nil {
		fmt.Println("filling in tokenmedata!")
		t.TokenMetadata = f.TokenMetadata
	}
	return t
}

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
		logger.For(ctx).Info("closing out channel")
	}()
	return outCh, errOut
}

func (w *FillInWrapper) addPage(p multichain.ChainAgnosticTokensAndContracts) func() multichain.ChainAgnosticTokensAndContracts {
	thunks := make([]func() (multichain.ChainAgnosticToken, error), len(p.Tokens))
	for i, t := range p.Tokens {
		thunks[i] = w.addToken(t)
	}

	return func() multichain.ChainAgnosticTokensAndContracts {
		var err error
		for i, thunk := range thunks {
			p.Tokens[i], err = thunk()
			if err != nil {
				logger.For(w.ctx).Warnf("failed to get fallbacks for page: %s", err)
			}
		}
		return p
	}
}

func (w *FillInWrapper) addToken(t multichain.ChainAgnosticToken) func() (multichain.ChainAgnosticToken, error) {
	ti := persist.TokenIdentifiers{
		TokenID:         t.TokenID,
		ContractAddress: t.ContractAddress,
		Chain:           w.chain,
	}

	if v, ok := w.resultCache.Load(ti); ok {
		fmt.Println("loading from cache!")
		return func() (multichain.ChainAgnosticToken, error) { return v.(multichain.ChainAgnosticToken), nil }
	}

	if _, _, err := media.FindMediaURLsChain(t.TokenMetadata, w.chain); err == nil && t.FallbackMedia.IsServable() {
		fmt.Println("token is already filled!")
		return func() (multichain.ChainAgnosticToken, error) { return t, nil }
	}

	w.mu.Lock()

	if w.batch == nil {
		w.batch = &batch{done: make(chan struct{})}
	}

	b := w.batch
	pos := b.addToBatch(w, ti)

	w.mu.Unlock()

	return func() (multichain.ChainAgnosticToken, error) {
		<-b.done
		if b.err != nil {
			return multichain.ChainAgnosticToken{}, b.err
		}
		return b.results[pos], nil
	}
}

type batch struct {
	tokens  []persist.TokenIdentifiers
	err     error
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
	b.results, b.err = w.reservoirProvider.GetTokensByTokenIdentifiersBatch(ctx, b.tokens)
	if b.err == nil {
		for i := range b.results {
			w.resultCache.Store(b.tokens[i], b.results[i])
		}
	}
	close(b.done)
}
