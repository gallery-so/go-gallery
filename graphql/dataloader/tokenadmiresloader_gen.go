// Code generated by github.com/gallery-so/dataloaden, DO NOT EDIT.

package dataloader

import (
	"context"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
)

type TokenAdmiresLoaderSettings interface {
	getContext() context.Context
	getWait() time.Duration
	getMaxBatchOne() int
	getMaxBatchMany() int
	getDisableCaching() bool
	getPublishResults() bool
	getPreFetchHook() func(context.Context, string) context.Context
	getPostFetchHook() func(context.Context, string)
	getSubscriptionRegistry() *[]interface{}
	getMutexRegistry() *[]*sync.Mutex
}

func (l *TokenAdmiresLoader) setContext(ctx context.Context) {
	l.ctx = ctx
}

func (l *TokenAdmiresLoader) setWait(wait time.Duration) {
	l.wait = wait
}

func (l *TokenAdmiresLoader) setMaxBatch(maxBatch int) {
	l.maxBatch = maxBatch
}

func (l *TokenAdmiresLoader) setDisableCaching(disableCaching bool) {
	l.disableCaching = disableCaching
}

func (l *TokenAdmiresLoader) setPublishResults(publishResults bool) {
	l.publishResults = publishResults
}

func (l *TokenAdmiresLoader) setPreFetchHook(preFetchHook func(context.Context, string) context.Context) {
	l.preFetchHook = preFetchHook
}

func (l *TokenAdmiresLoader) setPostFetchHook(postFetchHook func(context.Context, string)) {
	l.postFetchHook = postFetchHook
}

