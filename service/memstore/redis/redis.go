package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/tracing"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

const (
	GalleriesDB               = 0
	GalleriesTokenDB          = 1
	CommunitiesDB             = 2
	RequireNftsDB             = 3
	TestSuiteDB               = 5
	IndexerServerThrottleDB   = 6
	RefreshNFTsThrottleDB     = 7
	MediaProcessingThrottleDB = 8
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
	}

	return fmt.Sprintf("db %d", databaseId)
}

// Cache represents an abstraction over a redist client
type Cache struct {
	client *redis.Client
}

func newRedisClient(db int) *redis.Client {
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	return redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       db,
	})
}

// NewCache creates a new redis cache
func NewCache(db int) *Cache {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	client := newRedisClient(db)
	client.AddHook(tracing.NewRedisHook(db, GetNameForDatabase(db), true))
	if err := client.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	return &Cache{client: client}
}

// ClearCache deletes the entire cache
func ClearCache(db int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	client := newRedisClient(db)
	client.AddHook(tracing.NewRedisHook(db, GetNameForDatabase(db), true))
	return client.FlushAll(ctx).Err()
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
			return nil, memstore.ErrKeyNotFound{Key: key}
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

// FifoQueue implements a first-in, first-out queue with Redis Lists.
type FifoQueue struct {
	client *redis.Client
	name   string
}

// NewFifoQueue returns a connection to a new connection to a queue.
func NewFifoQueue(db int, name string) *FifoQueue {
	return &FifoQueue{
		client: newRedisClient(db),
		name:   name,
	}
}

// Push adds an item to the end of the queue.
func (q *FifoQueue) Push(ctx context.Context, value interface{}) (size int, err error) {
	sz, err := q.client.RPush(ctx, q.name, value).Result()
	return int(sz), err
}

// LPush adds an item to the beginning of the queue.
func (q *FifoQueue) LPush(ctx context.Context, value interface{}) (size int, err error) {
	sz, err := q.client.LPush(ctx, q.name, value).Result()
	return int(sz), err
}

// Pop gets the first item from the queue, blocking until an item is received or until the timeout.
func (q *FifoQueue) Pop(ctx context.Context, wait time.Duration) (string, error) {
	reply, err := q.client.BLPop(ctx, wait, q.name).Result()

	if err != nil {
		return "", err
	}

	// Reply is in format: [key, value]
	return reply[1], nil
}

// Semaphore implements a semaphore in Redis described here:
// https://redis.com/ebook/part-2-core-concepts/chapter-6-application-components-in-redis/6-3-counting-semaphores/
type Semaphore struct {
	client  *redis.Client
	name    string
	owners  string
	counter string
	cap     int
	timeout int
}

// NewSemaphore returns a new instance of a Sempahore.
func NewSemaphore(db int, name string, cap, timeout int) *Semaphore {
	return &Semaphore{
		client:  newRedisClient(db),
		name:    name,
		owners:  fmt.Sprintf("%s:%s", name, ":owner"),
		counter: fmt.Sprintf("%s:%s", name, ":counter"),
		cap:     cap,
		timeout: timeout,
	}
}

// Acquire attempts to acquire a semaphore.
func (s *Semaphore) Acquire(ctx context.Context, id string) (bool, error) {
	pipe := s.client.Pipeline()
	defer pipe.Close()

	// Timeout old holders
	done := time.Now().Unix() - int64(s.timeout)
	pipe.ZRemRangeByScore(ctx, s.name, "-inf", fmt.Sprintf("%d", done))
	pipe.ZInterWithScores(ctx, &redis.ZStore{
		Keys:      []string{s.owners, s.name},
		Weights:   []float64{1, 0},
		Aggregate: "SUM",
	})

	count := pipe.Incr(ctx, s.counter)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Try to acquire the semaphore
	pipe.Do(ctx, "ZADD", s.name, float64(time.Now().Unix()), id)
	pipe.Do(ctx, "ZADD", s.owners, count.Val(), id)
	rank := pipe.ZRank(ctx, s.name, id)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Acquired
	if int(rank.Val()) < s.cap {
		return true, nil
	}

	// Failed to acquire, remove from the set
	pipe.ZRem(ctx, s.name, id)
	pipe.ZRem(ctx, s.owners, id)
	_, err = pipe.Exec(ctx)
	return false, err
}

// Release releases a semaphore.
func (s *Semaphore) Release(ctx context.Context, id string) (bool, error) {
	pipe := s.client.Pipeline()
	defer pipe.Close()

	removed := pipe.ZRem(ctx, s.name, id)
	pipe.ZRem(ctx, s.owners, id)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return removed.Val() > 0, nil
}

// Refresh increases the lease on a held sempahore.
func (s *Semaphore) Refresh(ctx context.Context, id string) (bool, error) {
	result, err := s.client.Do(ctx, "ZADD", s.name, float64(time.Now().Unix()), id).Int64()
	if err != nil {
		return false, err
	}

	// Semaphore was lost already
	if result > 0 {
		_, err := s.client.ZRem(ctx, s.name, id).Result()
		return false, err
	}

	return true, nil
}

// Exists returns true if a semaphore is held by the id.
func (s *Semaphore) Exists(ctx context.Context, id string) (bool, error) {
	_, err := s.client.ZScore(ctx, s.name, id).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
