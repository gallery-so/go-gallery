package tokenmanage

import (
	"context"
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
		retryLimiter:    limiters.NewKeyRateLimiter(ctx, cache, "tokenmanage:retry", 10, 15*time.Minute),
	}
	return m
}

// Processing returns true if the token is currently being processed.
func (m Manager) Processing(ctx context.Context, tokenID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tokenID)
	return p
}

// StartProcessing marks a token as processing. It returns a callback function that should be called when work on the token is done to mark
// it as finished, release the lock and close the keepalive routine.
func (m Manager) StartProcessing(ctx context.Context, tokenID persist.DBID, token persist.TokenIdentifiers) (error, func(err error) error) {
	err := m.throttle.Lock(ctx, tokenID.String())
	if err != nil {
		return err, nil
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		for {
			select {
			case <-tick.C:
				m.processRegistry.keepAlive(ctx, tokenID)
			case <-stop:
				done <- true
				return
			}
		}
	}()

	callback := func(err error) error {
		stop <- true
		done <- true
		m.tryRetry(ctx, tokenID, token, err)
		m.throttle.Unlock(ctx, tokenID.String())
		return nil
	}

	return nil, callback
}

// SubmitUser enqueues a user's tokens for processing.
func (m Manager) SubmitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	err := m.processRegistry.setEnqueuedM(ctx, tokenIDs)
	if err != nil {
		// Only log the error so the task is still enqueued
		logger.For(ctx).Errorf("failed to set enqueued tokens for user %s: %s", userID, err)
	}
	return m.queue.submitUser(ctx, userID, tokenIDs, chains)
}

func (m Manager) tryRetry(ctx context.Context, tokenID persist.DBID, token persist.TokenIdentifiers, err error) error {
	if err == nil {
		m.processRegistry.finish(ctx, tokenID)
		return nil
	}

	canRetry, _, err := m.retryLimiter.ForKey(ctx, tokenID.String())
	if err != nil {
		return err
	}

	if !canRetry {
		m.processRegistry.finish(ctx, tokenID)
		return nil
	}

	err = m.processRegistry.setEnqueue(ctx, tokenID)
	if err != nil {
		// Only log the error so the task can still be retried
		logger.For(ctx).Errorf("failed to set enqueued for token %s: %s", tokenID, err)
	}

	return m.queue.submitToken(ctx, token)
}

type registry struct{ c *redis.Cache }

func key(tokenID persist.DBID) string { return "inflight:" + tokenID.String() }

func (r registry) processing(ctx context.Context, tokenID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, key(tokenID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Delete(ctx, key(tokenID))
}

func (r registry) setEnqueue(ctx context.Context, tokenID persist.DBID) error {
	_, err := r.c.SetNX(ctx, key(tokenID), []byte("enqueued"), 0)
	return err
}

func (r registry) setEnqueuedM(ctx context.Context, tokenIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		keyValues[key(tokenID)] = []byte("enqueued")
	}
	return r.c.MSet(ctx, keyValues)
}

func (r registry) keepAlive(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Set(ctx, key(tokenID), []byte("processing"), time.Minute)
}

type enqueue struct{ taskClient *cloudtasks.Client }

func (e enqueue) submitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	message := task.TokenProcessingUserMessage{UserID: userID, TokenIDs: tokenIDs, Chains: chains}
	return task.CreateTaskForTokenProcessing(ctx, message, e.taskClient)
}

func (e enqueue) submitToken(ctx context.Context, token persist.TokenIdentifiers) error {
	message := task.TokenProcessingTokenMessage{TokenID: token.TokenID, ContractAddress: token.ContractAddress, Chain: token.Chain}
	return task.CreateTaskForTokenTokenProcessing(ctx, message, e.taskClient)
}
