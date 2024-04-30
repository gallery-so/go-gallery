package batch

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type batchStatus int

const (
	Open batchStatus = iota
	Closed
)

type Batcher[TParam any, TResult any] struct {
	ctx context.Context

	maxBatchSize   int
	batchTimeout   time.Duration
	cacheResults   bool
	publishResults bool
	batchFunc      func([]TParam) ([]TResult, []error)

	currentBatchID    int32
	batches           sync.Map
	resultCache       sync.Map
	subscribers       []func(TResult)
	subscribersMu     sync.RWMutex
	paramIsComparable bool
	paramIndexFunc    func([]TParam, TParam) int
}

type batch[TParam any, TResult any] struct {
	batcher    *Batcher[TParam, TResult]
	id         int32
	params     []TParam
	jsonParams []string
	results    []TResult
	errors     []error
	status     batchStatus
	done       chan struct{}
	mu         sync.Mutex
	numCallers int32
}

// NewBatcher creates a new Batcher with a TParam type that is comparable (i.e. can be used as a map key). If TParam is not comparable, use NewBatcherWithNonComparableParam instead.
// The batcher will batch requests up to maxBatchSize (or until batchTimeout has elapsed), and then it will call the batchFunc with all the parameters. If cacheResults is true, the
// results will be cached and returned for subsequent calls with the same parameter. If publishResults is true, the results will be published to any registered subscribers.
func NewBatcher[TParam comparable, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	batchFunc func(context.Context, []TParam) ([]TResult, []error)) *Batcher[TParam, TResult] {
	return newBatcher(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, batchFunc, indexOf[TParam])
}

// NewBatcherWithNonComparableParam creates a new Batcher with a TParam type that is not comparable (i.e. cannot be used as a map key). The underlying implementation will
// serialize the parameter to JSON for use as a key, which allows for parameter types (e.g. structs with nested arrays) that would otherwise not be usable.
// The batcher will batch requests up to maxBatchSize (or until batchTimeout has elapsed), and then it will call the batchFunc with all the parameters. If cacheResults is true, the
// results will be cached and returned for subsequent calls with the same parameter. If publishResults is true, the results will be published to any registered subscribers.
func NewBatcherWithNonComparableParam[TParam any, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	batchFunc func(context.Context, []TParam) ([]TResult, []error)) *Batcher[TParam, TResult] {
	return newBatcher(ctx, maxBatchSize, batchTimeout, cacheResults, publishResults, batchFunc, nil)
}

func newBatcher[TParam any, TResult any](ctx context.Context, maxBatchSize int, batchTimeout time.Duration, cacheResults bool, publishResults bool,
	batchFunc func(context.Context, []TParam) ([]TResult, []error),
	paramIndexFunc func([]TParam, TParam) int) *Batcher[TParam, TResult] {
	return &Batcher[TParam, TResult]{
		ctx:               ctx,
		maxBatchSize:      maxBatchSize,
		batchTimeout:      batchTimeout,
		cacheResults:      cacheResults,
		publishResults:    publishResults,
		batchFunc:         func(params []TParam) ([]TResult, []error) { return batchFunc(ctx, params) },
		paramIndexFunc:    paramIndexFunc,
		paramIsComparable: paramIndexFunc != nil,
	}
}

// Do gets a TResult for the specified param, with batching and caching applied automatically (if configured).
// If the specified TParam is already in this batch, the underlying batch function will only receive the parameter
// once, and both callers will receive the result of that invocation.
func (d *Batcher[TParam, TResult]) Do(param TParam) (TResult, error) {
	return d.DoThunk(param)()
}

// DoThunk returns a function that when called will block waiting for a TResult.
// This method should be used if you want one goroutine to make requests to many
// different batchers without blocking until the thunk is called.
func (d *Batcher[TParam, TResult]) DoThunk(param TParam) func() (TResult, error) {
	var jsonParam string
	if !d.paramIsComparable {
		var err error
		jsonParam, err = paramToJSON(param)
		if err != nil {
			return func() (TResult, error) {
				return *new(TResult), err
			}
		}
	}

	if d.cacheResults {
		if d.paramIsComparable {
			if value, ok := d.resultCache.Load(param); ok {
				return func() (TResult, error) {
					return value.(TResult), nil
				}
			}
		} else {
			if value, ok := d.resultCache.Load(jsonParam); ok {
				return func() (TResult, error) {
					return value.(TResult), nil
				}
			}
		}
	}

	b, index := d.addParamToBatch(param, jsonParam)

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
				if d.paramIsComparable {
					d.resultCache.LoadOrStore(param, result)
				} else {
					d.resultCache.LoadOrStore(jsonParam, result)
				}
			}
		}

		return result, err
	}
}

// DoAll gets TResults for many params at once. It will be broken into appropriate sized
// sub batches depending on how the batcher is configured
func (d *Batcher[TParam, TResult]) DoAll(params []TParam) ([]TResult, []error) {
	resultThunks := make([]func() (TResult, error), len(params))
	for i, param := range params {
		resultThunks[i] = d.DoThunk(param)
	}

	results := make([]TResult, len(params))
	errors := make([]error, len(params))
	for i, thunk := range resultThunks {
		results[i], errors[i] = thunk()
	}

	return results, errors
}

