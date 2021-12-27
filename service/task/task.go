package task

import (
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// task represents a key and value pair
type task struct {
	key        string
	updateFunc func()
}

// Queue is a queue of updates to be run
type Queue struct {
	wg             *sync.WaitGroup
	poolMu         *sync.Mutex
	lastFinishedMu *sync.Mutex

	updates           chan task
	pools             map[string]*workerpool.WorkerPool
	lastFinishedPools map[string]time.Time

	poolFinished chan string
}

// NewQueue creates a new UpdateQueue
func NewQueue() *Queue {
	queue := &Queue{
		wg:             &sync.WaitGroup{},
		poolMu:         &sync.Mutex{},
		lastFinishedMu: &sync.Mutex{},

		updates:           make(chan task),
		pools:             map[string]*workerpool.WorkerPool{},
		lastFinishedPools: map[string]time.Time{},
		poolFinished:      make(chan string),
	}
	queue.start()
	return queue
}

// Start starts the update queue
func (uq *Queue) start() {
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
			pool.Submit(update.updateFunc)
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
func (uq *Queue) Stop() {
	close(uq.updates)
	uq.wg.Wait()
	uq.poolMu.Lock()
	defer uq.poolMu.Unlock()
	for _, pool := range uq.pools {
		pool.StopWait()
	}
}

// QueueTask queues an update to be run
func (uq *Queue) QueueTask(key string, updateFunc func()) {
	uq.updates <- task{key: key, updateFunc: updateFunc}
}
