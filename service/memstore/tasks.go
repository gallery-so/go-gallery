package memstore

import (
	"context"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/sirupsen/logrus"
)

// update represents a key and value pair
type update struct {
	key string
	val []byte

	ttl time.Duration
}

// UpdateQueue is a queue of updates to be run
type UpdateQueue struct {
	mu *sync.Mutex
	wp *workerpool.WorkerPool

	cache Cache

	updates        chan update
	runningUpdates map[string]bool
}

// NewUpdateQueue creates a new UpdateQueue
func NewUpdateQueue(cache Cache) *UpdateQueue {
	queue := &UpdateQueue{
		mu:             &sync.Mutex{},
		wp:             workerpool.New(10),
		cache:          cache,
		updates:        make(chan update),
		runningUpdates: make(map[string]bool),
	}
	queue.start()
	return queue
}

// Start starts the update queue
func (uq *UpdateQueue) start() {
	go func() {
		for update := range uq.updates {
			uq.mu.Lock()
			if uq.runningUpdates[update.key] {
				uq.mu.Unlock()
				continue
			}
			uq.runningUpdates[update.key] = true
			uq.mu.Unlock()

			updateFunc := func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := uq.cache.Set(ctx, update.key, update.val, update.ttl)
				if err != nil {
					logrus.WithError(err).Error("memstore: failed to update key")
				}

				uq.mu.Lock()
				defer uq.mu.Unlock()
				delete(uq.runningUpdates, update.key)
			}
			if uq.wp.WaitingQueueSize() > 25 {
				uq.wp.SubmitWait(updateFunc)
			} else {
				uq.wp.Submit(updateFunc)
			}
		}
	}()
}

// Stop stops the update queue
func (uq *UpdateQueue) Stop() {
	uq.wp.StopWait()
}

// QueueUpdate queues an update to be run
func (uq *UpdateQueue) QueueUpdate(key string, value []byte, ttl time.Duration) {
	uq.updates <- update{key: key, val: value, ttl: ttl}
}
