package generator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type batchStatus int

const (
	Open batchStatus = iota
	Closed
)

type Dataloader[TKey any, TResult any] struct {
	ctx context.Context

	maxBatchSize   int
	batchTimeout   time.Duration
	cacheResults   bool
	publishResults bool
	fetchFunc      func([]TKey) ([]TResult, []error)

	currentBatchID  int32
	batches         sync.Map
	resultCache     sync.Map
	subscribers     []func(TResult)
	subscribersMu   sync.RWMutex
	keyIsComparable bool
	keyIndexFunc    func([]TKey, TKey) int
}

type batch[TKey any, TResult any] struct {
	dataloader *Dataloader[TKey, TResult]
	id         int32
	keys       []TKey
	jsonKeys   []string
	results    []TResult
	errors     []error
	status     batchStatus
	done       chan struct{}
	mu         sync.Mutex
}

func NewDataloader[TKey comparable, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	fetchFunc func(context.Context, []TKey) ([]TResult, []error)) *Dataloader[TKey, TResult] {
	return newDataloader(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, fetchFunc, indexOf[TKey])
}

func NewDataloaderWithNonComparableKey[TKey any, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	fetchFunc func(context.Context, []TKey) ([]TResult, []error)) *Dataloader[TKey, TResult] {
	return newDataloader(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, fetchFunc, nil)
}

func newDataloader[TKey any, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	fetchFunc func(context.Context, []TKey) ([]TResult, []error),
	keyIndexFunc func([]TKey, TKey) int) *Dataloader[TKey, TResult] {
	return &Dataloader[TKey, TResult]{
		ctx:             ctx,
		maxBatchSize:    maxBatchSize,
		batchTimeout:    batchTimeout,
		cacheResults:    cacheResults,
		publishResults:  publishResults,
		fetchFunc:       func(keys []TKey) ([]TResult, []error) { return fetchFunc(ctx, keys) },
		keyIndexFunc:    keyIndexFunc,
		keyIsComparable: keyIndexFunc != nil,
	}
}

