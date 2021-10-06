package redis

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

const (
	// CollUnassignedRDB is a throttled cache for expensive queries finding unassigned NFTs
	CollUnassignedRDB DB = iota
	// OpenseaRDB is a throttled cache for expensive queries finding Opensea NFTs
	OpenseaRDB
)

// Redis is the set of active redis clients
var clients *Clients

// DB represents the database number to use for the redis client
type DB int

// Clients represents the redis clients throughout the application
type Clients struct {
	OpenseaClient    *redis.Client
	UnassignedClient *redis.Client
}

func init() {
	viper.SetDefault("REDIS_URL", "localhost:6379")
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	opensea := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       int(OpenseaRDB),
	})
	if err := opensea.Ping(context.Background()).Err(); err != nil {
		panic(err)
	}
	unassigned := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       int(CollUnassignedRDB),
	})
	if err := unassigned.Ping(context.Background()).Err(); err != nil {
		panic(err)
	}
	clients = &Clients{
		OpenseaClient:    opensea,
		UnassignedClient: unassigned,
	}
}

// Set sets a value in the redis cache
func Set(pCtx context.Context, r DB, key string, value interface{}, expiration time.Duration) error {
	switch r {
	case OpenseaRDB:
		return clients.OpenseaClient.Set(pCtx, key, value, expiration).Err()
	case CollUnassignedRDB:
		return clients.UnassignedClient.Set(pCtx, key, value, expiration).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// SetKeepTTL sets a value in the redis cache without resetting TTL
func SetKeepTTL(pCtx context.Context, r DB, key string, value interface{}) error {
	switch r {
	case OpenseaRDB:
		return clients.OpenseaClient.Set(pCtx, key, value, redis.KeepTTL).Err()
	case CollUnassignedRDB:
		return clients.UnassignedClient.Set(pCtx, key, value, redis.KeepTTL).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// Get gets a value from the redis cache
func Get(pCtx context.Context, r DB, key string) (string, error) {
	switch r {
	case OpenseaRDB:
		return clients.OpenseaClient.Get(pCtx, key).Result()
	case CollUnassignedRDB:
		return clients.UnassignedClient.Get(pCtx, key).Result()
	default:
		return "", errors.New("unknown redis database")
	}
}

// Delete deletes a value from the redis cache
func Delete(pCtx context.Context, r DB, key string) error {
	switch r {
	case OpenseaRDB:
		return clients.OpenseaClient.Del(pCtx, key).Err()
	case CollUnassignedRDB:
		return clients.UnassignedClient.Del(pCtx, key).Err()
	default:
		return errors.New("unknown redis database")
	}
}
