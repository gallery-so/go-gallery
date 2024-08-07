package limiters

import (
	"context"
	"fmt"
	"github.com/bsm/redislock"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/sirupsen/logrus"
	"time"

	"github.com/benny-conn/limiters"
)

type KeyRateLimiter struct {
	cache        *redis.Cache
	name         string
	capacity     int64
	refillRate   time.Duration
	timeToRefill time.Duration
	clock        *limiters.SystemClock
	logger       limiters.Logger
	lock         *distributedLock
}

// NewKeyRateLimiter creates a new rate limiter that will limit to `amount` operations `every` duration.
// The name will be used to uniquely identify this limiter in the specified redis cache. Consequently,
// two different limiters in the same cache should NOT have the same name. It is safe to share a single
// KeyRateLimiter object among multiple consumers to share a limit.
func NewKeyRateLimiter(ctx context.Context, cache *redis.Cache, name string, amount int64, every time.Duration) *KeyRateLimiter {
	clock := limiters.NewSystemClock()

	// Refill rate is per token, so we have to divide to get the correct rate
	refillRate := time.Duration(float64(every) / float64(amount))

	// Assuming no tokens are taken, this is how long it will take to completely refill the bucket.
	// This is useful for TTLs, because if this much time has passed and no tokens have been taken,
	// the bucket is full and we no longer need to track its state
	timeToRefill := every

	limiter := &KeyRateLimiter{
		cache:        cache,
		name:         name,
		capacity:     amount,
		refillRate:   refillRate,
		timeToRefill: timeToRefill,
		clock:        clock,
		logger:       newLogAdapter(ctx),
		lock:         newDistributedLock(cache, name),
	}

	return limiter
}

// ForKey will check if the given key has exceeded the rate limit for this named limiter
func (i *KeyRateLimiter) ForKey(ctx context.Context, key string) (bool, time.Duration, error) {
	key = i.cache.Prefix() + ":" + i.name + ":" + key
	backend := limiters.NewTokenBucketRedis(i.cache.Client(), key, i.timeToRefill, false)
	bucket := limiters.NewTokenBucket(i.capacity, i.refillRate, i.lock, backend, i.clock, i.logger)
	w, err := bucket.Limit(ctx)
	if err == limiters.ErrLimitExhausted {
		return false, w, nil
	} else if err == redislock.ErrNotObtained {
	} else if err != nil {
		// The limiter failed. This error should be logged and examined.
		rateErr := fmt.Errorf("rate limiting err: %s", err)
		logger.For(ctx).Warn(rateErr)
		return false, 0, rateErr
	}

	return true, 0, nil
}

// Reset resets the limit of a key
func (i *KeyRateLimiter) Reset(ctx context.Context, key string) error {
	err := i.lock.Lock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := i.lock.Unlock(ctx); err != nil {
			i.logger.Log(err)
		}
	}()
	// This is a somewhat hacky way to do this. The backing bucket isn't exported on the limiter,
	// so we create another instance of the bucket in order to reset its backing state in redis.
	bucketPrefix := i.cache.Prefix() + ":" + i.name + ":" + key
	tokenBucket := limiters.NewTokenBucketRedis(i.cache.Client(), bucketPrefix, i.timeToRefill, false)
	return tokenBucket.SetState(ctx, limiters.TokenBucketState{})
}

// Name returns the name of this limiter
func (i *KeyRateLimiter) Name() string {
	return i.name
}

type logAdapter struct {
	entry *logrus.Entry
}

func newLogAdapter(ctx context.Context) logAdapter {
	return logAdapter{
		entry: logger.For(ctx),
	}
}

func (l logAdapter) Log(v ...interface{}) {
	l.entry.Info(v...)
}

type distributedLock struct {
	client  *redislock.Client
	lock    *redislock.Lock
	key     string
	ttl     time.Duration
	options *redislock.Options
}

func newDistributedLock(cache *redis.Cache, limiterName string) *distributedLock {
	options := &redislock.Options{
		RetryStrategy: redislock.LimitRetry(redislock.LinearBackoff(time.Millisecond*500), 10),
	}

	// Unlocking should be handled by the limiter, but if it's not, we'll release the lock
	// after one second.
	ttl := time.Second

	return &distributedLock{
		client:  redis.NewLockClient(cache),
		key:     limiterName + ":lock",
		ttl:     ttl,
		options: options,
	}
}

func (l *distributedLock) Lock(ctx context.Context) error {
	lock, err := l.client.Obtain(ctx, l.key, l.ttl, l.options)
	if err != nil {
		return err
	}
	l.lock = lock
	return nil
}

func (l *distributedLock) Unlock(ctx context.Context) error {
	return l.lock.Release(ctx)
}
