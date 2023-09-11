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
	retryLimiter    *limiters.KeyRateLimiter
	processRegistry *registry
	queue           *enqueue
	throttle        *throttle.Locker
}

func New(ctx context.Context, taskClient *cloudtasks.Client) *Manager {
	cache := redis.NewCache(redis.TokenManageCache)
	m := &Manager{
		retryLimiter:    limiters.NewKeyRateLimiter(ctx, cache, "tokenmanage:retry", 10, 15*time.Minute),
		processRegistry: &registry{cache},
		queue:           &enqueue{taskClient},
		throttle:        throttle.NewThrottleLocker(cache, 30*time.Minute),
	}
	return m
}

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

func (r registry) finish(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Delete(ctx, "inflight:"+tokenID.String())
}

func (r registry) setEnqueue(ctx context.Context, tokenID persist.DBID) error {
	_, err := r.c.SetNX(ctx, "inflight:"+tokenID.String(), []byte("enqueued"), 0)
	return err
}

func (r registry) setEnqueuedM(ctx context.Context, tokenIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		keyValues["inflight:"+tokenID.String()] = []byte("enqueued")
	}
	return r.c.MSet(ctx, keyValues)
}

func (r registry) keepAlive(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Set(ctx, "inflight:"+tokenID.String(), []byte("processing"), time.Minute)
}

type enqueue struct{ taskClient *cloudtasks.Client }

func (e enqueue) submitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	return task.CreateTaskForTokenProcessing(ctx, task.TokenProcessingUserMessage{
		UserID:   userID,
		TokenIDs: tokenIDs,
		Chains:   chains,
	}, e.taskClient)
}

func (e enqueue) submitToken(ctx context.Context, token persist.TokenIdentifiers) error {
	return task.CreateTaskForTokenTokenProcessing(ctx, task.TokenProcessingTokenMessage{
		TokenID:         token.TokenID,
		ContractAddress: token.ContractAddress,
		Chain:           token.Chain,
	}, e.taskClient)
}
