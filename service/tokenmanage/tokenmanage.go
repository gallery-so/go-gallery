package tokenmanage

import (
	"context"
	"fmt"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"

	"github.com/mikeydub/go-gallery/service/limiters"
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
	// MaxRetries is the maximum number of times a token can be reenqueued before it is not retried again
	MaxRetries int
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

func NewWithRetries(ctx context.Context, taskClient *cloudtasks.Client, maxRetries int) *Manager {
	m := New(ctx, taskClient)
	m.MaxRetries = maxRetries
	m.delayer = limiters.NewKeyRateLimiter(ctx, m.cache, "retry", 2, 1*time.Minute)
	return m
}

// Processing returns true if the token is currently being processed.
func (m Manager) Processing(ctx context.Context, tokenID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tokenID)
	return p
}

// StartProcessing marks a token as processing. It returns a callback that should be called when work on the token is done to mark
// it as finished. If withRetry is true, the callback will attempt to reenqueue the token if an error is passed. The tries arg is ignored
// when MaxRetries is set to the default value of 0.
func (m Manager) StartProcessing(ctx context.Context, tokenID persist.DBID, tries int) (error, func(err error) error) {
	err := m.throttle.Lock(ctx, lockKey(tokenID))
	if err != nil {
		return err, nil
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				m.processRegistry.keepAlive(ctx, tokenID)
			case <-stop:
				close(done)
				return
			}
		}
	}()

	callback := func(err error) error {
		close(stop)
		<-done
		m.tryRetry(ctx, tokenID, err, tries)
		m.throttle.Unlock(ctx, lockKey(tokenID))
		return nil
	}

	return nil, callback
}

// SubmitUser enqueues a user's tokens for processing.
func (m Manager) SubmitUser(ctx context.Context, userID persist.DBID, tokenIDs []persist.DBID, chains []persist.Chain) error {
	m.processRegistry.setEnqueuedMany(ctx, tokenIDs)
	message := task.TokenProcessingUserMessage{UserID: userID, TokenIDs: tokenIDs, Chains: chains}
	return task.CreateTaskForTokenProcessing(ctx, message, m.taskClient)
}

func (m Manager) tryRetry(ctx context.Context, tokenID persist.DBID, err error, tries int) error {
	if err == nil || m.MaxRetries <= 0 || tries >= m.MaxRetries {
		m.processRegistry.finish(ctx, tokenID)
		fmt.Println("DONE WITH TOKEN", tokenID, "err", err, "tries", tries, "max", m.MaxRetries)
		return nil
	}

	_, delay, err := m.delayer.ForKey(ctx, tokenID.String())
	if err != nil {
		m.processRegistry.finish(ctx, tokenID)
		return err
	}

	m.processRegistry.setEnqueue(ctx, tokenID)
	message := task.TokenProcessingTokenInstanceMessage{TokenDBID: tokenID, Attempts: tries + 1}
	fmt.Println("reenqueued", tokenID, "atttempt", message.Attempts, "delayed for", delay)
	return task.CreateTaskForTokenInstanceTokenProcessing(ctx, message, m.taskClient, delay)
}

type registry struct{ c *redis.Cache }

func inflightKey(tokenID persist.DBID) string { return "inflight:" + tokenID.String() }
func lockKey(tokenID persist.DBID) string     { return "lock:" + tokenID.String() }

func (r registry) processing(ctx context.Context, tokenID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, inflightKey(tokenID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tokenID persist.DBID) error {
	return r.c.Delete(ctx, inflightKey(tokenID))
}

func (r registry) setEnqueue(ctx context.Context, tokenID persist.DBID) error {
	// Set a long TTL on the off-chance that a worker never picks up the message
	err := r.c.Set(ctx, inflightKey(tokenID), []byte("enqueued"), time.Hour*3)
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
