// Code generated by github.com/vektah/dataloaden, DO NOT EDIT.

package dataloader

import (
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/db/sqlc/coregen"
	"github.com/mikeydub/go-gallery/service/persist"
)

// WalletLoaderByIdConfig captures the config to create a new WalletLoaderById
type WalletLoaderByIdConfig struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []persist.DBID) ([]coregen.Wallet, []error)

	// Wait is how long wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = not limit
	MaxBatch int
}

// NewWalletLoaderById creates a new WalletLoaderById given a fetch, wait, and maxBatch
func NewWalletLoaderById(config WalletLoaderByIdConfig) *WalletLoaderById {
	return &WalletLoaderById{
		fetch:    config.Fetch,
		wait:     config.Wait,
		maxBatch: config.MaxBatch,
	}
}

// WalletLoaderById batches and caches requests
type WalletLoaderById struct {
	// this method provides the data for the loader
	fetch func(keys []persist.DBID) ([]coregen.Wallet, []error)

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// INTERNAL

	// lazily created cache
	cache map[persist.DBID]coregen.Wallet

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *walletLoaderByIdBatch

	// mutex to prevent races
	mu sync.Mutex
}

type walletLoaderByIdBatch struct {
	keys    []persist.DBID
	data    []coregen.Wallet
	error   []error
	closing bool
	done    chan struct{}
}

// Load a Wallet by key, batching and caching will be applied automatically
func (l *WalletLoaderById) Load(key persist.DBID) (coregen.Wallet, error) {
	return l.LoadThunk(key)()
}

// LoadThunk returns a function that when called will block waiting for a Wallet.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *WalletLoaderById) LoadThunk(key persist.DBID) func() (coregen.Wallet, error) {
	l.mu.Lock()
	if it, ok := l.cache[key]; ok {
		l.mu.Unlock()
		return func() (coregen.Wallet, error) {
			return it, nil
		}
	}
	if l.batch == nil {
		l.batch = &walletLoaderByIdBatch{done: make(chan struct{})}
	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return func() (coregen.Wallet, error) {
		<-batch.done

		var data coregen.Wallet
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
func (l *WalletLoaderById) LoadAll(keys []persist.DBID) ([]coregen.Wallet, []error) {
	results := make([]func() (coregen.Wallet, error), len(keys))

	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}

	wallets := make([]coregen.Wallet, len(keys))
	errors := make([]error, len(keys))
	for i, thunk := range results {
		wallets[i], errors[i] = thunk()
	}
	return wallets, errors
}

// LoadAllThunk returns a function that when called will block waiting for a Wallets.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *WalletLoaderById) LoadAllThunk(keys []persist.DBID) func() ([]coregen.Wallet, []error) {
	results := make([]func() (coregen.Wallet, error), len(keys))
	for i, key := range keys {
		results[i] = l.LoadThunk(key)
	}
	return func() ([]coregen.Wallet, []error) {
		wallets := make([]coregen.Wallet, len(keys))
		errors := make([]error, len(keys))
		for i, thunk := range results {
			wallets[i], errors[i] = thunk()
		}
		return wallets, errors
	}
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *WalletLoaderById) Prime(key persist.DBID, value coregen.Wallet) bool {
	l.mu.Lock()
	var found bool
	if _, found = l.cache[key]; !found {
		l.unsafeSet(key, value)
	}
	l.mu.Unlock()
	return !found
}

// Clear the value at key from the cache, if it exists
func (l *WalletLoaderById) Clear(key persist.DBID) {
	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()
}

func (l *WalletLoaderById) unsafeSet(key persist.DBID, value coregen.Wallet) {
	if l.cache == nil {
		l.cache = map[persist.DBID]coregen.Wallet{}
	}
	l.cache[key] = value
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *walletLoaderByIdBatch) keyIndex(l *WalletLoaderById, key persist.DBID) int {
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

func (b *walletLoaderByIdBatch) startTimer(l *WalletLoaderById) {
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

func (b *walletLoaderByIdBatch) end(l *WalletLoaderById) {
	b.data, b.error = l.fetch(b.keys)
	close(b.done)
}
