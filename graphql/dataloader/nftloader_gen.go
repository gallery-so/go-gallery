// Code generated by github.com/vektah/dataloaden, DO NOT EDIT.

package dataloader

import (
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
)

// NftLoaderConfig captures the config to create a new NftLoader
type NftLoaderConfig struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []string) ([]persist.NFT, []error)

	// Wait is how long wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = not limit
	MaxBatch int
}

// NewNftLoader creates a new NftLoader given a fetch, wait, and maxBatch
func NewNftLoader(config NftLoaderConfig) *NftLoader {
	return &NftLoader{
		fetch:    config.Fetch,
		wait:     config.Wait,
		maxBatch: config.MaxBatch,
	}
}

// NftLoader batches and caches requests
type NftLoader struct {
	// this method provides the data for the loader
	fetch func(keys []string) ([]persist.NFT, []error)

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// INTERNAL

	// lazily created cache
	cache map[string]persist.NFT

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *nftLoaderBatch

	// mutex to prevent races
	mu sync.Mutex
}

type nftLoaderBatch struct {
	keys    []string
	data    []persist.NFT
	error   []error
	closing bool
	done    chan struct{}
}

// Load a NFT by key, batching and caching will be applied automatically
func (l *NftLoader) Load(key string) (persist.NFT, error) {
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a NFT.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *NftLoader) LoadThunk(key string) func() (persist.NFT, error) {
	l.mu.Lock()
	if it, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return func() (persist.NFT, error) {
			return it, nil
		}
	}
	if l.batch == nil {
		l.batch = &nftLoaderBatch{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return func() (persist.NFT, error) {
		<-batch.done

		var data persist.NFT
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
func (l *NftLoader) LoadAll(keys []string) ([]persist.NFT, []error) {
	results := make([]func() (persist.NFT, error), len(keys))

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

	nFTs := make([]persist.NFT, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range results {
		nFTs[i], errors[i] = thunk()
	}
	return nFTs, errors
}

// LoadAllThunk returns a function that when called will block waiting for a NFTs.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *NftLoader) LoadAllThunk(keys []string) func() ([]persist.NFT, []error) {
	results := make([]func() (persist.NFT, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([]persist.NFT, []error) {
		nFTs := make([]persist.NFT, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range results {
			nFTs[i], errors[i] = thunk()
		}
		return nFTs, errors
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *NftLoader) Prime(key string, value persist.NFT) bool {
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		l.unsafeSet(key, value)
	}
	l.mu.Unlock()
	return !found
}

// Clear the value at key from the cache, if it exists
func (l *NftLoader) Clear(key string) {
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

func (l *NftLoader) unsafeSet(key string, value persist.NFT) {
	if l.cache == nil {
		l.cache = map[string]persist.NFT{}
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *nftLoaderBatch) keyIndex(l *NftLoader, key string) int {
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

func (b *nftLoaderBatch) startTimer(l *NftLoader) {
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

func (b *nftLoaderBatch) end(l *NftLoader) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}
