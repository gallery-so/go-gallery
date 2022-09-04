//go:generate go run github.com/vektah/dataloaden AddressFilterLoader github.com/mikeydub/go-gallery/db/sqlc/indexergen.GetAddressFilterBatchParams github.com/mikeydub/go-gallery/db/sqlc/indexergen.AddressFilter

package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bits-and-blooms/bloom"
	"github.com/jackc/pgx/v4"
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/indexergen"
	"github.com/mikeydub/go-gallery/service/logger"
	memstore "github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"

	"github.com/go-redis/redis/v8"
)

var defaultRefreshConfig RefreshConfig = RefreshConfig{
	// XXX: DefaultNoMessageWaitTime:  5 * time.Minute,
	DefaultNoMessageWaitTime:  5 * time.Second,
	DefaultPoolSize:           defaultWorkerPoolSize,
	LookbackWindow:            100000,
	DataloaderDefaultMaxBatch: 0, // no restriction on batch size
	DataloaderDefaultWaitTime: 2 * time.Millisecond,
	RefreshQueueName:          "deepRefresh:addressQueue",
	RefreshLockName:           "deepRefresh:addressLock",
	MaxConcurrentRuns:         24,
	Liveness:                  15 * 60,
}

// ErrNoFilter is returned when a filter does not exist.
var ErrNoFilter = errors.New("no filter")

// ErrRefreshTimedOut is returned when refresh is expired.
var ErrRefreshTimedOut = errors.New("refresh timed out")

// ErrNoMessage is returned when there are no messages to work on.
var ErrNoMessage = errors.New("no queued messages")

// ErrPopFromEmpty is returned when a pop from the processing queue returns no result.
var ErrPopFromEmpty = errors.New("processing queue is empty; expected one message")

// ErrUnexpectedMessage is returned when the message that was handled is not the
// message that is in the processing queue.
var ErrUnexpectedMessage = errors.New("message in processing queue is not the message that was handled")

// RefreshConfig configures how deep refreshes are ran.
type RefreshConfig struct {
	DefaultNoMessageWaitTime  time.Duration // How long to wait before polling for a message
	DefaultPoolSize           int           // Number of workers to allocate to a refresh
	LookbackWindow            int           // Refreshes will start this many blocks before the last indexer block processed
	DataloaderDefaultMaxBatch int           // The max batch size before submitting a batch
	DataloaderDefaultWaitTime time.Duration // Max time to wait before submitting a batch
	RefreshQueueName          string        // The name of the queue to buffer refreshes
	RefreshLockName           string        // The name of the lock to acquire refreshes
	MaxConcurrentRuns         int           // The number of refreshes that can run concurrently
	Liveness                  int           // How often jobs needs to refresh their locks
}

// RefreshQueue buffers deep refresh requests.
type RefreshQueue struct {
	q *memstore.FifoQueue
}

// NewRefreshQueue returns a connection to the refresh queue.
func NewRefreshQueue() *RefreshQueue {
	return &RefreshQueue{memstore.NewFifoQueue(memstore.IndexerServerThrottleDB, defaultRefreshConfig.RefreshQueueName)}
}

// Add adds a message to the queue.
func (r *RefreshQueue) Add(ctx context.Context, input UpdateTokenMediaInput) error {
	message, err := json.Marshal(input)
	if err != nil {
		return err
	}
	added, err := r.q.Push(ctx, message)
	if err != nil {
		return err
	}
	if !added {
		logger.For(ctx).Info("refresh already exists, skipping")
	}

	return nil
}

// Get blocks until a message is received from the queue.
func (r *RefreshQueue) Get(ctx context.Context) (UpdateTokenMediaInput, error) {
	queued, err := r.q.Pop(ctx, 0)
	if err == redis.Nil {
		return UpdateTokenMediaInput{}, ErrNoMessage
	}
	if err != nil {
		return UpdateTokenMediaInput{}, err
	}

	var msg UpdateTokenMediaInput
	err = json.Unmarshal([]byte(queued), &msg)
	if err != nil {
		return UpdateTokenMediaInput{}, err
	}

	return msg, nil
}

// Ack completes a message by removing it from the processing queue.
func (r *RefreshQueue) Ack(ctx context.Context, processed UpdateTokenMediaInput) error {
	last, err := r.q.Ack(ctx)

	// Should always be at most one message in the processing queue if
	// `processed` was obtained by calling `Get`.
	if err == redis.Nil {
		return ErrPopFromEmpty
	}

	processedMessage, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	// `last` should always be the same message as `processed`.
	if last != string(processedMessage) {
		return ErrUnexpectedMessage
	}

	return nil
}

