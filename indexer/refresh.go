//go:generate go run github.com/vektah/dataloaden AddressFilterLoader github.com/mikeydub/go-gallery/db/sqlc/indexergen.GetAddressFilterBatchParams github.com/mikeydub/go-gallery/db/sqlc/indexergen.AddressFilter

package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom"
	lru "github.com/hashicorp/golang-lru"
	"github.com/jackc/pgx/v4"
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/indexergen"
	"github.com/mikeydub/go-gallery/service/logger"
	memstore "github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"golang.org/x/sync/errgroup"

	"github.com/go-redis/redis/v8"
)

var defaultRefreshConfig RefreshConfig = RefreshConfig{
	// XXX: DefaultNoMessageWaitTime:  5 * time.Minute,
	DefaultNoMessageWaitTime:  5 * time.Second,
	DefaultPoolSize:           defaultWorkerPoolSize,
	ChunkSize:                 10000,
	CacheSize:                 8,
	ChunkWorkerSize:           128,
	LookbackWindow:            1400000,
	DataloaderDefaultMaxBatch: 1000,
	DataloaderDefaultWaitTime: 2 * time.Millisecond,
	RefreshQueueName:          "deepRefresh:addressQueue",
	RefreshLockName:           "deepRefresh:addressLock",
	MaxConcurrentRuns:         24,
	Liveness:                  5 * 60,
}

// ErrNoFilter is returned when a filter does not exist.
var ErrNoFilter = errors.New("no filter")

// ErrRefreshTimedOut is returned when a consumer no longer holds a lock.
var ErrRefreshTimedOut = errors.New("refresh timed out")

// ErrNoMessage is returned when there are no messages to work on.
var ErrNoMessage = errors.New("no queued messages")

// ErrPopFromEmpty is returned when a pop from the processing queue returns no result.
var ErrPopFromEmpty = errors.New("processing queue is empty; expected one message")

// ErrUnexpectedMessage is returned when the message that was handled is not the same message that is in the processing queue.
var ErrUnexpectedMessage = errors.New("message in processing queue is not the message that was handled")

// RefreshConfig configures how deep refreshes are ran.
type RefreshConfig struct {
	DefaultNoMessageWaitTime  time.Duration // How long to wait before polling for a message
	DefaultPoolSize           int           // Number of workers to allocate to a refresh
	LookbackWindow            int           // Refreshes will start this many blocks before the last indexer block processed
	ChunkSize                 int           // The number of filters in a chunk
	CacheSize                 int           // The number of chunks to keep on disk
	ChunkWorkerSize           int           // The number of workers used to download a chunk
	DataloaderDefaultMaxBatch int           // The max batch size before submitting a batch
	DataloaderDefaultWaitTime time.Duration // Max time to wait before submitting a batch
	RefreshQueueName          string        // The name of the queue to buffer refreshes
	RefreshLockName           string        // The name of the lock ask permission to run a refresh
	MaxConcurrentRuns         int           // The number of refreshes that can run concurrently
	Liveness                  int           // How frequently a consumer needs to refresh its lock
}

// RefreshQueue buffers refresh requests.
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
func (r *RefreshLock) Acquire(ctx context.Context) (bool, error) {
	return r.s.Acquire(ctx)
}

// Release removes a refresh from the running jobs.
func (r *RefreshLock) Release(ctx context.Context) (bool, error) {
	return r.s.Release(ctx)
}

// Refresh updates the lease on a running job.
func (r *RefreshLock) Refresh(ctx context.Context) (bool, error) {
	return r.s.Refresh(ctx)
}

// AddressExists checks if an address transacted in a block range.
func AddressExists(ctx context.Context, fm *BlockFilterManager, address persist.EthereumAddress, from, to persist.BlockNumber) (bool, error) {
	bf, err := fm.Get(ctx, from, to)
	if err != nil {
		return false, err
	}
	return bf.TestString(address.String()), nil
}

// BlockFilterManager handles the downloading and removing of block filters.
type BlockFilterManager struct {
	blocksPerLogFile int
	chunkSize        int
	fetchWorkerSize  int

	loader   *AddressFilterLoader
	lru      *lru.Cache
	fetchers map[persist.BlockNumber]*filterFetcher
	baseDir  string
	mu       *sync.Mutex
}

// NewBlockFilterManager returns a new instance of a BlockFilterManager.
func NewBlockFilterManager(ctx context.Context, q *sqlc.Queries, blocksPerLogFile int) *BlockFilterManager {
	var mu sync.Mutex
	baseDir, err := os.MkdirTemp("", "*")
	if err != nil {
		panic(err)
	}

	b := BlockFilterManager{
		blocksPerLogFile: blocksPerLogFile,
		chunkSize:        defaultRefreshConfig.ChunkSize,
		fetchWorkerSize:  defaultRefreshConfig.ChunkWorkerSize,
		loader: &AddressFilterLoader{
			maxBatch: defaultRefreshConfig.DataloaderDefaultMaxBatch,
			wait:     defaultRefreshConfig.DataloaderDefaultWaitTime,
			fetch:    loadBlockFilter(ctx, q),
		},
		fetchers: make(map[persist.BlockNumber]*filterFetcher),
		baseDir:  baseDir,
		mu:       &mu,
	}

	lru, err := lru.NewWithEvict(defaultRefreshConfig.CacheSize, func(key, value interface{}) {
		fetcher := value.(*filterFetcher)
		err := fetcher.deleteChunk()
		if err != nil {
			panic(err)
		}
		// Allow re-priming
		fetcher.done = nil
	})

	b.lru = lru
	return &b
}

