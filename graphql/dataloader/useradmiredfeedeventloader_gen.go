// Code generated by github.com/gallery-so/dataloaden, DO NOT EDIT.

package dataloader

import (
	"context"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
)

type UserAdmiredFeedEventLoaderSettings interface {
	getContext() context.Context
	getWait() time.Duration
	getMaxBatchOne() int
	getMaxBatchMany() int
	getDisableCaching() bool
	getPublishResults() bool
	getSubscriptionRegistry() *[]interface{}
	getMutexRegistry() *[]*sync.Mutex
}

// UserAdmiredFeedEventLoaderCacheSubscriptions
type UserAdmiredFeedEventLoaderCacheSubscriptions struct {
	// AutoCacheWithKey is a function that returns the coredb.GetUserAdmiredFeedEventParams cache key for a bool.
	// If AutoCacheWithKey is not nil, this loader will automatically cache published results from other loaders
	// that return a bool. Loaders that return pointers or slices of bool
	// will be dereferenced/iterated automatically, invoking this function with the base bool type.
	AutoCacheWithKey func(bool) coredb.GetUserAdmiredFeedEventParams

	// AutoCacheWithKeys is a function that returns the []coredb.GetUserAdmiredFeedEventParams cache keys for a bool.
	// Similar to AutoCacheWithKey, but for cases where a single value gets cached by many keys.
	// If AutoCacheWithKeys is not nil, this loader will automatically cache published results from other loaders
	// that return a bool. Loaders that return pointers or slices of bool
	// will be dereferenced/iterated automatically, invoking this function with the base bool type.
	AutoCacheWithKeys func(bool) []coredb.GetUserAdmiredFeedEventParams

	// TODO: Allow custom cache functions once we're able to use generics. It could be done without generics, but
	// would be messy and error-prone. A non-generic implementation might look something like:
	//
	//   CustomCacheFuncs []func(primeFunc func(key, value)) func(typeToRegisterFor interface{})
	//
	// where each CustomCacheFunc is a closure that receives this loader's unsafePrime method and returns a
	// function that accepts the type it's registering for and uses that type and the unsafePrime method
	// to prime the cache.
}

func (l *UserAdmiredFeedEventLoader) setContext(ctx context.Context) {
	l.ctx = ctx
}

func (l *UserAdmiredFeedEventLoader) setWait(wait time.Duration) {
	l.wait = wait
}

func (l *UserAdmiredFeedEventLoader) setMaxBatch(maxBatch int) {
	l.maxBatch = maxBatch
}

func (l *UserAdmiredFeedEventLoader) setDisableCaching(disableCaching bool) {
	l.disableCaching = disableCaching
}

func (l *UserAdmiredFeedEventLoader) setPublishResults(publishResults bool) {
	l.publishResults = publishResults
}

