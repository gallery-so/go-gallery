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

type redisDB int

type CacheConfig struct {
	database    redisDB
	displayName string
	keyPrefix   string
}

const (
	locks                   redisDB = 0
	rateLimiters            redisDB = 1
	communities             redisDB = 2
	misc                    redisDB = 3
	indexerServerThrottle   redisDB = 6
	refreshNFTsThrottle     redisDB = 7
	tokenProcessingThrottle redisDB = 8
	emailThrottle           redisDB = 9
	graphQLAPQ              redisDB = 12
	feed                    redisDB = 13
	social                  redisDB = 14
)

// Every cache is uniquely defined by its database and key prefix. Display names are used for tracing.

var (
	NotificationLockCache        = CacheConfig{database: locks, keyPrefix: "notif", displayName: "notificationLock"}
	EmailRateLimitersCache       = CacheConfig{database: rateLimiters, keyPrefix: "email", displayName: "emailRateLimiters"}
	OneTimeLoginCache            = CacheConfig{database: misc, keyPrefix: "otl", displayName: "oneTimeLogin"}
	CommunitiesCache             = CacheConfig{database: communities, keyPrefix: "", displayName: "communities"}
	IndexerServerThrottleCache   = CacheConfig{database: indexerServerThrottle, keyPrefix: "", displayName: "indexerServerThrottle"}
	RefreshNFTsThrottleCache     = CacheConfig{database: refreshNFTsThrottle, keyPrefix: "", displayName: "refreshNFTsThrottle"}
	TokenProcessingThrottleCache = CacheConfig{database: tokenProcessingThrottle, keyPrefix: "", displayName: "tokenProcessingThrottle"}
	EmailThrottleCache           = CacheConfig{database: emailThrottle, keyPrefix: "", displayName: "emailThrottle"}
	GraphQLAPQCache              = CacheConfig{database: graphQLAPQ, keyPrefix: "", displayName: "graphQLAPQ"}
	FeedCache                    = CacheConfig{database: feed, keyPrefix: "", displayName: "feed"}
	SocialCache                  = CacheConfig{database: social, keyPrefix: "", displayName: "social"}
)

func newClient(db redisDB, traceName string) *redis.Client {
	databaseID := int(db)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	redisURL := env.GetString("REDIS_URL")
	redisPass := env.GetString("REDIS_PASS")
	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       databaseID,
	})
	client.AddHook(tracing.NewRedisHook(databaseID, traceName, true))
	if err := client.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	return client
}

// Cache represents an abstraction over a redis client
type Cache struct {
	client    *redis.Client
	keyPrefix string
}

func (c *Cache) Client() *redis.Client {
	return c.client
}

func (c *Cache) Prefix() string {
	return c.keyPrefix
}

// NewCache creates a new redis cache
func NewCache(config CacheConfig) *Cache {
	return &Cache{
		client:    newClient(config.database, config.displayName),
		keyPrefix: config.keyPrefix,
	}
}

// Set sets a value in the redis cache
func (c *Cache) Set(pCtx context.Context, key string, value []byte, expiration time.Duration) error {
	return c.client.Set(pCtx, c.getPrefixedKey(key), value, expiration).Err()
}

// SetNX sets a value in the redis cache if it doesn't already exist. Returns true if the key did not
// already exist and was set, false if the key did exist and therefore was not set.
func (c *Cache) SetNX(pCtx context.Context, key string, value []byte, expiration time.Duration) (bool, error) {
	cmd := c.client.SetNX(pCtx, c.getPrefixedKey(key), value, expiration)

	err := cmd.Err()
	if err != nil {
		return false, err
	}

	return cmd.Val(), nil
}

// Get gets a value from the redis cache
func (c *Cache) Get(pCtx context.Context, key string) ([]byte, error) {
	bs, err := c.client.Get(pCtx, c.getPrefixedKey(key)).Bytes()
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
	return c.client.Del(pCtx, c.getPrefixedKey(key)).Err()
}

// Close closes the underlying redis client
func (c *Cache) Close() error {
	return c.client.Close()
}

func (c *Cache) getPrefixedKey(key string) string {
	if c.keyPrefix == "" {
		return key
	}

	return c.keyPrefix + ":" + key
}

func (c *Cache) getPrefixedKeys(keys []string) []string {
	if c.keyPrefix == "" {
		return keys
	}

	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = c.keyPrefix + ":" + key
	}
	return prefixedKeys
}

func (e ErrKeyNotFound) Error() string {
	return fmt.Sprintf("key %s not found", e.Key)
}

func NewLockClient(cache *Cache) *redislock.Client {
	return redislock.New(&redislockCacheClient{cache: cache})
}

// redislockCacheClient is a minimal implementation of redislock.RedisClient that uses a Cache to namespace its keys
type redislockCacheClient struct {
	cache *Cache
}

func (r *redislockCacheClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	return r.cache.client.SetNX(ctx, r.cache.getPrefixedKey(key), value, expiration)
}

func (r *redislockCacheClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return r.cache.client.Eval(ctx, script, r.cache.getPrefixedKeys(keys), args...)
}

func (r *redislockCacheClient) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd {
	return r.cache.client.EvalSha(ctx, sha1, r.cache.getPrefixedKeys(keys), args...)
}

func (r *redislockCacheClient) ScriptExists(ctx context.Context, scripts ...string) *redis.BoolSliceCmd {
	return r.cache.client.ScriptExists(ctx, scripts...)
}

func (r *redislockCacheClient) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	return r.cache.client.ScriptLoad(ctx, script)
}
