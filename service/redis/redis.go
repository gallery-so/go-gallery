package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/bsm/redislock"
	"github.com/go-redis/redis/v8"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
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
	locks                 redisDB = 0
	rateLimiters          redisDB = 1
	communities           redisDB = 2
	misc                  redisDB = 3
	indexerServerThrottle redisDB = 6
	refreshNFTsThrottle   redisDB = 7
	tokenProcessing       redisDB = 8
	emailThrottle         redisDB = 9
	graphQLAPQ            redisDB = 12
	feed                  redisDB = 13
	social                redisDB = 14
)

// Every cache is uniquely defined by its database and key prefix. Display names are used for tracing.

var (
	NotificationLockCache             = CacheConfig{database: locks, keyPrefix: "notif", displayName: "notificationLock"}
	EmailRateLimitersCache            = CacheConfig{database: rateLimiters, keyPrefix: "email", displayName: "emailRateLimiters"}
	PushNotificationRateLimitersCache = CacheConfig{database: rateLimiters, keyPrefix: "push", displayName: "pushNotificationLimiters"}
	OneTimeLoginCache                 = CacheConfig{database: misc, keyPrefix: "otl", displayName: "oneTimeLogin"}
	AuthTokenForceRefreshCache        = CacheConfig{database: misc, keyPrefix: "authRefresh", displayName: "authTokenForceRefresh"}
	CommunitiesCache                  = CacheConfig{database: communities, keyPrefix: "", displayName: "communities"}
	IndexerServerThrottleCache        = CacheConfig{database: indexerServerThrottle, keyPrefix: "", displayName: "indexerServerThrottle"}
	RefreshNFTsThrottleCache          = CacheConfig{database: refreshNFTsThrottle, keyPrefix: "", displayName: "refreshNFTsThrottle"}
	TokenProcessingThrottleCache      = CacheConfig{database: tokenProcessing, keyPrefix: "throttle", displayName: "tokenProcessingThrottle"}
	TokenProcessingMetadataCache      = CacheConfig{database: tokenProcessing, keyPrefix: "metadata", displayName: "tokenProcessingMetadata"}
	EmailThrottleCache                = CacheConfig{database: emailThrottle, keyPrefix: "", displayName: "emailThrottle"}
	GraphQLAPQCache                   = CacheConfig{database: graphQLAPQ, keyPrefix: "", displayName: "graphQLAPQ"}
	FeedCache                         = CacheConfig{database: feed, keyPrefix: "", displayName: "feed"}
	SocialCache                       = CacheConfig{database: social, keyPrefix: "", displayName: "social"}
	SearchCache                       = CacheConfig{keyPrefix: "search", displayName: "search"}
	UserPrefCache                     = CacheConfig{keyPrefix: "userpref", displayName: "userPref"}
	TokenManageCache                  = CacheConfig{keyPrefix: "tokenmanage", displayName: "tokenManage"}
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
	scripter  *scripter
}

func (c *Cache) Client() *redis.Client {
	return c.client
}

func (c *Cache) Prefix() string {
	return c.keyPrefix
}

// Scripter returns an implementation of the redis.Scripter interface using this Cache
func (c *Cache) Scripter() redis.Scripter {
	return c.scripter
}

// NewCache creates a new redis cache
func NewCache(config CacheConfig) *Cache {
	cache := &Cache{
		client:    newClient(config.database, config.displayName),
		keyPrefix: config.keyPrefix,
	}

	cache.scripter = &scripter{cache: cache}

	return cache
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

// MSetWithTTL sets multiple keys in the redis cache via pipelining.
func (c *Cache) MSetWithTTL(ctx context.Context, keyValues map[string]any, expiration time.Duration) error {
	p := c.client.Pipeline()
	defer p.Close()
	for k, v := range keyValues {
		p.Set(ctx, c.getPrefixedKey(k), v, expiration)
	}
	_, err := p.Exec(ctx)
	return err
}

// SetTime sets a time in the redis cache. If onlyIfLater is true, the value will only be set if the
// key doesn't exist, or if the existing key's value is an earlier time than the one being set.
func (c *Cache) SetTime(ctx context.Context, key string, value time.Time, expiration time.Duration, onlyIfLater bool) error {
	unixTimestamp := value.Unix()

	if !onlyIfLater {
		return c.client.Set(ctx, c.getPrefixedKey(key), unixTimestamp, expiration).Err()
	}

	unixExpiration := time.Now().Add(expiration).Unix()

	// scripter already handled key prefixing, so we need to pass the raw key here
	err := setTimeScript.Run(ctx, c.scripter, []string{key}, unixTimestamp, unixExpiration).Err()
	if err != nil {
		return err
	}

	return nil
}

func (c *Cache) GetTime(ctx context.Context, key string) (time.Time, error) {
	result, err := c.client.Get(ctx, c.getPrefixedKey(key)).Result()
	if err != nil {
		if err == redis.Nil {
			return time.Time{}, ErrKeyNotFound{Key: key}
		}
		return time.Time{}, err
	}

	timestamp, err := strconv.ParseInt(result, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(timestamp, 0), nil
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

// scripter is an implementation of the redis.Scripter interface that uses a Cache to namespace keys
type scripter struct {
	cache *Cache
}

func (s scripter) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return s.cache.client.Eval(ctx, script, s.cache.getPrefixedKeys(keys), args...)
}

func (s scripter) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd {
	return s.cache.client.EvalSha(ctx, sha1, s.cache.getPrefixedKeys(keys), args...)
}

func (s scripter) ScriptExists(ctx context.Context, scripts ...string) *redis.BoolSliceCmd {
	return s.cache.client.ScriptExists(ctx, scripts...)
}

func (s scripter) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	return s.cache.client.ScriptLoad(ctx, script)
}

func NewLockClient(cache *Cache) *redislock.Client {
	return redislock.New(&redislockCacheClient{
		scripter: *cache.scripter,
	})
}

// redislockCacheClient is a minimal implementation of redislock.RedisClient that uses a Cache to namespace its keys.
type redislockCacheClient struct {
	scripter
}

func (r *redislockCacheClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	return r.cache.client.SetNX(ctx, r.cache.getPrefixedKey(key), value, expiration)
}

// Scripts
var setTimeScript = redis.NewScript(`
local key = KEYS[1]
local newTimestamp = ARGV[1]
local expirationTime = ARGV[2]

local currentTimestamp = redis.call('GET', key)

if currentTimestamp == false or tonumber(currentTimestamp) < tonumber(newTimestamp) then
	redis.call('SET', key, newTimestamp)
	redis.call('EXPIREAT', key, expirationTime)
    return 1
end

return 0
`)

// LazyCache implements a lazy loading cache that stores data only when it is requested
type LazyCache struct {
	Cache    *Cache
	CalcFunc func(context.Context) ([]byte, error)
	Key      string
	TTL      time.Duration
}

// Load queries the cache for the given key, and if it is current returns the data.
// It's possible for Load to return stale data, however the staleness of data can be
// limited by configuring a shorter TTL. The tradeoff being that a shorter TTL results in more
// cache misses which can have a noticeable delay in getting data.
func (l LazyCache) Load(ctx context.Context) ([]byte, error) {
	b, err := l.Cache.Get(ctx, l.Key)
	if err == nil {
		return b, nil
	}
	if !util.ErrorAs[ErrKeyNotFound](err) {
		return nil, err
	}
	b, err = l.CalcFunc(ctx)
	if err != nil {
		return nil, err
	}
	err = l.Cache.Set(ctx, l.Key, b, l.TTL)
	return b, err
}
