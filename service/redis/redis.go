package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/bsm/redislock"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/tracing"

	"github.com/go-redis/redis/v8"
)

type ErrKeyNotFound struct {
	Key string
}

const (
	GalleriesDB               = 0
	GalleriesTokenDB          = 1
	CommunitiesDB             = 2
	RequireNftsDB             = 3
	TestSuiteDB               = 5
	IndexerServerThrottleDB   = 6
	RefreshNFTsThrottleDB     = 7
	TokenProcessingThrottleDB = 8
	EmailThrottleDB           = 9
	NotificationLockDB        = 10
	EmailRateLimiterDB        = 11
	GraphQLAPQ                = 12
	FeedDB                    = 13
	SocialDB                  = 14
)

// GetNameForDatabase returns a name for the given database ID, if available.
// This is useful for adding debug information to Redis calls (like tracing).
func GetNameForDatabase(databaseId int) string {
	switch databaseId {
	case GalleriesDB:
		return "GalleriesDB"
	case GalleriesTokenDB:
		return "GalleriesTokenDB"
	case CommunitiesDB:
		return "CommunitiesDB"
	case RequireNftsDB:
		return "RequireNftsDB"
	case TestSuiteDB:
		return "TestSuiteDB"
	case FeedDB:
		return "FeedDB"
	}

	return fmt.Sprintf("db %d", databaseId)
}

func NewClient(db int) *redis.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	redisURL := env.GetString(ctx, "REDIS_URL")
	redisPass := env.GetString(ctx, "REDIS_PASS")
	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       db,
	})
	client.AddHook(tracing.NewRedisHook(db, GetNameForDatabase(db), true))
	if err := client.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	return client
}

// Cache represents an abstraction over a redist client
type Cache struct {
	client *redis.Client
}

// NewCache creates a new redis cache
func NewCache(db int) *Cache {
	return &Cache{client: NewClient(db)}
}

// ClearCache deletes the entire cache
func ClearCache(db int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	client := NewClient(db)
	return client.FlushDB(ctx).Err()
}

// Set sets a value in the redis cache
func (c *Cache) Set(pCtx context.Context, key string, value []byte, expiration time.Duration) error {
	return c.client.Set(pCtx, key, value, expiration).Err()
}

// Get gets a value from the redis cache
func (c *Cache) Get(pCtx context.Context, key string) ([]byte, error) {
	bs, err := c.client.Get(pCtx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrKeyNotFound{Key: key}
		}
		return nil, err
	}
	return bs, nil
}

// Delete deletes a value from the redis cache
func (c *Cache) Delete(pCtx context.Context, key string) error {
	return c.client.Del(pCtx, key).Err()
}

// Close closes the redis client and optionally deletes the cache
func (c *Cache) Close(clear bool) error {
	if clear {
		if err := c.client.FlushDB(context.Background()).Err(); err != nil {
			return err
		}
	}
	return c.client.Close()
}

func NewLockClient(db int) *redislock.Client {
	return redislock.New(NewClient(db))
}

// GlobalLock is a distributed lock that does not depend on a key.
// It can store its own context and lock because we can assume that there never be more than one global lock in use at a time
// and therefore no concurrency issues with overwriting the context and lock.
type GlobalLock struct {
	client *redislock.Client
	ttl    time.Duration
	lock   *redislock.Lock
}

func NewGlobalLock(client *redislock.Client, ttl time.Duration) *GlobalLock {
	return &GlobalLock{client: client, ttl: ttl}
}

func (l *GlobalLock) Lock(ctx context.Context) error {
	lock, err := l.client.Obtain(ctx, "lock", l.ttl, &redislock.Options{RetryStrategy: redislock.LimitRetry(redislock.LinearBackoff(time.Second), 10)})
	if err != nil {
		return err
	}
	l.lock = lock
	return nil
}

func (l *GlobalLock) Unlock(ctx context.Context) error {
	return l.lock.Release(ctx)
}

func (e ErrKeyNotFound) Error() string {
	return fmt.Sprintf("key %s not found", e.Key)
}
