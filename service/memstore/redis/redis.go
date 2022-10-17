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
	TokenProcessingThrottleDB = 8
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

// NewCache creates a new redis cache
func NewCache(db int) *Cache {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       db,
	})
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
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       db,
	})
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
