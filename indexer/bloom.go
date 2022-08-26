package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/tracing"
)

const (
	db       = 0
	dbName   = "IndexerDB"
	bfAdd    = "BF.ADD"
	bfMadd   = "BF.MADD"
	bfExists = "BF.EXISTS"
	// Filter to check if an address is present in a transfer event
	// across a block range. Both ends of the range are inclusive.
	transferAddressFilterFmt = "filter:transferAddress:%d:%d"
)

// TransferFilter is a bloom filter that checks if an address
// had a transfer event (either to and from) in a block range.
type TransferFilter struct {
	client *redis.Client
	fmt    string
}

// NewTransferFilter returns a new instance of TransferFilter.
func NewTransferFilter(url, pass string) *TransferFilter {
	client := redis.NewClient(&redis.Options{
		Addr:     url,
		Password: pass,
		DB:       db,
	})
	client.AddHook(tracing.NewRedisHook(db, dbName, true))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		panic(err)
	}

	return &TransferFilter{
		client: client,
		fmt:    transferAddressFilterFmt,
	}
}

// Add adds an address to a block range.
func (t *TransferFilter) Add(ctx context.Context, start, end persist.BlockNumber, address persist.EthereumAddress) (inserted bool, err error) {
	key := fmt.Sprintf(t.fmt, start, end)
	return t.client.Do(ctx, bfAdd, key, address.String()).Bool()
}

// AddMultiple adds multiple addresses to a block range.
func (t *TransferFilter) AddMultiple(ctx context.Context, start, end persist.BlockNumber, addresses ...persist.EthereumAddress) (inserted []bool, err error) {
	key := fmt.Sprintf(t.fmt, start, end)

	args := []interface{}{bfMadd, key}
	for _, address := range addresses {
		args = append(args, address.String())
	}

	return t.client.Do(ctx, args...).BoolSlice()
}

// Exists checks if an address exists in a block range.
// The check is probabilistic so false positives are possible i.e. an address existing in a block range when it doesn't.
func (t *TransferFilter) Exists(ctx context.Context, start, end persist.BlockNumber, address persist.EthereumAddress) (exists bool, err error) {
	key := fmt.Sprintf(t.fmt, start, end)
	return t.client.Do(ctx, bfExists, key, address.String()).Bool()
}