// DoAllThunk returns a function that when called will block waiting for results.
// This method should be used if you want one goroutine to make requests to many
// different batchers without blocking until the thunk is called.
func (d *Batcher[TParam, TResult]) DoAllThunk(params []TParam) func() ([]TResult, []error) {
	resultThunks := make([]func() (TResult, error), len(params))
	for i, param := range params {
		resultThunks[i] = d.DoThunk(param)
	}

	return func() ([]TResult, []error) {
		results := make([]TResult, len(params))
		errors := make([]error, len(params))
		for i, thunk := range resultThunks {
			results[i], errors[i] = thunk()
		}
		return results, errors
	}
}

// Prime caches a result for the specified param. This can be used to prime the cache with known results,
// but is only useful if caching is enabled for this batcher.
func (d *Batcher[TParam, TResult]) Prime(param TParam, result TResult) {
	if !d.cacheResults {
		return
	}

	if d.paramIsComparable {
		d.resultCache.LoadOrStore(param, result)
	} else {
		jsonParam, err := paramToJSON(param)
		if err != nil {
			return
		}

		d.resultCache.LoadOrStore(jsonParam, result)
	}
}

// RegisterResultSubscriber registers a function that will be called for every
// result that is returned by this batcher.
func (d *Batcher[TParam, TResult]) RegisterResultSubscriber(subscriber func(TResult)) {
	d.subscribersMu.Lock()
	defer d.subscribersMu.Unlock()
	d.subscribers = append(d.subscribers, subscriber)
}

func (d *Batcher[TParam, TResult]) newBatch(batchID int32) *batch[TParam, TResult] {
	b := batch[TParam, TResult]{
		batcher: d,
		id:      batchID,
		params:  make([]TParam, 0, d.maxBatchSize),
		done:    make(chan struct{}),
	}

	if !d.paramIsComparable {
		b.jsonParams = make([]string, 0, d.maxBatchSize)
	}

	return &b
}

func (b *batch[TParam, TResult]) closeAfterTimeout(timeout time.Duration) {
	time.Sleep(timeout)

	b.mu.Lock()

	if b.status == Open {
		b.status = Closed
		b.mu.Unlock()
		b.submitBatch()
	} else {
		b.mu.Unlock()
	}
}

func (d *Batcher[TParam, TResult]) addParamToBatch(param TParam, jsonParam string) (*batch[TParam, TResult], int) {
	for {
		// Read the current batch ID
		currentID := atomic.LoadInt32(&d.currentBatchID)

		// Attempt to load the batch first. LoadOrStore requires that we create a "just in case" batch
		// to store if the param isn't present. Since we expect the batch to exist most of the time, these
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

		b := actual.(*batch[TParam, TResult])

		// Prevent lock contention within a batch by allowing only the first maxBatchSize callers
		// to obtain the lock.
		numAssigned := atomic.AddInt32(&b.numCallers, 1)
		if numAssigned > int32(d.maxBatchSize) {
			atomic.CompareAndSwapInt32(&d.currentBatchID, currentID, currentID+1)
			continue
		}

		b.mu.Lock()

		// If the batch we were assigned to is closed, increment the batch ID and try again
		if b.status == Closed {
			b.mu.Unlock()
			atomic.CompareAndSwapInt32(&d.currentBatchID, currentID, currentID+1)
			continue
		}

		var paramIndex int

		if d.paramIsComparable {
			// If the param is comparable, look for it in the params slice
			paramIndex = d.paramIndexFunc(b.params, param)
		} else {
			// If the param is not comparable, look for its JSON representation in the jsonParams slice
			paramIndex = indexOf(b.jsonParams, jsonParam)
		}

		// If the param is already in the batch, return the batch and the param's index
		if paramIndex != -1 {
			b.mu.Unlock()
			return b, paramIndex
		}

		// Otherwise, add the param to the batch
		paramIndex = len(b.params)

		b.params = append(b.params, param)
		if !d.paramIsComparable {
			b.jsonParams = append(b.jsonParams, jsonParam)
		}

		// If this is the first thing we've added to the batch, start the timeout
		if paramIndex == 0 {
			go b.closeAfterTimeout(d.batchTimeout)
		}

		if len(b.params) == d.maxBatchSize {
			b.status = Closed
			b.mu.Unlock()
			b.submitBatch()
			atomic.CompareAndSwapInt32(&d.currentBatchID, currentID, currentID+1)
		} else {
			b.mu.Unlock()
		}

		return b, paramIndex
	}
}

func paramToJSON[TParam any](param TParam) (string, error) {
	bytes, err := json.Marshal(param)
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

func (d *Batcher[TParam, TResult]) publishToSubscribers(results []TResult, errors []error) {
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

func (b *batch[TParam, TResult]) submitBatch() {
	b.results, b.errors = b.batcher.batchFunc(b.params)

	if b.batcher.publishResults {
		b.batcher.publishToSubscribers(b.results, b.errors)
	}

	close(b.done)
}
