package apq

import (
	"context"
	"encoding/json"
	"github.com/mikeydub/go-gallery/service/redis"
	"time"
)

type APQCache struct {
	Cache *redis.Cache
}

func (c *APQCache) Add(ctx context.Context, key string, value interface{}) {
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

func (c *APQCache) UploadPersistedQueries(ctx context.Context, persistedQueriesString string) error {
	persistedQueries := map[string]string{}

	err := json.Unmarshal([]byte(persistedQueriesString), &persistedQueries)

	if err != nil {
		return err
	}

	for hash, queryText := range persistedQueries {
		c.Add(ctx, hash, queryText)
	}

	return nil
}
