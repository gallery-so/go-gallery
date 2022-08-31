// Code generated by github.com/vektah/dataloaden, DO NOT EDIT.

package dataloader

import (
	"sync"
	"time"

<<<<<<< HEAD
	"github.com/mikeydub/go-gallery/db/gen/coredb"
=======
	"github.com/mikeydub/go-gallery/db/sqlc/coregen"
>>>>>>> 93a3a41 (Add indexer models)
)

// UserFeedLoaderConfig captures the config to create a new UserFeedLoader
type UserFeedLoaderConfig struct {
	// Fetch is a method that provides the data for the loader
<<<<<<< HEAD
	Fetch func(keys []coredb.GetUserFeedViewBatchParams) ([][]coredb.FeedEvent, []error)
=======
	Fetch func(keys []coregen.GetUserFeedViewBatchParams) ([][]coregen.FeedEvent, []error)
>>>>>>> 93a3a41 (Add indexer models)

	// Wait is how long wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = not limit
	MaxBatch int
}

// NewUserFeedLoader creates a new UserFeedLoader given a fetch, wait, and maxBatch
func NewUserFeedLoader(config UserFeedLoaderConfig) *UserFeedLoader {
	return &UserFeedLoader{
		fetch:    config.Fetch,
		wait:     config.Wait,
		maxBatch: config.MaxBatch,
	}
}

// UserFeedLoader batches and caches requests
type UserFeedLoader struct {
	// this method provides the data for the loader
<<<<<<< HEAD
	fetch func(keys []coredb.GetUserFeedViewBatchParams) ([][]coredb.FeedEvent, []error)
=======
	fetch func(keys []coregen.GetUserFeedViewBatchParams) ([][]coregen.FeedEvent, []error)
>>>>>>> 93a3a41 (Add indexer models)

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// INTERNAL

	// lazily created cache
<<<<<<< HEAD
	cache map[coredb.GetUserFeedViewBatchParams][]coredb.FeedEvent
=======
	cache map[coregen.GetUserFeedViewBatchParams][]coregen.FeedEvent
>>>>>>> 93a3a41 (Add indexer models)

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userFeedLoaderBatch

	// mutex to prevent races
	mu sync.Mutex
}

type userFeedLoaderBatch struct {
<<<<<<< HEAD
	keys    []coredb.GetUserFeedViewBatchParams
	data    [][]coredb.FeedEvent
=======
	keys    []coregen.GetUserFeedViewBatchParams
	data    [][]coregen.FeedEvent
>>>>>>> 93a3a41 (Add indexer models)
	error   []error
	closing bool
	done    chan struct{}
}

