package tokenprocessing

import (
	"context"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

// MetadataFinder is a service for fetching metadata for a token
// Batching pattern adapted from dataloaden (https://github.com/vektah/dataloaden)
type MetadataFinder struct {
	mc       *multichain.Provider
	ctx      context.Context
	mu       sync.Mutex
	batch    *batch
	wait     time.Duration
	maxBatch int
}

func (m *MetadataFinder) GetMetadata(ctx context.Context, t persist.TokenIdentifiers) (persist.TokenMetadata, error) {
	return m.add(ctx, t)()
}

func (m *MetadataFinder) add(ctx context.Context, t persist.TokenIdentifiers) func() (persist.TokenMetadata, error) {
	if _, ok := m.mc.Chains[t.Chain].(common.TokenMetadataBatcher); !ok {
		return func() (persist.TokenMetadata, error) {
			return m.mc.GetTokenMetadataByTokenIdentifiers(ctx, t.ContractAddress, t.TokenID, t.Chain)
		}
	}

	m.mu.Lock()

	if m.batch == nil {
		m.batch = &batch{
			tokens:  make(map[persist.Chain][]persist.TokenIdentifiers),
			results: make(map[persist.Chain][]persist.TokenMetadata),
			errors:  make(map[persist.Chain]error),
			done:    make(chan struct{}),
		}
	}

	b := m.batch
	pos := b.addToBatch(m, t)

	m.mu.Unlock()

	return func() (persist.TokenMetadata, error) {
		<-b.done
		if err := b.errors[t.Chain]; err != nil {
			return persist.TokenMetadata{}, err
		}
		return b.results[t.Chain][pos], nil
	}
}

type batch struct {
	total   int
	tokens  map[persist.Chain][]persist.TokenIdentifiers
	done    chan struct{}
	closing bool
	errors  map[persist.Chain]error
	results map[persist.Chain][]persist.TokenMetadata
}

func (b *batch) addToBatch(m *MetadataFinder, t persist.TokenIdentifiers) int {
	tot := b.total
	pos := len(b.tokens[t.Chain])
	b.tokens[t.Chain] = append(b.tokens[t.Chain], t)
	b.total++
	if tot == 0 {
		go b.startTimer(m)
	}
	if m.maxBatch != 0 && tot >= m.maxBatch-1 {
		if !b.closing {
			b.closing = true
			m.batch = nil
			go b.end(m)
		}
	}
	return pos
}

func (b *batch) startTimer(m *MetadataFinder) {
	time.Sleep(m.wait)
	m.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		m.mu.Unlock()
		return
	}

	m.batch = nil
	m.mu.Unlock()

	b.end(m)
}

func (b *batch) end(m *MetadataFinder) {
	for chain, tokens := range b.tokens {
		tIDs := util.MapWithoutError(tokens, func(t persist.TokenIdentifiers) common.ChainAgnosticIdentifiers {
			return common.ChainAgnosticIdentifiers{ContractAddress: t.ContractAddress, TokenID: t.TokenID}
		})
		metadata, err := m.mc.GetTokenMetadataByTokenIdentifiersBatch(m.ctx, chain, tIDs)
		if err != nil {
			logger.For(m.ctx).Errorf("failed to load batch of metadata for chain=%s: %s", chain, err)
			b.errors[chain] = err
		} else {
			b.results[chain] = metadata
		}
	}
	close(b.done)
}
