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

type MaxRetryF func(tID persist.TokenIdentifiers) int

type Manager struct {
	cache           *redis.Cache
	processRegistry *registry
	taskClient      *cloudtasks.Client
	throttle        *throttle.Locker
	// delayer sets the linear delay for retrying tokens up to MaxRetries
	delayer *limiters.KeyRateLimiter
	// maxRetryF is a function that defines the maximum number of times a token can be reenqueued before it is not retried again
	maxRetryF MaxRetryF
}

func New(ctx context.Context, taskClient *cloudtasks.Client) *Manager {
	cache := redis.NewCache(redis.TokenManageCache)
	return &Manager{
		cache:           cache,
		processRegistry: &registry{cache},
		taskClient:      taskClient,
		throttle:        throttle.NewThrottleLocker(cache, 30*time.Minute),
	}
}

func NewWithRetries(ctx context.Context, taskClient *cloudtasks.Client, maxRetryF MaxRetryF) *Manager {
	m := New(ctx, taskClient)
	m.maxRetryF = maxRetryF
	m.delayer = limiters.NewKeyRateLimiter(ctx, m.cache, "retry", 2, 1*time.Minute)
	return m
}

// Processing returns true if the token is processing or enqueued.
func (m Manager) Processing(ctx context.Context, tDefID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tDefID)
	return p
}

// StartProcessing marks a token as processing. It returns a callback that must be called when work on the token is finished in order to mark
// it as finished. If withRetry is true, the callback will attempt to reenqueue the token if an error is passed. attemps is ignored when MaxRetries
// is set to the default value of 0.
func (m Manager) StartProcessing(ctx context.Context, tDefID persist.DBID, tID persist.TokenIdentifiers, attempts int) (func(err error) error, error) {
	err := m.throttle.Lock(ctx, "lock:"+tDefID.String())
	if err != nil {
		return nil, err
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		defer tick.Stop()
		m.processRegistry.keepAlive(ctx, tDefID)
		for {
			select {
			case <-tick.C:
				m.processRegistry.keepAlive(ctx, tDefID)
			case <-stop:
				close(done)
				return
			}
		}
	}()

	callback := func(err error) error {
		close(stop)
		<-done
		m.tryRetry(ctx, tDefID, tID, err, attempts)
		m.throttle.Unlock(ctx, "lock:"+tDefID.String())
		return nil
	}

	return callback, err
}

// SubmitBatch enqueues tokens for processing.
func (m Manager) SubmitBatch(ctx context.Context, tDefIDs []persist.DBID) error {
	if len(tDefIDs) == 0 {
		return nil
	}
	m.processRegistry.setManyEnqueue(ctx, tDefIDs)
	message := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tDefIDs}
	logger.For(ctx).WithField("batchID", message.BatchID).Infof("enqueued batch: %s (size=%d)", message.BatchID, len(tDefIDs))
	return task.CreateTaskForTokenProcessing(ctx, message, m.taskClient)
}

func (m Manager) tryRetry(ctx context.Context, tDefID persist.DBID, tID persist.TokenIdentifiers, err error, attempts int) error {
	if err == nil || m.maxRetryF == nil || attempts >= m.maxRetryF(tID) {
		m.processRegistry.finish(ctx, tDefID)
		return nil
	}

	_, delay, err := m.delayer.ForKey(ctx, tDefID.String())
	if err != nil {
		m.processRegistry.finish(ctx, tDefID)
		return err
	}

	m.processRegistry.setEnqueue(ctx, tDefID)
	message := task.TokenProcessingTokenMessage{TokenDefinitionID: tDefID, Attempts: attempts + 1}
	return task.CreateTaskForTokenTokenProcessing(ctx, message, m.taskClient, delay)
}

type registry struct{ c *redis.Cache }

func (r registry) processing(ctx context.Context, tDefID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, prefixKey(tDefID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tDefID persist.DBID) error {
	return r.c.Delete(ctx, prefixKey(tDefID))
}

func (r registry) setEnqueue(ctx context.Context, tDefID persist.DBID) error {
	return r.setManyEnqueue(ctx, []persist.DBID{tDefID})
}

func (r registry) setManyEnqueue(ctx context.Context, tDefIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tDefIDs))
	for _, id := range tDefIDs {
		keyValues[prefixKey(id)] = []byte("enqueued")
	}
	return r.c.MSetWithTTL(ctx, keyValues, time.Hour)
}

func (r registry) keepAlive(ctx context.Context, tDefID persist.DBID) error {
	return r.c.Set(ctx, prefixKey(tDefID), []byte("processing"), time.Minute)
}

func prefixKey(id persist.DBID) string { return "inflight:" + id.String() }
