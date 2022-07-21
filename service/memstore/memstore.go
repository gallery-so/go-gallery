package memstore

import (
	"context"
	"time"
)

type ErrKeyNotFound struct {
	Key string
}

// Cache represents an in-memory key-value store
type Cache interface {
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
	Get(pCtx context.Context, key string) ([]byte, error)
	Delete(pCtx context.Context, key string) error
	Close(clear bool) error
}

func (k ErrKeyNotFound) Error() string {
	return "key not found: " + k.Key
}
