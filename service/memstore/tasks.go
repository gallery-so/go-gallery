package memstore

import (
	"context"
	"sync"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"

	"github.com/gammazero/workerpool"
)

// update represents a key and value pair
type update struct {
	key string
	val []byte

	ttl  time.Duration
	sync bool
}

// UpdateQueue is a queue of updates to be run
type UpdateQueue struct {
	wg             *sync.WaitGroup
	poolMu         *sync.Mutex
	lastFinishedMu *sync.Mutex

	cache Cache

	updates           chan update
	pools             map[string]*workerpool.WorkerPool
	lastFinishedPools map[string]time.Time

	poolFinished chan string
}

// NewUpdateQueue creates a new UpdateQueue
func NewUpdateQueue(cache Cache) *UpdateQueue {
	queue := &UpdateQueue{
		wg:                &sync.WaitGroup{},
		poolMu:            &sync.Mutex{},
		lastFinishedMu:    &sync.Mutex{},
		cache:             cache,
		updates:           make(chan update),
		pools:             map[string]*workerpool.WorkerPool{},
		lastFinishedPools: map[string]time.Time{},
		poolFinished:      make(chan string),
	}
	queue.start()
	return queue
}

// Start starts the update queue
func (uq *UpdateQueue) start() {
	uq.wg.Add(1)
	go func() {
		pools := map[string]*workerpool.WorkerPool{}
		defer uq.wg.Done()
		for update := range uq.updates {
			pool, ok := pools[update.key]
			if !ok {
				pool = workerpool.New(1)
				pools[update.key] = pool
			}
			pool.Submit(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				err := uq.cache.Set(ctx, update.key, update.val, update.ttl)
				if err != nil {
					logger.For(ctx).WithError(err).Error("memstore: failed to update key")
				}
				uq.poolFinished <- update.key
			})
		}
	}()
	go func() {
		for key := range uq.poolFinished {
			uq.lastFinishedMu.Lock()
			uq.lastFinishedPools[key] = time.Now()
			uq.lastFinishedMu.Unlock()
		}
	}()
	go func() {
		for {
			uq.lastFinishedMu.Lock()
			for key, lastFinished := range uq.lastFinishedPools {
				if time.Since(lastFinished) > time.Minute*10 {
					delete(uq.lastFinishedPools, key)
					uq.poolMu.Lock()
					pool, ok := uq.pools[key]
					if ok {
						pool.StopWait()
						delete(uq.pools, key)
					}
					uq.poolMu.Unlock()
				}
			}
			uq.lastFinishedMu.Unlock()
			time.Sleep(time.Second * 10)
		}
	}()
}

// Stop stops the update queue
func (uq *UpdateQueue) Stop() {
	close(uq.updates)
	uq.wg.Wait()
	uq.poolMu.Lock()
	defer uq.poolMu.Unlock()
	for _, pool := range uq.pools {
		pool.StopWait()
	}
}

// QueueUpdate queues an update to be run
func (uq *UpdateQueue) QueueUpdate(key string, value []byte, ttl time.Duration) {
	uq.updates <- update{key: key, val: value, ttl: ttl}
}
