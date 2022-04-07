package redis

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

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
	if err := client.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	return &Cache{client: client}
}

// ClearCache deletes the entire cache
func ClearCache() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       0,
	})
	return client.FlushAll(ctx).Err()
}

// Set sets a value in the redis cache
func (c *Cache) Set(pCtx context.Context, key string, value []byte, expiration time.Duration) error {
	return c.client.Set(pCtx, key, value, expiration).Err()
}

// Get gets a value from the redis cache
func (c *Cache) Get(pCtx context.Context, key string) ([]byte, error) {
	return c.client.Get(pCtx, key).Bytes()
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
