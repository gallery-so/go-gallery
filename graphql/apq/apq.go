package apq

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/redis"
	"time"
)

type APQCache struct {
	Cache *redis.Cache
}

func (c *APQCache) Add(ctx context.Context, key string, value interface{}) {
	fmt.Printf("ADD: Underlying Type: %T\n", value)
	resultingString, ok := value.(string)

	if !ok {
		return
	}

	c.Cache.Set(ctx, key, []byte(resultingString), time.Hour*24)
}

func (c *APQCache) Get(ctx context.Context, key string) (interface{}, bool) {
	value, _ := c.Cache.Get(ctx, key)

	return string(value), true
}
