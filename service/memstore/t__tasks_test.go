package memstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

func TestTasks(t *testing.T) {

	t.Run("can update queue", func(t *testing.T) {
		assert := setupTest(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		redisCache := redis.NewCache(5)

		nft := persist.NFT{
			CollectorsNote: "bob",
			Description:    "test",
		}

		asJSON, err := json.Marshal(nft)
		assert.Nil(err)

		redisCache.Set(ctx, "test", asJSON, time.Hour)

		bs, err := redisCache.Get(ctx, "test")
		assert.Nil(err)

		result := persist.NFT{}
		err = json.Unmarshal([]byte(bs), &result)
		assert.Nil(err)
		assert.Equal(nft.CollectorsNote.String(), result.CollectorsNote.String())

		uq := NewUpdateQueue(redisCache)

		result.Description = "updated"

		asJSON, err = json.Marshal(result)
		assert.Nil(err)
		uq.QueueUpdate("test", asJSON, -1)

		result.Description = "updated2"
		next, err := json.Marshal(result)
		assert.Nil(err)
		uq.QueueUpdate("test", next, -1)

		time.Sleep(time.Second * 3)

		uq.Stop()

		bs, err = redisCache.Get(ctx, "test")
		assert.Nil(err)

		result = persist.NFT{}
		err = json.Unmarshal([]byte(bs), &result)
		assert.Nil(err)
		assert.Equal(result.Description.String(), "updated2")
	})
}