// Get returns a filter if it exists. If the filter's chunk hasn't been loaded yet, this
// call will block until the chunk has been downloaded.
func (b *BlockFilterManager) Get(ctx context.Context, from, to persist.BlockNumber) (*bloom.BloomFilter, error) {
	chunk := from - (from % persist.BlockNumber(b.chunkSize))

	if _, ok := b.lru.Get(chunk); !ok {
		if err := b.prime(ctx, chunk); err != nil {
			return nil, err
		}
	}

	f, _ := b.lru.Get(chunk)

	return f.(*filterFetcher).loadFilter(ctx, from, to)
}

// Clear removes the filter from it's cache.
func (b *BlockFilterManager) Clear(ctx context.Context, from, to persist.BlockNumber) {
	b.loader.Clear(sqlc.GetAddressFilterBatchParams{FromBlock: from, ToBlock: to})
}

// Close deletes filters that have been loaded.
func (b *BlockFilterManager) Close() {
	os.RemoveAll(b.baseDir)
}

func (b *BlockFilterManager) prime(ctx context.Context, chunkStart persist.BlockNumber) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.fetchers[chunkStart]; !ok {
		b.fetchers[chunkStart] = newFilterFetcher(b.chunkSize, b.fetchWorkerSize, b.loader, b.baseDir)
	}

	f := b.fetchers[chunkStart]

	if f.done == nil {
		f.done = make(chan struct{})
		if err := f.loadChunk(ctx, chunkStart, b.blocksPerLogFile); err != nil {
			f.done = nil
			return err
		}
		b.lru.Add(chunkStart, f)
	}

	<-f.done
	return nil
}

// filterFetcher is an unexported type that handles the downloading of a chunk of filter objects.
type filterFetcher struct {
	chunkSize  int
	workerSize int
	loader     *AddressFilterLoader
	outDir     string
	done       chan struct{}
}

func newFilterFetcher(chunkSize, workerSize int, addressFilterLoader *AddressFilterLoader, baseDir string) *filterFetcher {
	outDir, err := os.MkdirTemp(baseDir, "*")
	if err != nil {
		panic(err)
	}
	return &filterFetcher{
		chunkSize:  chunkSize,
		workerSize: workerSize,
		loader:     addressFilterLoader,
		outDir:     outDir,
	}
}

func (f *filterFetcher) loadChunk(ctx context.Context, chunkStart persist.BlockNumber, blocksPerLogFile int) error {
	defer close(f.done)

	to := chunkStart + persist.BlockNumber(f.chunkSize)
	eg := new(errgroup.Group)

	for block := chunkStart; block < to; block += persist.BlockNumber(blocksPerLogFile) {
		filterStart := block
		filterEnd := filterStart + persist.BlockNumber(blocksPerLogFile)

		eg.Go(func() error {
			bf, err := loadFromRepo(ctx, filterStart, filterEnd, f.loader)
			if err == ErrNoFilter {
				return nil
			}
			if err != nil {
				return err
			}

			err = f.saveFilter(ctx, filterStart, filterEnd, bf)
			if err != nil {
				return err
			}

			return nil
		})
	}

	return eg.Wait()
}

func (f *filterFetcher) loadFilter(ctx context.Context, from, to persist.BlockNumber) (*bloom.BloomFilter, error) {
	return loadFromFile(f.logFileName(from, to))
}

func (f *filterFetcher) saveFilter(ctx context.Context, from, to persist.BlockNumber, bf *bloom.BloomFilter) error {
	if bf != nil {
		return saveToFile(f.logFileName(from, to), bf)
	}
	return nil
}

func (f *filterFetcher) deleteChunk() error {
	return os.RemoveAll(f.outDir)
}

func (f *filterFetcher) logFileName(from, to persist.BlockNumber) string {
	return logFileName(f.outDir, from, to)
}

func loadFromFile(path string) (*bloom.BloomFilter, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNoFilter
	}

	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return nil, err
	}

	var bf bloom.BloomFilter
	bf.ReadFrom(f)
	return &bf, nil
}

func loadFromRepo(ctx context.Context, from, to persist.BlockNumber, addressFilterLoader *AddressFilterLoader) (*bloom.BloomFilter, error) {
	data, err := addressFilterLoader.Load(sqlc.GetAddressFilterBatchParams{
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

func saveToFile(path string, bf *bloom.BloomFilter) error {
	f, err := os.Create(path)
	defer f.Close()

	if err != nil {
		return err
	}

	_, err = bf.WriteTo(f)
	return err
}

func logFileName(outDir string, from, to persist.BlockNumber) string {
	return filepath.Join(outDir, fmt.Sprintf("%s-%s", from, to))
}

func loadBlockFilter(ctx context.Context, q *sqlc.Queries) func([]sqlc.GetAddressFilterBatchParams) ([]sqlc.AddressFilter, []error) {
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
