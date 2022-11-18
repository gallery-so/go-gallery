package middleware

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/benny-conn/limiters"
	"github.com/go-redis/redis/v8"
	gredis "github.com/mikeydub/go-gallery/service/memstore/redis"
)

// KeyRateLimiter .
type KeyRateLimiter struct {
	rateDuration time.Duration
	rateAmount   int64
	reg          *limiters.Registry
	red          *redis.Client
	clock        *limiters.SystemClock
	logger       *limiters.StdLogger
	lock         *gredis.GlobalLock
}

// NewKeyRateLimiter .
func NewKeyRateLimiter(rateAmount int64, every time.Duration, red *redis.Client) *KeyRateLimiter {

	i := &KeyRateLimiter{
		rateDuration: every,
		rateAmount:   rateAmount,
		reg:          limiters.NewRegistry(),
		clock:        limiters.NewSystemClock(),
		logger:       limiters.NewStdLogger(),
		red:          red,
		lock:         gredis.NewGlobalLock(gredis.NewLockClient(gredis.EmailRateLimiterDB), every*time.Duration(rateAmount)),
	}

	return i
}

// ForKey will check if the IP address has exceeded the rate limit
func (i *KeyRateLimiter) ForKey(ctx context.Context, key string) (bool, time.Duration, error) {
	bucket := i.reg.GetOrCreate(key, func() interface{} {
		return limiters.NewTokenBucket(i.rateAmount, i.rateDuration, i.lock, limiters.NewTokenBucketRedis(i.red, fmt.Sprintf("limiter:%s", key), i.rateDuration, false), i.clock, i.logger)
	}, time.Duration(i.rateAmount), i.clock.Now())

	w, err := bucket.(*limiters.TokenBucket).Limit(ctx)
	if err == limiters.ErrLimitExhausted {
		return false, w, nil
	} else if err != nil {
		// The limiter failed. This error should be logged and examined.
		log.Println(err)
		return false, 0, fmt.Errorf("rate limiting err: %s", err)
	}

	return true, 0, nil
}
