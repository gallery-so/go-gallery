package memstore

import (
	"context"
	"time"
)

// Cache represents an in-memory key-value store
type Cache interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(pCtx context.Context, key string) ([]byte, error)
	Delete(pCtx context.Context, key string) error
}
