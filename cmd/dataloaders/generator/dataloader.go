package generator

import (
	"context"
	"github.com/mikeydub/go-gallery/util/batch"
	"time"
)

// Dataloader is a thin wrapper around the Batcher type that provides idiomatic dataloader
// function names (i.e. "Load" instead of "Do")
type Dataloader[TKey any, TResult any] struct {
	b *batch.Batcher[TKey, TResult]
}

func NewDataloader[TKey comparable, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	fetchFunc func(context.Context, []TKey) ([]TResult, []error)) *Dataloader[TKey, TResult] {
	return &Dataloader[TKey, TResult]{
		b: batch.NewBatcher(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, fetchFunc),
	}
}

func NewDataloaderWithNonComparableKey[TKey any, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	fetchFunc func(context.Context, []TKey) ([]TResult, []error)) *Dataloader[TKey, TResult] {
	return &Dataloader[TKey, TResult]{
		b: batch.NewBatcherWithNonComparableParam(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, fetchFunc),
	}
}

// Load a TResult by key, batching and caching will be applied automatically
func (d *Dataloader[TKey, TResult]) Load(key TKey) (TResult, error) {
	return d.b.Do(key)
}

// LoadThunk returns a function that when called will block waiting for a TResult.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (d *Dataloader[TKey, TResult]) LoadThunk(key TKey) func() (TResult, error) {
	return d.b.DoThunk(key)
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (d *Dataloader[TKey, TResult]) LoadAll(keys []TKey) ([]TResult, []error) {
	return d.b.DoAll(keys)
}

// LoadAllThunk returns a function that when called will block waiting for results.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (d *Dataloader[TKey, TResult]) LoadAllThunk(keys []TKey) func() ([]TResult, []error) {
	return d.b.DoAllThunk(keys)
}

func (d *Dataloader[TKey, TResult]) Prime(key TKey, result TResult) {
	d.b.Prime(key, result)
}

// RegisterResultSubscriber registers a function that will be called with every
// result that is loaded.
func (d *Dataloader[TKey, TResult]) RegisterResultSubscriber(subscriber func(TResult)) {
	d.b.RegisterResultSubscriber(subscriber)
}
