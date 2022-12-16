package dataloader

import (
	"context"
	"sync"
	"time"
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

func withContext(ctx context.Context) func(interface {
	setContext(ctx context.Context)
	setWait(time.Duration)
	setMaxBatch(int)
	setDisableCaching(bool)
	setPublishResults(bool)
}) {
	return func(s interface {
		setContext(ctx context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}) {
		s.setContext(ctx)
	}
}

func withMaxWait(wait time.Duration) func(interface {
	setContext(ctx context.Context)
	setWait(time.Duration)
	setMaxBatch(int)
	setDisableCaching(bool)
	setPublishResults(bool)
}) {
	return func(s interface {
		setContext(ctx context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}) {
		s.setWait(wait)
	}
}

func withMaxBatch(batchSize int) func(interface {
	setContext(ctx context.Context)
	setWait(time.Duration)
	setMaxBatch(int)
	setDisableCaching(bool)
	setPublishResults(bool)
}) {
	return func(s interface {
		setContext(ctx context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}) {
		s.setMaxBatch(batchSize)
	}
}

func withDisableCaching(disable bool) func(interface {
	setContext(ctx context.Context)
	setWait(time.Duration)
	setMaxBatch(int)
	setDisableCaching(bool)
	setPublishResults(bool)
}) {
	return func(s interface {
		setContext(ctx context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}) {
		s.setDisableCaching(disable)
	}
}

func withPublishResults(publish bool) func(interface {
	setContext(ctx context.Context)
	setWait(time.Duration)
	setMaxBatch(int)
	setDisableCaching(bool)
	setPublishResults(bool)
}) {
	return func(s interface {
		setContext(ctx context.Context)
		setWait(time.Duration)
		setMaxBatch(int)
		setDisableCaching(bool)
		setPublishResults(bool)
	}) {
		s.setPublishResults(publish)
	}
}

// fillErrors fills a slice of errors with the specified error. Useful for batched lookups where
// a single top-level error may need to be returned for each request in the batch.
func fillErrors(errors []error, err error) {
	for i := 0; i < len(errors); i++ {
		errors[i] = err
	}
}
