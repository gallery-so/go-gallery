package redis

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// FifoQueue implements a reliable unique FIFO queue.
// When an message is popped from the pending queue it gets added to the processing queue which
// is unique to that consumer. When the consumer is done with the message, it is responsible
// for removing the message from its processing queue by calling `Ack`.
type FifoQueue struct {
	client     *redis.Client
	pending    string
	processing string
	id         string
	name       string
	pgSize     int
}

// NewFifoQueue returns a new connection to a queue.
func NewFifoQueue(db int, name string) *FifoQueue {
	id := newConsumerID()
	return &FifoQueue{
		client:     newRedisClient(db),
		pending:    fmt.Sprintf("%s:%s", name, "pending"),
		processing: fmt.Sprintf("%s:%s:%s", name, "processing", id),
		id:         string(id),
		name:       name,
		pgSize:     100,
	}
}

// Push adds an item to the end of the queue.
func (q *FifoQueue) Push(ctx context.Context, value interface{}) (bool, error) {
	added, err := q.client.Do(ctx, "ZADD", q.pending, "NX", float64(time.Now().Unix()), value).Int()
	if err != nil {
		return false, err
	}
	return added > 0, err
}

// popMessage atomically receives a message from the pending queue and adds it to the consumer's processing queue.
var popMessage *redis.Script = redis.NewScript(`
	local item = redis.call("ZPOPMIN", KEYS[1])
	local message = item[1]
	if message == nil then
		return nil
	end
	redis.call("ZADD", KEYS[2], ARGV[1], message)
	return item[1]
`)

// Pop removes the earliest item from the pending queue and adds it to the consumer's processing queue.
func (q *FifoQueue) Pop(ctx context.Context) (string, error) {
	item, err := popMessage.Run(ctx, q.client, []string{q.pending, q.processing}, time.Now().Unix()).Result()
	if err != nil {
		return "", err
	}
	return item.(string), nil
}

// Ack removes the last item from the consumer's processing queue.
func (q *FifoQueue) Ack(ctx context.Context) (string, error) {
	messages, err := q.client.ZPopMin(ctx, q.processing, 1).Result()
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", redis.Nil
	}
	return messages[0].Member.(string), nil
}

// getProcessing returns keys that are being processed.
func (q *FifoQueue) getProcessing(ctx context.Context) []string {
	pattern := fmt.Sprintf("%s:%s:*", q.name, "processing")
	processing := make([]string, 0, 100)
	iterator := q.client.Scan(ctx, 0, pattern, int64(q.pgSize)).Iterator()
	for iterator.Next(ctx) {
		processing = append(processing, iterator.Val())
	}
	return processing
}

// Reprocess moves inactive jobs back to the pending queuing for reprocessing.
func (q *FifoQueue) Reprocess(ctx context.Context, timeout time.Duration, sem *Semaphore) error {
	processing := q.getProcessing(ctx)
	for _, consumerQueue := range processing {
		hasLock, err := sem.hasLock(ctx, consumerIDFromQueue(consumerQueue))
		if err != nil {
			return err
		}
		if hasLock {
			continue
		}
		messages, err := q.client.ZRangeWithScores(ctx, consumerQueue, 0, 0).Result()
		if err != nil {
			return err
		}
		message := messages[0]
		// Remove from processing queue and add back to pending queue
		timeoutOn := time.Unix(int64(message.Score), 0).Add(timeout)
		if time.Now().After(timeoutOn) {
			if _, err := q.client.ZRem(ctx, consumerQueue, message.Member).Result(); err != nil {
				return err
			}
			if _, err := q.Push(ctx, message.Member); err != nil {
				return err
			}
		}
	}
	return nil
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
	id      string
}

// NewSemaphore returns a new instance of a Sempahore.
func NewSemaphore(db int, name string, cap, timeout int) *Semaphore {
	return &Semaphore{
		client:  newRedisClient(db),
		name:    name,
		owners:  fmt.Sprintf("%s:%s", name, "owner"),
		counter: fmt.Sprintf("%s:%s", name, "counter"),
		cap:     cap,
		timeout: timeout,
		id:      newConsumerID(),
	}
}

// Acquire attempts to acquire a semaphore.
func (s *Semaphore) Acquire(ctx context.Context) (bool, error) {
	pipe := s.client.Pipeline()
	defer pipe.Close()

	// Timeout old holders
	done := time.Now().Unix() - int64(s.timeout)
	pipe.ZRemRangeByScore(ctx, s.name, "-inf", fmt.Sprintf("%d", done))
	pipe.ZInterStore(ctx, s.owners, &redis.ZStore{
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
	pipe.Do(ctx, "ZADD", s.name, float64(time.Now().Unix()), s.id)
	pipe.Do(ctx, "ZADD", s.owners, count.Val(), s.id)
	rank := pipe.ZRank(ctx, s.owners, s.id)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Acquired
	if int(rank.Val()) < s.cap {
		return true, nil
	}

	// Failed to acquire, remove from the set
	pipe.ZRem(ctx, s.name, s.id)
	pipe.ZRem(ctx, s.owners, s.id)
	_, err = pipe.Exec(ctx)
	return false, err
}

// Release releases a semaphore.
func (s *Semaphore) Release(ctx context.Context) (bool, error) {
	pipe := s.client.Pipeline()
	defer pipe.Close()

	removed := pipe.ZRem(ctx, s.name, s.id)
	pipe.ZRem(ctx, s.owners, s.id)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	return removed.Val() > 0, nil
}

// Refresh increases the lease on a held sempahore.
func (s *Semaphore) Refresh(ctx context.Context) (bool, error) {
	// Try to update the lease.
	result, err := s.client.Do(ctx, "ZADD", s.name, float64(time.Now().Unix()), s.id).Int()
	if err != nil {
		return false, err
	}

	// Semaphore was lost already
	if result > 0 {
		_, err := s.client.ZRem(ctx, s.name, s.id).Result()
		return false, err
	}

	return true, nil
}

func (s *Semaphore) hasLock(ctx context.Context, consumerID string) (bool, error) {
	_, err := s.client.ZScore(ctx, s.owners, consumerID).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// newConsumerID generates a new consumerID
func newConsumerID() string {
	hostname, err := os.Hostname()
	hostname = strings.ReplaceAll(hostname, ":", "_")
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s_%d", hostname, os.Getpid())
}

func consumerIDFromQueue(queue string) string {
	parts := strings.Split(queue, ":")
	return parts[len(parts)-1]
}
