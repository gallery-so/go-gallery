//go:generate go run github.com/vektah/dataloaden AddressFilterLoader github.com/mikeydub/go-gallery/db/gen/indexerdb.GetAddressFilterBatchParams github.com/mikeydub/go-gallery/db/gen/indexerdb.AddressFilter

package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom"
	lru "github.com/hashicorp/golang-lru"
	"github.com/jackc/pgx/v4"
	db "github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/service/persist"
	"golang.org/x/sync/errgroup"
)

// config configures how deep refreshes are ran.
type config struct {
	TaskSize                  int                 // The range of blocks will are chunked into tasks of this size which are run concurrently
	DefaultPoolSize           int                 // Number of workers to allocate to a refresh
	LookbackWindow            int                 // Refreshes will start this many blocks before the last indexer block processed
	ChunkSize                 int                 // The number of filters to download per chunk
	CacheSize                 int                 // The number of chunks to keep on disk at a time
	ChunkWorkerSize           int                 // The number of workers used to download a chunk
	DataloaderDefaultMaxBatch int                 // The max batch size before submitting a batch
	DataloaderDefaultWaitTime time.Duration       // Max time to wait before submitting a batch
	MaxConcurrentRuns         int                 // The number of refreshes that can run concurrently
	MinStartingBlock          persist.BlockNumber // The earliest block that can be handled
	BlocksPerCachedLog        int                 // How many blocks the indexer stores per cached log file
}

var DefaultConfig config = config{
	TaskSize:                  240000,
	DefaultPoolSize:           3,
	ChunkSize:                 10000,
	CacheSize:                 8,
	ChunkWorkerSize:           128,
	LookbackWindow:            5000000,
	DataloaderDefaultMaxBatch: 1000,
	DataloaderDefaultWaitTime: 2 * time.Millisecond,
	MaxConcurrentRuns:         24,
	MinStartingBlock:          5000000,
	BlocksPerCachedLog:        50,
}

// ErrNoFilter is returned when a filter does not exist.
var ErrNoFilter = errors.New("no filter")

// ErrInvalidRefreshRange is returned when the refresh range input is invalid.
var ErrInvalidRefreshRange = errors.New("refresh range is invalid")

// AddressExists checks if an address transacted in a block range.
func AddressExists(ctx context.Context, fm *BlockFilterManager, address persist.EthereumAddress, from, to persist.BlockNumber) (bool, error) {
	bf, err := fm.Get(ctx, from, to)
	if err != nil {
		return false, err
	}
	return bf.TestString(address.String()), nil
}

// ResolveRange standardizes the refresh input range.
func ResolveRange(r persist.BlockRange) (persist.BlockRange, error) {
	out := r
	from, to := out[0], out[1]
	if from > to {
		return out, ErrInvalidRefreshRange
	}
	if out[0] < DefaultConfig.MinStartingBlock {
		out[0] = DefaultConfig.MinStartingBlock
	}
	if out[1] < out[0] {
		out[1] = out[0]
	}
	return out, nil
}

// BlockFilterManager handles the downloading and removing of block filters.
type BlockFilterManager struct {
	blocksPerLogFile int
	chunkSize        int
	fetchWorkerSize  int
	loader           *AddressFilterLoader
	lru              *lru.Cache
	fetchers         map[persist.BlockNumber]*filterFetcher
	baseDir          string
	mu               *sync.Mutex
}

// NewBlockFilterManager returns a new instance of a BlockFilterManager.
func NewBlockFilterManager(ctx context.Context, q *db.Queries, blocksPerLogFile int) *BlockFilterManager {
	var mu sync.Mutex
	baseDir, err := os.MkdirTemp("", "*")
	if err != nil {
		panic(err)
	}

	lru, err := lru.NewWithEvict(DefaultConfig.CacheSize, func(key, value interface{}) {
		fetcher := value.(*filterFetcher)
		err := fetcher.deleteChunk()
		if err != nil {
			panic(err)
		}
	})
	if err != nil {
		panic(err)
	}

	return &BlockFilterManager{
		blocksPerLogFile: blocksPerLogFile,
		chunkSize:        DefaultConfig.ChunkSize,
		fetchWorkerSize:  DefaultConfig.ChunkWorkerSize,
		loader: &AddressFilterLoader{
			maxBatch: DefaultConfig.DataloaderDefaultMaxBatch,
			wait:     DefaultConfig.DataloaderDefaultWaitTime,
			fetch:    loadBlockFilter(ctx, q),
		},
		fetchers: make(map[persist.BlockNumber]*filterFetcher),
		baseDir:  baseDir,
		mu:       &mu,
		lru:      lru,
	}
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
	b.loader.Clear(db.GetAddressFilterBatchParams{FromBlock: from, ToBlock: to})
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

// filterFetcher is an unexported type that handles downloading a chunk of filter objects.
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
	return filepath.Join(f.outDir, fmt.Sprintf("%s-%s", from, to))
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
	data, err := addressFilterLoader.Load(db.GetAddressFilterBatchParams{
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

func loadBlockFilter(ctx context.Context, q *db.Queries) func([]db.GetAddressFilterBatchParams) ([]db.AddressFilter, []error) {
	return func(params []db.GetAddressFilterBatchParams) ([]db.AddressFilter, []error) {
		filters := make([]db.AddressFilter, len(params))
		errs := make([]error, len(params))

		b := q.GetAddressFilterBatch(ctx, params)
		defer b.Close()

		b.QueryRow(func(i int, bf db.AddressFilter, err error) {
			filters[i], errs[i] = bf, err
		})

		return filters, errs
	}
}
