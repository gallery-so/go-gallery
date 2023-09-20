package dataloader

import (
	"context"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/tracing"
)

type settings struct {
	ctx                  context.Context
	waitTime             time.Duration
	disableCaching       bool
	publishResults       bool
	preFetchHook         func(context.Context, string) context.Context
	postFetchHook        func(context.Context, string)
	maxBatchOne          int
	maxBatchMany         int
	subscriptionRegistry *[]interface{}
	mutexRegistry        *[]*sync.Mutex
}

func (s settings) getContext() context.Context {
	return s.ctx
}

func (s settings) getWait() time.Duration {
	return s.waitTime
}

func (s settings) getDisableCaching() bool {
	return s.disableCaching
}

func (s settings) getPublishResults() bool {
	return s.publishResults
}

func (s settings) getPreFetchHook() func(context.Context, string) context.Context {
	return s.preFetchHook
}

func (s settings) getPostFetchHook() func(context.Context, string) {
	return s.postFetchHook
}

func (s settings) getSubscriptionRegistry() *[]interface{} {
	return s.subscriptionRegistry
}

func (s settings) getMutexRegistry() *[]*sync.Mutex {
	return s.mutexRegistry
}

func (s settings) getMaxBatchOne() int {
	return s.maxBatchOne
}

func (s settings) getMaxBatchMany() int {
	return s.maxBatchMany
}

func defaultSettings(ctx context.Context, disableCaching bool, subscriptionRegistry *[]any, mutexRegistry *[]*sync.Mutex) settings {
	return settings{
		ctx:                  ctx,
		maxBatchOne:          100,
		maxBatchMany:         10,
		waitTime:             2 * time.Millisecond,
		disableCaching:       disableCaching,
		publishResults:       true,
		preFetchHook:         tracing.DataloaderPreFetchHook,
		postFetchHook:        tracing.DataloaderPostFetchHook,
		subscriptionRegistry: subscriptionRegistry,
		mutexRegistry:        mutexRegistry,
	}
}

func defaultSettingsPlusOpts(ctx context.Context, disableCaching bool, subscriptionRegistry *[]any, mutexRegistry *[]*sync.Mutex, opts ...func(*settings)) settings {
	s := defaultSettings(ctx, disableCaching, subscriptionRegistry, mutexRegistry)
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

func withMaxBatchOne(batchSize int) func(*settings) {
	return func(s *settings) {
		s.maxBatchOne = batchSize
	}
}

func withMaxWait(t time.Duration) func(*settings) {
	return func(s *settings) {
		s.waitTime = t
	}
}

func withContext(ctx context.Context) func(*settings) {
	return func(s *settings) {
		s.ctx = ctx
	}
}

func withMaxBatchMany(batchSize int) func(*settings) {
	return func(s *settings) {
		s.maxBatchMany = batchSize
	}
}

func withDisableCaching(disable bool) func(*settings) {
	return func(s *settings) {
		s.disableCaching = disable
	}
}

func withPublishResults(publish bool) func(*settings) {
	return func(s *settings) {
		s.publishResults = publish
	}
}

// fillErrors fills a slice of errors with the specified error. Useful for batched lookups where
// a single top-level error may need to be returned for each request in the batch.
func fillErrors(errors []error, err error) {
	for i := 0; i < len(errors); i++ {
		errors[i] = err
	}
}

func emptyResultsWithError[T any](length int, err error) ([]T, []error) {
	errors := make([]error, length)
	fillErrors(errors, err)
	return make([]T, length), errors
}

// fillUnnestedJoinResults is a helper for cases where we're looking up a slice of keys by their IDs via
// inner joining on an unnested list of keys. For example:
//
// select u.* from unnest(@user_ids::text[]) ids(id) join users u on u.id = ids.id
//
// Given the list of keys, the results from the query, and a function to extract a key from a result,
// fillUnnestedJoinResults will figure out which keys were returned in the results and use the supplied
// onNotFound function to fill in result entries for the keys, returning a set of results with the same
// order and length as the supplied keys.
func fillUnnestedJoinResults[TKey comparable, TResult any](keys []TKey, results []TResult, keyFunc func(TResult) TKey, onNotFound func(TKey) (TResult, error)) ([]TResult, []error) {
	output := make([]TResult, len(keys))
	errors := make([]error, len(keys))

	resultIdx := 0
	for i, key := range keys {
		if resultIdx < len(results) && keyFunc(results[resultIdx]) == key {
			output[i], errors[i] = results[resultIdx], nil
			resultIdx++
			continue
		}

		output[i], errors[i] = onNotFound(key)
	}

	return output, errors
}