// Load a ContractCreator by key, batching and caching will be applied automatically
func (d *Dataloader[TKey, TResult]) Load(key TKey) (TResult, error) {
	return d.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a ContractCreator.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (d *Dataloader[TKey, TResult]) LoadThunk(key TKey) func() (TResult, error) {
	var jsonKey string
	if !d.keyIsComparable {
		var err error
		jsonKey, err = keyToJSON(key)
		if err != nil {
			return func() (TResult, error) {
				return *new(TResult), err
			}
		}
	}

	if d.cacheResults {
		if d.keyIsComparable {
			if value, ok := d.resultCache.Load(key); ok {
				return func() (TResult, error) {
					return value.(TResult), nil
				}
			}
		} else {
			if value, ok := d.resultCache.Load(jsonKey); ok {
				return func() (TResult, error) {
					return value.(TResult), nil
				}
			}
		}
	}

	b, index := d.addKeyToBatch(key, jsonKey)

	return func() (TResult, error) {
		<-b.done

		var result TResult
		if index < len(b.results) {
			result = b.results[index]
		}

		var err error
		// its convenient to be able to return a single error for everything
		if len(b.errors) == 1 {
			err = b.errors[0]
		} else if b.errors != nil {
			err = b.errors[index]
		}

		if err == nil {
			if d.cacheResults {
				if d.keyIsComparable {
					d.resultCache.LoadOrStore(key, result)
				} else {
					d.resultCache.LoadOrStore(jsonKey, result)
				}
			}
		}

		return result, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (d *Dataloader[TKey, TResult]) LoadAll(keys []TKey) ([]TResult, []error) {
	resultThunks := make([]func() (TResult, error), len(keys))
	for i, key := range keys {
		resultThunks[i] = d.LoadThunk(key)
	}

	results := make([]TResult, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range resultThunks {
		results[i], errors[i] = thunk()
	}

	return results, errors
}

// LoadAllThunk returns a function that when called will block waiting for results.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (d *Dataloader[TKey, TResult]) LoadAllThunk(keys []TKey) func() ([]TResult, []error) {
	resultThunks := make([]func() (TResult, error), len(keys))
	for i, key := range keys {
		resultThunks[i] = d.LoadThunk(key)
	}

	return func() ([]TResult, []error) {
		results := make([]TResult, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range resultThunks {
			results[i], errors[i] = thunk()
		}
		return results, errors
	}
}

func (d *Dataloader[TKey, TResult]) Prime(key TKey, result TResult) {
	if !d.cacheResults {
		return
	}

	if d.keyIsComparable {
		d.resultCache.LoadOrStore(key, result)
	} else {
		jsonKey, err := keyToJSON(key)
		if err != nil {
			return
		}

		d.resultCache.LoadOrStore(jsonKey, result)
	}
}

// RegisterResultSubscriber registers a function that will be called with every
// result that is loaded.
func (d *Dataloader[TKey, TResult]) RegisterResultSubscriber(subscriber func(TResult)) {
	d.subscribersMu.Lock()
	defer d.subscribersMu.Unlock()
	d.subscribers = append(d.subscribers, subscriber)
}

func (d *Dataloader[TKey, TResult]) newBatch(batchID int32) *batch[TKey, TResult] {
	b := batch[TKey, TResult]{
		dataloader: d,
		id:         batchID,
		keys:       make([]TKey, 0, d.maxBatchSize),
		done:       make(chan struct{}),
	}

	if !d.keyIsComparable {
		b.jsonKeys = make([]string, 0, d.maxBatchSize)
	}

	return &b
}

func (b *batch[TKey, TResult]) closeAfterTimeout(timeout time.Duration) {
	time.Sleep(timeout)

	b.mu.Lock()

	if b.status == Open {
		b.status = Closed
		b.mu.Unlock()
		fmt.Printf("batch %d closed after %v timeout\n", b.id, timeout)
		b.submitBatch()
	} else {
		b.mu.Unlock()
	}
}

func (d *Dataloader[TKey, TResult]) addKeyToBatch(key TKey, jsonKey string) (*batch[TKey, TResult], int) {
	for {
		// Read the current batch ID
		currentID := atomic.LoadInt32(&d.currentBatchID)

		// Attempt to load the batch first. LoadOrStore requires that we create a "just in case" batch
		// to store if the key isn't present. Since we expect the batch to exist most of the time, these
		// "just in case" batches will usually be unnecessary. To avoid creating many unused structs,
		// we try to load the batch first, and only call LoadOrStore if we can't find an existing batch.
		actual, ok := d.batches.Load(currentID)

		// If the batch doesn't exist, create it
		if !ok {
			newBatch := d.newBatch(currentID)

			var loaded bool
			actual, loaded = d.batches.LoadOrStore(currentID, newBatch)
			if loaded {
				// If newBatch wasn't actually stored, close the Done channel since it won't be used
				close(newBatch.done)
			}
		}

		b := actual.(*batch[TKey, TResult])

		b.mu.Lock()

		// If the batch we were assigned to is closed, increment the batch ID and try again
		if b.status == Closed {
			b.mu.Unlock()
			atomic.CompareAndSwapInt32(&d.currentBatchID, currentID, currentID+1)
			continue
		}

		var keyIndex int

		if d.keyIsComparable {
			// If the key is comparable, look for it in the keys slice
			keyIndex = d.keyIndexFunc(b.keys, key)
		} else {
			// If the key is not comparable, look for its JSON representation in the jsonKeys slice
			keyIndex = indexOf(b.jsonKeys, jsonKey)
		}

		// If the key is already in the batch, return the batch and the key's index
		if keyIndex != -1 {
			b.mu.Unlock()
			return b, keyIndex
		}

		// Otherwise, add the key to the batch
		keyIndex = len(b.keys)

		b.keys = append(b.keys, key)
		if !d.keyIsComparable {
			b.jsonKeys = append(b.jsonKeys, jsonKey)
		}

		// If this is the first thing we've added to the batch, start the timeout
		if keyIndex == 0 {
			go b.closeAfterTimeout(d.batchTimeout)
		}

		if len(b.keys) == d.maxBatchSize {
			b.status = Closed
			fmt.Printf("batch %d closed after reaching max batch size of %d\n", b.id, d.maxBatchSize)
			b.mu.Unlock()
			atomic.CompareAndSwapInt32(&d.currentBatchID, currentID, currentID+1)
		} else {
			b.mu.Unlock()
		}

		return b, keyIndex
	}
}

func keyToJSON[TKey any](key TKey) (string, error) {
	bytes, err := json.Marshal(key)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func indexOf[T comparable](slice []T, item T) int {
	for i, existingItem := range slice {
		if item == existingItem {
			return i
		}
	}

	return -1
}

func (d *Dataloader[TKey, TResult]) publishToSubscribers(results []TResult, errors []error) {
	// Only hold the mutex long enough to get a snapshot of the subscribers
	d.subscribersMu.RLock()
	subscribers := d.subscribers
	d.subscribersMu.RUnlock()

	for i, result := range results {
		// Only publish results that didn't return an error
		if errors[i] != nil {
			continue
		}

		for _, s := range subscribers {
			s(result)
		}
	}

}

func (b *batch[TKey, TResult]) submitBatch() {
	b.results, b.errors = b.dataloader.fetchFunc(b.keys)

	if b.dataloader.publishResults {
		b.dataloader.publishToSubscribers(b.results, b.errors)
	}

	close(b.done)
}
