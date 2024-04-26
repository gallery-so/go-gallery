package reservoir

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	mc "github.com/mikeydub/go-gallery/service/multichain"
)

// TODO: Consider abtracting the batching pattern into a separate package at some point

var TotalTokenRequests atomic.Uint64      // for debugging
var TotalCollectionRequests atomic.Uint64 // for debugging

// batchToken groups multiple token requests into a single batch request
// Batching pattern adapted from dataloaden (https://github.com/vektah/dataloaden)
type batchToken struct {
	provider *Provider
	ctx      context.Context
	mu       sync.Mutex
	batch    *batchTokenMessage
	wait     time.Duration
	maxBatch int
}

func (b *batchToken) Get(ctx context.Context, t mc.ChainAgnosticIdentifiers) (reservoirToken, error) {
	return b.add(ctx, t)()
}

func (b *batchToken) add(ctx context.Context, t mc.ChainAgnosticIdentifiers) func() (reservoirToken, error) {
	b.mu.Lock()
	if b.batch == nil {
		b.batch = &batchTokenMessage{done: make(chan struct{})}
	}
	batch := b.batch
	pos := batch.add(b, t)
	b.mu.Unlock()
	return func() (reservoirToken, error) {
		<-batch.done
		if err := batch.errors[pos]; err != nil {
			return reservoirToken{}, batch.errors[pos]
		}
		return batch.results[pos], nil
	}
}

type batchTokenMessage struct {
	total   int
	done    chan struct{}
	closing bool
	tokens  []mc.ChainAgnosticIdentifiers
	results []reservoirToken
	errors  []error
}

func (b *batchTokenMessage) add(bt *batchToken, t mc.ChainAgnosticIdentifiers) int {
	tot := b.total
	pos := len(b.tokens)
	b.tokens = append(b.tokens, t)
	b.total++
	if tot == 0 {
		go b.startTimer(bt)
	}
	if bt.maxBatch != 0 && tot >= bt.maxBatch-1 {
		if !b.closing {
			b.closing = true
			bt.batch = nil
			go b.end(bt)
		}
	}
	return pos
}

func (b *batchTokenMessage) startTimer(bt *batchToken) {
	time.Sleep(bt.wait)
	bt.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		bt.mu.Unlock()
		return
	}

	bt.batch = nil
	bt.mu.Unlock()

	b.end(bt)
}

func (b *batchTokenMessage) end(bt *batchToken) {
	TotalTokenRequests.Add(uint64(1))
	results, errors := bt.provider.getTokenBatch(bt.ctx, b.tokens)
	b.results = results
	b.errors = errors
	close(b.done)
}

// batchCollection groups multiple token requests into a single batch request
// Batching pattern adapted from dataloaden (https://github.com/vektah/dataloaden)
type batchCollection struct {
	provider *Provider
	ctx      context.Context
	mu       sync.Mutex
	batch    *batchCollectionMessage
	wait     time.Duration
	maxBatch int
}

func (b *batchCollection) Get(ctx context.Context, collectionID string) (reservoirCollection, error) {
	return b.add(ctx, collectionID)()
}

func (b *batchCollection) add(ctx context.Context, collectionID string) func() (reservoirCollection, error) {
	b.mu.Lock()
	if b.batch == nil {
		b.batch = &batchCollectionMessage{done: make(chan struct{})}
	}
	batch := b.batch
	pos := batch.add(b, collectionID)
	b.mu.Unlock()
	return func() (reservoirCollection, error) {
		<-batch.done
		if err := batch.errors[pos]; err != nil {
			return reservoirCollection{}, batch.errors[pos]
		}
		return batch.results[pos], nil
	}
}

type batchCollectionMessage struct {
	total         int
	done          chan struct{}
	closing       bool
	collectionIDs []string
	results       []reservoirCollection
	errors        []error
}

func (b *batchCollectionMessage) add(bc *batchCollection, collectionID string) int {
	tot := b.total
	pos := len(b.collectionIDs)
	b.collectionIDs = append(b.collectionIDs, collectionID)
	b.total++
	if tot == 0 {
		go b.startTimer(bc)
	}
	if bc.maxBatch != 0 && tot >= bc.maxBatch-1 {
		if !b.closing {
			b.closing = true
			bc.batch = nil
			go b.end(bc)
		}
	}
	return pos
}

func (b *batchCollectionMessage) startTimer(bc *batchCollection) {
	time.Sleep(bc.wait)
	bc.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		bc.mu.Unlock()
		return
	}

	bc.batch = nil
	bc.mu.Unlock()

	b.end(bc)
}

func (b *batchCollectionMessage) end(bc *batchCollection) {
	TotalCollectionRequests.Add(uint64(1))
	results, errors := bc.provider.getCollectionBatch(bc.ctx, b.collectionIDs)
	b.results = results
	b.errors = errors
	close(b.done)
}