// Load a FeedEvent by key, batching and caching will be applied automatically
<<<<<<< HEAD
func (l *UserFeedLoader) Load(key coredb.GetUserFeedViewBatchParams) ([]coredb.FeedEvent, error) {
=======
func (l *UserFeedLoader) Load(key coregen.GetUserFeedViewBatchParams) ([]coregen.FeedEvent, error) {
>>>>>>> 93a3a41 (Add indexer models)
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a FeedEvent.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
<<<<<<< HEAD
func (l *UserFeedLoader) LoadThunk(key coredb.GetUserFeedViewBatchParams) func() ([]coredb.FeedEvent, error) {
	l.mu.Lock()
	if it, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return func() ([]coredb.FeedEvent, error) {
=======
func (l *UserFeedLoader) LoadThunk(key coregen.GetUserFeedViewBatchParams) func() ([]coregen.FeedEvent, error) {
	l.mu.Lock()
	if it, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return func() ([]coregen.FeedEvent, error) {
>>>>>>> 93a3a41 (Add indexer models)
			return it, nil
		}
	}
	if l.batch == nil {
		l.batch = &userFeedLoaderBatch{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

<<<<<<< HEAD
	return func() ([]coredb.FeedEvent, error) {
		<-batch.done

		var data []coredb.FeedEvent
=======
	return func() ([]coregen.FeedEvent, error) {
		<-batch.done

		var data []coregen.FeedEvent
>>>>>>> 93a3a41 (Add indexer models)
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
			l.mu.Lock()
			l.unsafeSet(key, data)
			l.mu.Unlock()
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
<<<<<<< HEAD
func (l *UserFeedLoader) LoadAll(keys []coredb.GetUserFeedViewBatchParams) ([][]coredb.FeedEvent, []error) {
	results := make([]func() ([]coredb.FeedEvent, error), len(keys))
=======
func (l *UserFeedLoader) LoadAll(keys []coregen.GetUserFeedViewBatchParams) ([][]coregen.FeedEvent, []error) {
	results := make([]func() ([]coregen.FeedEvent, error), len(keys))
>>>>>>> 93a3a41 (Add indexer models)

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

<<<<<<< HEAD
	feedEvents := make([][]coredb.FeedEvent, len(keys))
=======
	feedEvents := make([][]coregen.FeedEvent, len(keys))
>>>>>>> 93a3a41 (Add indexer models)
	errors := make([]error, len(keys))
	for i, thunk := range results {
		feedEvents[i], errors[i] = thunk()
	}
	return feedEvents, errors
}

// LoadAllThunk returns a function that when called will block waiting for a FeedEvents.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
<<<<<<< HEAD
func (l *UserFeedLoader) LoadAllThunk(keys []coredb.GetUserFeedViewBatchParams) func() ([][]coredb.FeedEvent, []error) {
	results := make([]func() ([]coredb.FeedEvent, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([][]coredb.FeedEvent, []error) {
		feedEvents := make([][]coredb.FeedEvent, len(keys))
=======
func (l *UserFeedLoader) LoadAllThunk(keys []coregen.GetUserFeedViewBatchParams) func() ([][]coregen.FeedEvent, []error) {
	results := make([]func() ([]coregen.FeedEvent, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([][]coregen.FeedEvent, []error) {
		feedEvents := make([][]coregen.FeedEvent, len(keys))
>>>>>>> 93a3a41 (Add indexer models)
		errors := make([]error, len(keys))
		for i, thunk := range results {
			feedEvents[i], errors[i] = thunk()
		}
		return feedEvents, errors
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
<<<<<<< HEAD
func (l *UserFeedLoader) Prime(key coredb.GetUserFeedViewBatchParams, value []coredb.FeedEvent) bool {
=======
func (l *UserFeedLoader) Prime(key coregen.GetUserFeedViewBatchParams, value []coregen.FeedEvent) bool {
>>>>>>> 93a3a41 (Add indexer models)
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
<<<<<<< HEAD
		cpy := make([]coredb.FeedEvent, len(value))
=======
		cpy := make([]coregen.FeedEvent, len(value))
>>>>>>> 93a3a41 (Add indexer models)
		copy(cpy, value)
		l.unsafeSet(key, cpy)
	}
	l.mu.Unlock()
	return !found
}

// Clear the value at key from the cache, if it exists
<<<<<<< HEAD
func (l *UserFeedLoader) Clear(key coredb.GetUserFeedViewBatchParams) {
=======
func (l *UserFeedLoader) Clear(key coregen.GetUserFeedViewBatchParams) {
>>>>>>> 93a3a41 (Add indexer models)
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

<<<<<<< HEAD
func (l *UserFeedLoader) unsafeSet(key coredb.GetUserFeedViewBatchParams, value []coredb.FeedEvent) {
	if l.cache == nil {
		l.cache = map[coredb.GetUserFeedViewBatchParams][]coredb.FeedEvent{}
=======
func (l *UserFeedLoader) unsafeSet(key coregen.GetUserFeedViewBatchParams, value []coregen.FeedEvent) {
	if l.cache == nil {
		l.cache = map[coregen.GetUserFeedViewBatchParams][]coregen.FeedEvent{}
>>>>>>> 93a3a41 (Add indexer models)
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
<<<<<<< HEAD
func (b *userFeedLoaderBatch) keyIndex(l *UserFeedLoader, key coredb.GetUserFeedViewBatchParams) int {
=======
func (b *userFeedLoaderBatch) keyIndex(l *UserFeedLoader, key coregen.GetUserFeedViewBatchParams) int {
>>>>>>> 93a3a41 (Add indexer models)
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

func (b *userFeedLoaderBatch) startTimer(l *UserFeedLoader) {
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

func (b *userFeedLoaderBatch) end(l *UserFeedLoader) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}
