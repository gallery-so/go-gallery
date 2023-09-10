package tokenmanage

import (
	"context"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"

	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
)

type Manager struct {
	retryLimiter    *limiters.KeyRateLimiter
	processRegistry *registry
	taskEnqueuer    *enqueue
	processThrottle *throttle.Locker
}

func New() *Manager {
	panic("not implemented")
}

func (m Manager) StartProcessing(ctx context.Context, tokenID persist.DBID, token persist.TokenIdentifiers) (error, func(err error) error) {
	err := m.processThrottle.Lock(ctx, tokenID.String())
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
		m.processThrottle.Unlock(ctx, tokenID.String())
		return nil
	}

	return nil, callback
}

func (m Manager) SubmitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	err := m.processRegistry.enqueueBatch(ctx, tokenIDs)
	if err != nil {
		return err
	}
	return m.taskEnqueuer.submitUser(ctx, userID, tokenIDs, chains)
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

	err = m.processRegistry.enqueue(ctx, tokenID)
	if err != nil {
		return err
	}

	return m.taskEnqueuer.submitToken(ctx, token)
}

type registry struct{ c *redis.Cache }

func (r registry) finish(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Delete(ctx, "inflight:"+tokenID.String())
}

func (r registry) enqueue(ctx context.Context, tokenID persist.DBID) error {
	_, err := r.c.SetNX(ctx, "inflight:"+tokenID.String(), []byte("enqueued"), 0)
	return err
}

func (r registry) enqueueBatch(ctx context.Context, tokenIDs []persist.DBID) error {
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
