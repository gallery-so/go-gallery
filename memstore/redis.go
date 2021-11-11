package memstore

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// CollUnassignedRDB is a throttled cache for expensive queries finding unassigned NFTs
	CollUnassignedRDB DB = iota
	// OpenseaRDB is a throttled cache for expensive queries finding Opensea NFTs
	OpenseaRDB
)

// DB represents the database number to use for the redis client
type DB int

// Clients represents the redis clients throughout the application
type Clients struct {
	openseaClient    *redis.Client
	unassignedClient *redis.Client
}

// NewMemstoreClients creates a new redis client for the given database
func NewMemstoreClients(openseaClient *redis.Client, unassignedClient *redis.Client) *Clients {
	return &Clients{
		openseaClient:    openseaClient,
		unassignedClient: unassignedClient,
	}
}

// Set sets a value in the redis cache
func (c *Clients) Set(pCtx context.Context, r DB, key string, value interface{}, expiration time.Duration) error {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.Set(pCtx, key, value, expiration).Err()
	case CollUnassignedRDB:
		return c.unassignedClient.Set(pCtx, key, value, expiration).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// SetMap sets a map value in the redis cache
func (c *Clients) SetMap(pCtx context.Context, r DB, key string, value map[string]interface{}, expiration time.Duration) error {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.HSet(pCtx, key, value, expiration).Err()
	case CollUnassignedRDB:
		return c.unassignedClient.HSet(pCtx, key, value, expiration).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// SetKeepTTL sets a value in the redis cache without resetting TTL
func (c *Clients) SetKeepTTL(pCtx context.Context, r DB, key string, value interface{}) error {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.Set(pCtx, key, value, redis.KeepTTL).Err()
	case CollUnassignedRDB:
		return c.unassignedClient.Set(pCtx, key, value, redis.KeepTTL).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// SetMapKeepTTL sets a map value in the redis cache and ensures TTL lives on
func (c *Clients) SetMapKeepTTL(pCtx context.Context, r DB, key string, value map[string]interface{}, expiration time.Duration) error {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.HSet(pCtx, key, value, redis.KeepTTL).Err()
	case CollUnassignedRDB:
		return c.unassignedClient.HSet(pCtx, key, value, redis.KeepTTL).Err()
	default:
		return errors.New("unknown redis database")
	}
}

// Get gets a value from the redis cache
func (c *Clients) Get(pCtx context.Context, r DB, key string) (string, error) {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.Get(pCtx, key).Result()
	case CollUnassignedRDB:
		return c.unassignedClient.Get(pCtx, key).Result()
	default:
		return "", errors.New("unknown redis database")
	}
}

// GetMapValue gets a value from a map at a given key in the redis cache
func (c *Clients) GetMapValue(pCtx context.Context, r DB, key, mapKey string) (string, error) {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.HGet(pCtx, key, mapKey).Result()
	case CollUnassignedRDB:
		return c.unassignedClient.HGet(pCtx, key, mapKey).Result()
	default:
		return "", errors.New("unknown redis database")
	}
}

// Delete deletes a value from the redis cache
func (c *Clients) Delete(pCtx context.Context, r DB, key string) error {
	switch r {
	case OpenseaRDB:
		return c.openseaClient.Del(pCtx, key).Err()
	case CollUnassignedRDB:
		return c.unassignedClient.Del(pCtx, key).Err()
	default:
		return errors.New("unknown redis database")
	}
}
