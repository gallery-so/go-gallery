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
	cache           *redis.Cache
	processRegistry *registry
	taskClient      *cloudtasks.Client
	throttle        *throttle.Locker
	// delayer sets the linear delay for retrying tokens up to MaxRetries
	delayer *limiters.KeyRateLimiter
	// maxRetryF is a function that defines the maximum number of times a token can be reenqueued before it is not retried again
	maxRetryF func(id persist.DBID) int
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

func NewWithRetries(ctx context.Context, taskClient *cloudtasks.Client, maxRetryF func(persist.DBID) int) *Manager {
	m := New(ctx, taskClient)
	m.maxRetryF = maxRetryF
	m.delayer = limiters.NewKeyRateLimiter(ctx, m.cache, "retry", 2, 1*time.Minute)
	return m
}

// Processing returns true if the token is processing or enqueued.
func (m Manager) Processing(ctx context.Context, tokenDefinitionID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tokenDefinitionID)
	return p
}

// StartProcessing marks a token as processing. It returns a callback that must be called when work on the token is finished in order to mark
// it as finished. If withRetry is true, the callback will attempt to reenqueue the token if an error is passed. attemps is ignored when MaxRetries
// is set to the default value of 0.
func (m Manager) StartProcessing(ctx context.Context, tokenDefinitionID persist.DBID, attempts int) (func(err error) error, error) {
	err := m.throttle.Lock(ctx, "lock:"+tokenDefinitionID.String())
	if err != nil {
		return nil, err
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		defer tick.Stop()
		m.processRegistry.keepAlive(ctx, tokenDefinitionID)
		for {
			select {
			case <-tick.C:
				m.processRegistry.keepAlive(ctx, tokenDefinitionID)
			case <-stop:
				close(done)
				return
			}
		}
	}()

	callback := func(err error) error {
		close(stop)
		<-done
		m.tryRetry(ctx, tokenDefinitionID, err, attempts)
		m.throttle.Unlock(ctx, "lock:"+tokenDefinitionID.String())
		return nil
	}

	return callback, err
}

// SubmitBatch enqueues tokens for processing.
func (m Manager) SubmitBatch(ctx context.Context, tokenDefinitionIDs []persist.DBID) error {
	m.processRegistry.setManyEnqueue(ctx, tokenDefinitionIDs)
	message := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tokenDefinitionIDs}
	logger.For(ctx).WithField("batchID", message.BatchID).Infof("enqueued batch: %s", message.BatchID)
	return task.CreateTaskForTokenProcessing(ctx, message, m.taskClient)
}

func (m Manager) tryRetry(ctx context.Context, tokenDefinitionID persist.DBID, err error, attempts int) error {
	if err == nil || m.maxRetryF == nil || attempts >= m.maxRetryF(tokenDefinitionID) {
		m.processRegistry.finish(ctx, tokenDefinitionID)
		return nil
	}

	_, delay, err := m.delayer.ForKey(ctx, tokenDefinitionID.String())
	if err != nil {
		m.processRegistry.finish(ctx, tokenDefinitionID)
		return err
	}

	m.processRegistry.setEnqueue(ctx, tokenDefinitionID)
	message := task.TokenProcessingTokenMessage{TokenDefinitionID: tokenDefinitionID, Attempts: attempts + 1}
	return task.CreateTaskForTokenTokenProcessing(ctx, message, m.taskClient, delay)
}

type registry struct{ c *redis.Cache }

func (r registry) processing(ctx context.Context, tokenDefinitionID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, prefixKey(tokenDefinitionID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tokenDefinitionID persist.DBID) error {
	return r.c.Delete(ctx, prefixKey(tokenDefinitionID))
}

func (r registry) setEnqueue(ctx context.Context, tokenDefinitionID persist.DBID) error {
	return r.setManyEnqueue(ctx, []persist.DBID{tokenDefinitionID})
}

func (r registry) setManyEnqueue(ctx context.Context, tokenDefinitionIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tokenDefinitionIDs))
	for _, id := range tokenDefinitionIDs {
		keyValues[prefixKey(id)] = []byte("enqueued")
	}
	return r.c.MSetWithTTL(ctx, keyValues, time.Hour)
}

func (r registry) keepAlive(ctx context.Context, tokenDefinitionID persist.DBID) error {
	return r.c.Set(ctx, prefixKey(tokenDefinitionID), []byte("processing"), time.Minute)
}

func prefixKey(id persist.DBID) string { return "inflight:" + id.String() }