// ReAdd adds a message to the top of the queue.
func (r *RefreshQueue) ReAdd(ctx context.Context, input UpdateTokenMediaInput) error {
	// Remove from processing queue.
	err := r.Ack(ctx, input)
	if err != nil {
		return err
	}

	message, err := json.Marshal(input)
	if err != nil {
		return err
	}

	// Add to front of pending queue.
	added, err := r.q.LPush(ctx, message)
	if err != nil {
		return err
	}
	if !added {
		logger.For(ctx).Info("refresh already exists, skipping")
	}

	return nil
}

// RefreshLock manages the number of concurrent refreshes allowed.
type RefreshLock struct {
	s *memstore.Semaphore
}

// NewRefreshLock returns a connection to the fresh lock.
func NewRefreshLock() *RefreshLock {
	return &RefreshLock{
		s: memstore.NewSemaphore(
			memstore.IndexerServerThrottleDB,
			defaultRefreshConfig.RefreshLockName,
			defaultRefreshConfig.MaxConcurrentRuns,
			defaultRefreshConfig.Liveness,
		),
	}
}

// Acquire attempts to acquire permission to run a refresh.
func (r *RefreshLock) Acquire(ctx context.Context, input UpdateTokenMediaInput) (bool, error) {
	key := fmt.Sprintf("%s:%s:%s", input.OwnerAddress, input.ContractAddress, input.TokenID)
	return r.s.Acquire(ctx, key)
}

// Release removes a refresh from the running jobs.
func (r *RefreshLock) Release(ctx context.Context, input UpdateTokenMediaInput) (bool, error) {
	key := fmt.Sprintf("%s:%s:%s", input.OwnerAddress, input.ContractAddress, input.TokenID)
	return r.s.Release(ctx, key)
}

// Refresh updates the lease on a running job.
func (r *RefreshLock) Refresh(ctx context.Context, input UpdateTokenMediaInput) (bool, error) {
	key := fmt.Sprintf("%s:%s:%s", input.OwnerAddress, input.ContractAddress, input.TokenID)
	return r.s.Refresh(ctx, key)
}

// Exists checks if a job is already running.
func (r *RefreshLock) Exists(ctx context.Context, input UpdateTokenMediaInput) (bool, error) {
	key := fmt.Sprintf("%s:%s:%s", input.OwnerAddress, input.ContractAddress, input.TokenID)
	return r.s.Exists(ctx, key)
}

// AddressExists checks if an address transacted in a block range.
func AddressExists(fm *BlockFilterManager, address persist.EthereumAddress, from, to persist.BlockNumber) (bool, error) {
	bf, err := fm.Get(from, to)
	if err != nil {
		return false, err
	}
	return bf.TestString(address.String()), nil
}

// BlockFilterManager handles the downloading and removing of block filters.
type BlockFilterManager struct {
	loader AddressFilterLoader
}

// NewBlockFilterManager returns a new instance of a BlockFilterManager.
func NewBlockFilterManager(ctx context.Context, q *sqlc.Queries) *BlockFilterManager {
	return &BlockFilterManager{
		loader: AddressFilterLoader{
			maxBatch: defaultRefreshConfig.DataloaderDefaultMaxBatch,
			wait:     defaultRefreshConfig.DataloaderDefaultWaitTime,
			fetch:    fetcher(ctx, q),
		},
	}
}

// Get returns a filter if one exists.
func (b *BlockFilterManager) Get(from, to persist.BlockNumber) (*bloom.BloomFilter, error) {
	data, err := b.loader.Load(sqlc.GetAddressFilterBatchParams{
		FromBlock: from,
		ToBlock:   to,
	})
	if err != nil && err.Error() == pgx.ErrNoRows.Error() {
		return nil, ErrNoFilter
	}
	if err != nil {
		return nil, err
	}

	var bf bloom.BloomFilter
	if err := bf.UnmarshalJSON(data.BloomFilter); err != nil {
		return nil, err
	}

	return &bf, nil
}

// Clear removes the filter from it's cache.
func (b *BlockFilterManager) Clear(ctx context.Context, from, to persist.BlockNumber) {
	b.loader.Clear(sqlc.GetAddressFilterBatchParams{FromBlock: from, ToBlock: to})
}

func fetcher(ctx context.Context, q *sqlc.Queries) func([]sqlc.GetAddressFilterBatchParams) ([]sqlc.AddressFilter, []error) {
	return func(params []sqlc.GetAddressFilterBatchParams) ([]sqlc.AddressFilter, []error) {
		filters := make([]sqlc.AddressFilter, len(params))
		errs := make([]error, len(params))

		b := q.GetAddressFilterBatch(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, bf sqlc.AddressFilter, err error) {
			filters[i], errs[i] = bf, err
		})

		return filters, errs
	}
}