// NewTokenAdmiresLoader creates a new TokenAdmiresLoader with the given settings, functions, and options
func NewTokenAdmiresLoader(
	settings TokenAdmiresLoaderSettings, fetch func(ctx context.Context, keys []coredb.PaginateAdmiresByTokenIDBatchParams) ([][]coredb.Admire, []error),
	opts ...func(interface {
		setContext(context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
		setPreFetchHook(func(context.Context, string) context.Context)
		setPostFetchHook(func(context.Context, string))
	}),
) *TokenAdmiresLoader {
	loader := &TokenAdmiresLoader{
		ctx:                  settings.getContext(),
		wait:                 settings.getWait(),
		disableCaching:       settings.getDisableCaching(),
		publishResults:       settings.getPublishResults(),
		preFetchHook:         settings.getPreFetchHook(),
		postFetchHook:        settings.getPostFetchHook(),
		subscriptionRegistry: settings.getSubscriptionRegistry(),
		mutexRegistry:        settings.getMutexRegistry(),
		maxBatch:             settings.getMaxBatchMany(),
	}

	for _, opt := range opts {
		opt(loader)
	}

	// Set this after applying options, in case a different context was set via options
	loader.fetch = func(keys []coredb.PaginateAdmiresByTokenIDBatchParams) ([][]coredb.Admire, []error) {
		ctx := loader.ctx

		// Allow the preFetchHook to modify and return a new context
		if loader.preFetchHook != nil {
			ctx = loader.preFetchHook(ctx, "TokenAdmiresLoader")
		}

		results, errors := fetch(ctx, keys)

		if loader.postFetchHook != nil {
			loader.postFetchHook(ctx, "TokenAdmiresLoader")
		}

		return results, errors
	}

	if loader.subscriptionRegistry == nil {
		panic("subscriptionRegistry may not be nil")
	}

	if loader.mutexRegistry == nil {
		panic("mutexRegistry may not be nil")
	}

	// No cache functions here; caching isn't very useful for dataloaders that return slices. This dataloader can
	// still send its results to other cache-priming receivers, but it won't register its own cache-priming function.

	return loader
}

// TokenAdmiresLoader batches and caches requests
type TokenAdmiresLoader struct {
	// context passed to fetch functions
	ctx context.Context

	// this method provides the data for the loader
	fetch func(keys []coredb.PaginateAdmiresByTokenIDBatchParams) ([][]coredb.Admire, []error)

	// how long to wait before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// whether this dataloader will cache results
	disableCaching bool

	// whether this dataloader will publish its results for others to cache
	publishResults bool

	// a hook invoked before the fetch operation, useful for things like tracing.
	// the returned context will be passed to the fetch operation.
	preFetchHook func(ctx context.Context, loaderName string) context.Context

	// a hook invoked after the fetch operation, useful for things like tracing
	postFetchHook func(ctx context.Context, loaderName string)

	// a shared slice where dataloaders will register and invoke caching functions.
	// the same slice should be passed to every dataloader.
	subscriptionRegistry *[]interface{}

	// a shared slice, parallel to the subscription registry, that holds a reference to the
	// cache mutex for the subscription's dataloader
	mutexRegistry *[]*sync.Mutex

	// INTERNAL

	// lazily created cache
	cache map[coredb.PaginateAdmiresByTokenIDBatchParams][]coredb.Admire

	// typed cache functions
	//subscribers []func([]coredb.Admire)
	subscribers []tokenAdmiresLoaderSubscriber

	// functions used to cache published results from other dataloaders
	cacheFuncs []interface{}

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *tokenAdmiresLoaderBatch

	// mutex to prevent races
	mu sync.Mutex

	// only initialize our typed subscription cache once
	once sync.Once
}

type tokenAdmiresLoaderBatch struct {
	keys    []coredb.PaginateAdmiresByTokenIDBatchParams
	data    [][]coredb.Admire
	error   []error
	closing bool
	done    chan struct{}
}

// Load a Admire by key, batching and caching will be applied automatically
func (l *TokenAdmiresLoader) Load(key coredb.PaginateAdmiresByTokenIDBatchParams) ([]coredb.Admire, error) {
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a Admire.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *TokenAdmiresLoader) LoadThunk(key coredb.PaginateAdmiresByTokenIDBatchParams) func() ([]coredb.Admire, error) {
	l.mu.Lock()
	if !l.disableCaching {
		if it, ok := l.cache[key]; ok {
			l.mu.Unlock()
			return func() ([]coredb.Admire, error) {
				return it, nil
			}
		}
	}
	if l.batch == nil {
		l.batch = &tokenAdmiresLoaderBatch{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return func() ([]coredb.Admire, error) {
		<-batch.done

		var data []coredb.Admire
		if pos < len(batch.data) {
			data = batch.data[pos]
		}

		var err error
		// its convenient to be able to return a single error for everything
		if len(batch.error) == 1 {
			err = batch.error[0]
		} else if batch.error != nil {
			err = batch.error[pos]
		}

		if err == nil {
			if !l.disableCaching {
				l.mu.Lock()
				l.unsafeSet(key, data)
				l.mu.Unlock()
			}

			if l.publishResults {
				l.publishToSubscribers(data)
			}
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (l *TokenAdmiresLoader) LoadAll(keys []coredb.PaginateAdmiresByTokenIDBatchParams) ([][]coredb.Admire, []error) {
	results := make([]func() ([]coredb.Admire, error), len(keys))

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

	admires := make([][]coredb.Admire, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range results {
		admires[i], errors[i] = thunk()
	}
	return admires, errors
}

// LoadAllThunk returns a function that when called will block waiting for a Admires.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *TokenAdmiresLoader) LoadAllThunk(keys []coredb.PaginateAdmiresByTokenIDBatchParams) func() ([][]coredb.Admire, []error) {
	results := make([]func() ([]coredb.Admire, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([][]coredb.Admire, []error) {
		admires := make([][]coredb.Admire, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range results {
			admires[i], errors[i] = thunk()
		}
		return admires, errors
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *TokenAdmiresLoader) Prime(key coredb.PaginateAdmiresByTokenIDBatchParams, value []coredb.Admire) bool {
	if l.disableCaching {
		return false
	}
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
		cpy := make([]coredb.Admire, len(value))
		copy(cpy, value)
		l.unsafeSet(key, cpy)
	}
	l.mu.Unlock()
	return !found
}

// Clear the value at key from the cache, if it exists
func (l *TokenAdmiresLoader) Clear(key coredb.PaginateAdmiresByTokenIDBatchParams) {
	if l.disableCaching {
		return
	}
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

func (l *TokenAdmiresLoader) unsafeSet(key coredb.PaginateAdmiresByTokenIDBatchParams, value []coredb.Admire) {
	if l.cache == nil {
		l.cache = map[coredb.PaginateAdmiresByTokenIDBatchParams][]coredb.Admire{}
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *tokenAdmiresLoaderBatch) keyIndex(l *TokenAdmiresLoader, key coredb.PaginateAdmiresByTokenIDBatchParams) int {
	for i, existingKey := range b.keys {
		if key == existingKey {
			return i
		}
	}

	pos := len(b.keys)
	b.keys = append(b.keys, key)
	if pos == 0 {
		go b.startTimer(l)
	}

	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			b.closing = true
			l.batch = nil
			go b.end(l)
		}
	}

	return pos
}

func (b *tokenAdmiresLoaderBatch) startTimer(l *TokenAdmiresLoader) {
	time.Sleep(l.wait)
	l.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		l.mu.Unlock()
		return
	}

	l.batch = nil
	l.mu.Unlock()

	b.end(l)
}

func (b *tokenAdmiresLoaderBatch) end(l *TokenAdmiresLoader) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}

type tokenAdmiresLoaderSubscriber struct {
	cacheFunc func(coredb.Admire)
	mutex     *sync.Mutex
}

func (l *TokenAdmiresLoader) publishToSubscribers(value []coredb.Admire) {
	// Lazy build our list of typed cache functions once
	l.once.Do(func() {
		for i, subscription := range *l.subscriptionRegistry {
			if typedFunc, ok := subscription.(*func(coredb.Admire)); ok {
				// Don't invoke our own cache function
				if !l.ownsCacheFunc(typedFunc) {
					l.subscribers = append(l.subscribers, tokenAdmiresLoaderSubscriber{cacheFunc: *typedFunc, mutex: (*l.mutexRegistry)[i]})
				}
			}
		}
	})

	// Handling locking here (instead of in the subscribed functions themselves) isn't the
	// ideal pattern, but it's an optimization that allows the publisher to iterate over slices
	// without having to acquire the lock many times.
	for _, s := range l.subscribers {
		s.mutex.Lock()
		for _, v := range value {
			s.cacheFunc(v)
		}
		s.mutex.Unlock()
	}
}

func (l *TokenAdmiresLoader) registerCacheFunc(cacheFunc interface{}, mutex *sync.Mutex) {
	l.cacheFuncs = append(l.cacheFuncs, cacheFunc)
	*l.subscriptionRegistry = append(*l.subscriptionRegistry, cacheFunc)
	*l.mutexRegistry = append(*l.mutexRegistry, mutex)
}

func (l *TokenAdmiresLoader) ownsCacheFunc(f *func(coredb.Admire)) bool {
	for _, cacheFunc := range l.cacheFuncs {
		if cacheFunc == f {
			return true
		}
	}

	return false
}
