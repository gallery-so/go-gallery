//go:generate go run github.com/vektah/dataloaden BlockFilterLoader github.com/mikeydub/go-gallery/db/sqlc/indexergen.GetBlockFilterBatchParams github.com/mikeydub/go-gallery/db/sqlc/indexergen.BlockFilter

package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom"
	"github.com/gammazero/workerpool"
	lru "github.com/hashicorp/golang-lru"
	"github.com/jackc/pgx"
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/indexergen"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
)

var defaultRefreshConfig RefreshConfig = RefreshConfig{
	DefaultPoolSize:           4,
	LookbackWindow:            100,
	ChunkSize:                 10000,
	CacheSize:                 10,
	ChunkWorkerSize:           32,
	DataloaderDefaultMaxBatch: 100,
	DataloaderDefaultWaitTime: 10 * time.Millisecond,
	RefreshQueueName:          "deepRefresh:addressQueue",
	RefreshLockName:           "deepRefresh:addressLock",
	MaxConcurrentRuns:         1,
	Liveness:                  15 * 60,
}

// RefreshConfig configures how deep refreshes are ran.
type RefreshConfig struct {
	DefaultPoolSize           int           // Number of workers to allocate to a refresh
	LookbackWindow            int           // Refreshes will start this many blocks before the last indexer block processed
	ChunkSize                 int           // The number of filters in a chunk
	CacheSize                 int           // The number of chunks to keep on disk
	ChunkWorkerSize           int           // The number of workers used to download a chunk
	DataloaderDefaultMaxBatch int           // The max batch size before submitting a batch
	DataloaderDefaultWaitTime time.Duration // Max time to wait before submitting a batch
	RefreshQueueName          string        // The name of the queue to buffer refreshes
	RefreshLockName           string        // The name of the lock to acquire refreshes
	MaxConcurrentRuns         int           // The number of refreshes that can run concurrently
	Liveness                  int           // How often jobs needs to refresh their locks
}

// RefreshQueue buffers deep refresh requests.
type RefreshQueue struct {
	q *redis.FifoQueue
}

// NewRefreshQueue returns a connection to the refresh queue.
func NewRefreshQueue() *RefreshQueue {
	return &RefreshQueue{redis.NewFifoQueue(redis.IndexerServerThrottleDB, defaultRefreshConfig.RefreshQueueName)}
}

// AddMessage adds a message to the queue.
func (r *RefreshQueue) AddMessage(ctx context.Context, input UpdateTokenMediaInput) error {
	message, err := json.Marshal(input)
	if err != nil {
		return err
	}
	_, err = r.q.Push(ctx, message)
	return err
}

// GetMessage blocks and pops a message from the queue.
func (r *RefreshQueue) GetMessage(ctx context.Context) (UpdateTokenMediaInput, error) {
	queued, err := r.q.Pop(ctx, 0)

	var msg UpdateTokenMediaInput

	err = json.Unmarshal([]byte(queued), &msg)
	if err != nil {
		return UpdateTokenMediaInput{}, err
	}

	return msg, nil
}

// PutbackMessage adds a message to the top of the queue.
func (r *RefreshQueue) PutbackMessage(ctx context.Context, input UpdateTokenMediaInput) error {
	message, err := json.Marshal(input)
	if err != nil {
		return err
	}
	_, err = r.q.LPush(ctx, message)
	return err
}

// RefreshLock manages the number of concurrent refreshes allowed.
type RefreshLock struct {
	s *redis.Semaphore
}

func NewRefreshLock() *RefreshLock {
	return &RefreshLock{
		s: redis.NewSemaphore(
			redis.IndexerServerThrottleDB,
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

// BlockFilterManager handles the downloading and removing of block filters.
// When a filter is requested, it attempts to load it from disk. If the filter's respective
// chunk hasn't been downloaded yet, BlockFilterManager will download the entire chunk to disk via
// a dataloader.
type BlockFilterManager struct {
	blocksPerLogFile int
	chunkSize        int
	fetchWorkerSize  int

	loader   *BlockFilterLoader
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
		loader: &BlockFilterLoader{
			maxBatch: defaultRefreshConfig.DataloaderDefaultMaxBatch,
			wait:     defaultRefreshConfig.DataloaderDefaultWaitTime,
			fetch:    loadBlockFilter(ctx, q),
		},
		fetchers: make(map[persist.BlockNumber]*filterFetcher),
		baseDir:  baseDir,
		mu:       &mu,
	}

	lru, err := lru.NewWithEvict(defaultRefreshConfig.CacheSize, func(key, value interface{}) {
		err := value.(*filterFetcher).deleteChunk()
		if err != nil {
			panic(err)
		}
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
	loader     *BlockFilterLoader
	outDir     string
	done       chan struct{}
}

func newFilterFetcher(chunkSize, workerSize int, blockFilterLoader *BlockFilterLoader, baseDir string) *filterFetcher {
	outDir, err := os.MkdirTemp(baseDir, "*")
	if err != nil {
		panic(err)
	}
	return &filterFetcher{
		chunkSize:  chunkSize,
		workerSize: workerSize,
		loader:     blockFilterLoader,
		outDir:     outDir,
	}
}

func (f *filterFetcher) loadChunk(ctx context.Context, chunkStart persist.BlockNumber, blocksPerLogFile int) error {
	errors := make(chan error)
	to := chunkStart + persist.BlockNumber(f.chunkSize)
	wp := workerpool.New(f.workerSize)

	for block := chunkStart; block < to; block += persist.BlockNumber(blocksPerLogFile) {
		filterStart := block
		filterEnd := filterStart + persist.BlockNumber(blocksPerLogFile)
		wp.Submit(func() {
			bf, err := loadFromRepo(ctx, filterStart, filterEnd, f.loader)
			if err != nil {
				errors <- err
			}

			err = f.saveFilter(ctx, filterStart, filterEnd, bf)
			if err != nil {
				errors <- err
			}
		})
	}
	wp.StopWait()
	close(errors)
	close(f.done)
	return <-errors
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
		return nil, nil
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

func loadFromRepo(ctx context.Context, from, to persist.BlockNumber, blockFilterLoader *BlockFilterLoader) (*bloom.BloomFilter, error) {
	data, err := blockFilterLoader.Load(sqlc.GetBlockFilterBatchParams{
		FromBlock: from,
		ToBlock:   to,
	})
	if err != nil && err.Error() == pgx.ErrNoRows.Error() {
		return nil, nil
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

func loadBlockFilter(ctx context.Context, q *sqlc.Queries) func([]sqlc.GetBlockFilterBatchParams) ([]sqlc.BlockFilter, []error) {
	return func(params []sqlc.GetBlockFilterBatchParams) ([]sqlc.BlockFilter, []error) {
		filters := make([]sqlc.BlockFilter, len(params))
		errs := make([]error, len(params))

		b := q.GetBlockFilterBatch(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, bf sqlc.BlockFilter, err error) {
			filters[i], errs[i] = bf, err
		})

		return filters, errs
	}
}
