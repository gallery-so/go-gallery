package tokenmanage

import (
	"context"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"

	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
)

type Manager struct {
	processRegistry *registry
	queue           *enqueue
	throttle        *throttle.Locker
	retryLimiter    *limiters.KeyRateLimiter
}

func New(ctx context.Context, taskClient *cloudtasks.Client) *Manager {
	cache := redis.NewCache(redis.TokenManageCache)
	m := &Manager{
		processRegistry: &registry{cache},
		queue:           &enqueue{taskClient},
		throttle:        throttle.NewThrottleLocker(cache, 30*time.Minute),
		// XXX retryLimiter:    limiters.NewKeyRateLimiter(ctx, cache, "retry", 10, 15*time.Minute),
		retryLimiter: limiters.NewKeyRateLimiter(ctx, cache, "retry", 3, 15*time.Minute),
	}
	return m
}

// Processing returns true if the token is currently being processed.
func (m Manager) Processing(ctx context.Context, tokenID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tokenID)
	return p
}

func (m Manager) StartProcessingRetry(ctx context.Context, tokenID persist.DBID) (error, func(err error) error) {
	return m.startProcessing(ctx, tokenID, true)
}

func (m Manager) StartProcessingNoRetry(ctx context.Context, tokenID persist.DBID) (error, func(err error) error) {
	return m.startProcessing(ctx, tokenID, false)
}

// startProcessing marks a token as processing. It returns a callback function that should be called when work on the token is done to mark
// it as finished, release the lock and close the keepalive routine. If withRetry is true, the callback will attempt to retry the token
// by re-enqueuing it if an error is passed to it.
func (m Manager) startProcessing(ctx context.Context, tokenID persist.DBID, withRetry bool) (error, func(err error) error) {
	err := m.throttle.Lock(ctx, lockKey(tokenID))
	if err != nil {
		return err, nil
	}

	stop := make(chan bool)
	done := make(chan bool)
	// XXX tick := time.NewTicker(10 * time.Second)
	tick := time.NewTicker(4 * time.Second)

	go func() {
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				fmt.Println("keeping alive", tokenID)
				m.processRegistry.keepAlive(ctx, tokenID)
			case <-stop:
				fmt.Println("finished", tokenID)
				close(done)
				return
			}
		}
	}()

	callback := func(err error) error {
		fmt.Println("in callback", tokenID)
		close(stop)
		<-done
		if withRetry {
			err = m.tryRetry(ctx, tokenID, err)
			if err != nil {
				panic(err)
			}
		}
		fmt.Println("unlocking", tokenID)
		err = m.throttle.Unlock(ctx, lockKey(tokenID))
		fmt.Println("done unlocking", tokenID)
		if err != nil {
			panic(err)
		}
		return nil
	}

	return nil, callback
}

// SubmitUser enqueues a user's tokens for processing.
func (m Manager) SubmitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	err := m.processRegistry.setEnqueuedMany(ctx, tokenIDs)
	if err != nil {
		panic(err)
		// Only log the error so the task is still enqueued
		logger.For(ctx).Errorf("failed to set enqueued tokens for user %s: %s", userID, err)
	}
	return m.queue.submitUser(ctx, userID, tokenIDs, chains)
}

func (m Manager) tryRetry(ctx context.Context, tokenID persist.DBID, err error) error {
	if err == nil {
		err := m.processRegistry.finish(ctx, tokenID)
		if err != nil {
			panic(err)
		}
		fmt.Println("DONE WITH TOKEN", tokenID)
		return nil
	}

	canRetry, _, err := m.retryLimiter.ForKey(ctx, tokenID.String())
	if err != nil {
		panic(err)
		return err
	}

	if !canRetry {
		fmt.Println("CAN'T RETRY", tokenID)
		err := m.processRegistry.finish(ctx, tokenID)
		if err != nil {
			panic(err)
		}
		return nil
	}

	err = m.processRegistry.setEnqueue(ctx, tokenID)
	if err != nil {
		panic(err)
		// Only log the error so the task can still be retried
		logger.For(ctx).Errorf("failed to set enqueued for token %s: %s", tokenID, err)
	}

	fmt.Println("putting back in queue", tokenID, "...")
	err = m.queue.submitTokenForRetry(ctx, tokenID)
	if err != nil {
		panic(err)
	}
	fmt.Println("REQUEUED", tokenID)
	return err
}

type registry struct{ c *redis.Cache }

func inflightKey(tokenID persist.DBID) string { return "inflight:" + tokenID.String() }
func lockKey(tokenID persist.DBID) string     { return "lock:" + tokenID.String() }

func (r registry) processing(ctx context.Context, tokenID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, inflightKey(tokenID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tokenID persist.DBID) error {
	fmt.Println("deleting", tokenID, inflightKey(tokenID))
	return r.c.Delete(ctx, inflightKey(tokenID))
}

func (r registry) setEnqueue(ctx context.Context, tokenID persist.DBID) error {
	err := r.c.Set(ctx, inflightKey(tokenID), []byte("enqueued"), 0)
	return err
}

func (r registry) setEnqueuedMany(ctx context.Context, tokenIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		keyValues[inflightKey(tokenID)] = []byte("enqueued")
	}
	return r.c.MSet(ctx, keyValues)
}

func (r registry) keepAlive(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Set(ctx, inflightKey(tokenID), []byte("processing"), time.Minute)
}

type enqueue struct{ taskClient *cloudtasks.Client }

func (e enqueue) submitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	message := task.TokenProcessingUserMessage{UserID: userID, TokenIDs: tokenIDs, Chains: chains}
	return task.CreateTaskForTokenProcessing(ctx, message, e.taskClient)
}

func (e enqueue) submitTokenForRetry(ctx context.Context, tokenID persist.DBID) error {
	message := task.TokenProcessingTokenInstanceMessage{TokenDBID: tokenID}
	return task.CreateTaskForTokenInstanceTokenProcessing(ctx, message, e.taskClient)
}