// NewUserAdmiredFeedEventLoader creates a new UserAdmiredFeedEventLoader with the given settings, functions, and options
func NewUserAdmiredFeedEventLoader(
	settings UserAdmiredFeedEventLoaderSettings, fetch func(ctx context.Context, keys []coredb.GetUserAdmiredFeedEventParams) ([]bool, []error),
	funcs UserAdmiredFeedEventLoaderCacheSubscriptions,
	opts ...func(interface {
		setContext(context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}),
) *UserAdmiredFeedEventLoader {
	loader := &UserAdmiredFeedEventLoader{
		ctx:                  settings.getContext(),
		wait:                 settings.getWait(),
		disableCaching:       settings.getDisableCaching(),
		publishResults:       settings.getPublishResults(),
		subscriptionRegistry: settings.getSubscriptionRegistry(),
		mutexRegistry:        settings.getMutexRegistry(),
		maxBatch:             settings.getMaxBatchOne(),
	}

	for _, opt := range opts {
		opt(loader)
	}

	// Set this after applying options, in case a different context was set via options
	loader.fetch = func(keys []coredb.GetUserAdmiredFeedEventParams) ([]bool, []error) { return fetch(loader.ctx, keys) }

	if loader.subscriptionRegistry == nil {
		panic("subscriptionRegistry may not be nil")
	}

	if loader.mutexRegistry == nil {
		panic("mutexRegistry may not be nil")
	}

	if !loader.disableCaching {
		// One-to-one mappings: cache one value with one key
		if funcs.AutoCacheWithKey != nil {
			cacheFunc := func(t bool) {
				loader.unsafePrime(funcs.AutoCacheWithKey(t), t)
			}
			loader.registerCacheFunc(&cacheFunc, &loader.mu)
		}

		// One-to-many mappings: cache one value with many keys
		if funcs.AutoCacheWithKeys != nil {
			cacheFunc := func(t bool) {
				keys := funcs.AutoCacheWithKeys(t)
				for _, key := range keys {
					loader.unsafePrime(key, t)
				}
			}
			loader.registerCacheFunc(&cacheFunc, &loader.mu)
		}
	}

	return loader
}

// UserAdmiredFeedEventLoader batches and caches requests
type UserAdmiredFeedEventLoader struct {
	// context passed to fetch functions
	ctx context.Context

	// this method provides the data for the loader
	fetch func(keys []coredb.GetUserAdmiredFeedEventParams) ([]bool, []error)

	// how long to wait before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// whether this dataloader will cache results
	disableCaching bool

	// whether this dataloader will publish its results for others to cache
	publishResults bool

	// a shared slice where dataloaders will register and invoke caching functions.
	// the same slice should be passed to every dataloader.
	subscriptionRegistry *[]interface{}

	// a shared slice, parallel to the subscription registry, that holds a reference to the
	// cache mutex for the subscription's dataloader
	mutexRegistry *[]*sync.Mutex

	// INTERNAL

	// lazily created cache
	cache map[coredb.GetUserAdmiredFeedEventParams]bool

	// typed cache functions
	//subscribers []func(bool)
	subscribers []userAdmiredFeedEventLoaderSubscriber

	// functions used to cache published results from other dataloaders
	cacheFuncs []interface{}

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userAdmiredFeedEventLoaderBatch

	// mutex to prevent races
	mu sync.Mutex

	// only initialize our typed subscription cache once
	once sync.Once
}

type userAdmiredFeedEventLoaderBatch struct {
	keys    []coredb.GetUserAdmiredFeedEventParams
	data    []bool
	error   []error
	closing bool
	done    chan struct{}
}

// Load a bool by key, batching and caching will be applied automatically
func (l *UserAdmiredFeedEventLoader) Load(key coredb.GetUserAdmiredFeedEventParams) (bool, error) {
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a bool.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserAdmiredFeedEventLoader) LoadThunk(key coredb.GetUserAdmiredFeedEventParams) func() (bool, error) {
	l.mu.Lock()
	if !l.disableCaching {
		if it, ok := l.cache[key]; ok {
			l.mu.Unlock()
			return func() (bool, error) {
				return it, nil
			}
		}
	}
	if l.batch == nil {
		l.batch = &userAdmiredFeedEventLoaderBatch{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return func() (bool, error) {
		<-batch.done

		var data bool
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
func (l *UserAdmiredFeedEventLoader) LoadAll(keys []coredb.GetUserAdmiredFeedEventParams) ([]bool, []error) {
	results := make([]func() (bool, error), len(keys))

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

	bools := make([]bool, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range results {
		bools[i], errors[i] = thunk()
	}
	return bools, errors
}

// LoadAllThunk returns a function that when called will block waiting for a bools.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserAdmiredFeedEventLoader) LoadAllThunk(keys []coredb.GetUserAdmiredFeedEventParams) func() ([]bool, []error) {
	results := make([]func() (bool, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([]bool, []error) {
		bools := make([]bool, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range results {
			bools[i], errors[i] = thunk()
		}
		return bools, errors
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *UserAdmiredFeedEventLoader) Prime(key coredb.GetUserAdmiredFeedEventParams, value bool) bool {
	if l.disableCaching {
		return false
	}
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		l.unsafeSet(key, value)
	}
	l.mu.Unlock()
	return !found
}

// Prime the cache without acquiring locks. Should only be used when the lock is already held.
func (l *UserAdmiredFeedEventLoader) unsafePrime(key coredb.GetUserAdmiredFeedEventParams, value bool) bool {
	if l.disableCaching {
		return false
	}
	var found bool
	if _, found = l.cache[key]; !found {
		l.unsafeSet(key, value)
	}
	return !found
}

// Clear the value at key from the cache, if it exists
func (l *UserAdmiredFeedEventLoader) Clear(key coredb.GetUserAdmiredFeedEventParams) {
	if l.disableCaching {
		return
	}
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

func (l *UserAdmiredFeedEventLoader) unsafeSet(key coredb.GetUserAdmiredFeedEventParams, value bool) {
	if l.cache == nil {
		l.cache = map[coredb.GetUserAdmiredFeedEventParams]bool{}
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *userAdmiredFeedEventLoaderBatch) keyIndex(l *UserAdmiredFeedEventLoader, key coredb.GetUserAdmiredFeedEventParams) int {
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

func (b *userAdmiredFeedEventLoaderBatch) startTimer(l *UserAdmiredFeedEventLoader) {
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

func (b *userAdmiredFeedEventLoaderBatch) end(l *UserAdmiredFeedEventLoader) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}

type userAdmiredFeedEventLoaderSubscriber struct {
	cacheFunc func(bool)
	mutex     *sync.Mutex
}

func (l *UserAdmiredFeedEventLoader) publishToSubscribers(value bool) {
	// Lazy build our list of typed cache functions once
	l.once.Do(func() {
		for i, subscription := range *l.subscriptionRegistry {
			if typedFunc, ok := subscription.(*func(bool)); ok {
				// Don't invoke our own cache function
				if !l.ownsCacheFunc(typedFunc) {
					l.subscribers = append(l.subscribers, userAdmiredFeedEventLoaderSubscriber{cacheFunc: *typedFunc, mutex: (*l.mutexRegistry)[i]})
				}
			}
		}
	})

	// Handling locking here (instead of in the subscribed functions themselves) isn't the
	// ideal pattern, but it's an optimization that allows the publisher to iterate over slices
	// without having to acquire the lock many times.
	for _, s := range l.subscribers {
		s.mutex.Lock()
		s.cacheFunc(value)
		s.mutex.Unlock()
	}
}

func (l *UserAdmiredFeedEventLoader) registerCacheFunc(cacheFunc interface{}, mutex *sync.Mutex) {
	l.cacheFuncs = append(l.cacheFuncs, cacheFunc)
	*l.subscriptionRegistry = append(*l.subscriptionRegistry, cacheFunc)
	*l.mutexRegistry = append(*l.mutexRegistry, mutex)
}

func (l *UserAdmiredFeedEventLoader) ownsCacheFunc(f *func(bool)) bool {
	for _, cacheFunc := range l.cacheFuncs {
		if cacheFunc == f {
			return true
		}
	}

	return false
}
