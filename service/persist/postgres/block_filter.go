package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bits-and-blooms/bloom"
	"github.com/gammazero/workerpool"
	lru "github.com/hashicorp/golang-lru"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/db/sqlc"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/persist"
)

const (
	cacheSize         = 10
	fetcherWorkerSize = 100
	chunkSize         = 10000
)

type BlockFilterRepository struct {
	Queries *sqlc.Queries
}

func (r *BlockFilterRepository) Add(ctx context.Context, from, to persist.BlockNumber, filter *bloom.BloomFilter) error {
	data, err := filter.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = r.Queries.CreateBlockFilter(ctx, sqlc.CreateBlockFilterParams{
		ID:          persist.GenerateID(),
		FromBlock:   from,
		ToBlock:     to,
		BloomFilter: data,
	})
	return err
}

type BlockFilterManager struct {
	blocksPerLogFile int
	loaders          *dataloader.Loaders

	lru      *lru.Cache
	fetchers map[persist.BlockNumber]*filterFetcher
	outDir   string
	mu       *sync.Mutex
}

func NewBlockFilterManager(loaders *dataloader.Loaders, blocksPerLogFile int) *BlockFilterManager {
	var mu sync.Mutex
	outDir, err := os.MkdirTemp("", "*")
	if err != nil {
		panic(err)
	}

	b := BlockFilterManager{
		loaders:          loaders,
		blocksPerLogFile: blocksPerLogFile,
		fetchers:         make(map[persist.BlockNumber]*filterFetcher),
		outDir:           outDir,
		mu:               &mu,
	}

	lru, err := lru.NewWithEvict(cacheSize, func(key, value interface{}) {
		err := value.(*filterFetcher).DeleteChunk()
		if err != nil {
			panic(err)
		}
	})

	b.lru = lru
	return &b
}

func (b *BlockFilterManager) Get(ctx context.Context, from, to persist.BlockNumber) (*bloom.BloomFilter, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chunk := from - (from % persist.BlockNumber(chunkSize))

	// Check if the chunk is loaded
	if f, ok := b.lru.Get(chunk); ok {
		bf, err := f.(*filterFetcher).LoadFilter(from, to)
		if err != nil {
			bf, err = loadFromRepo(ctx, from, to, b.loaders)
			if err != nil {
				return nil, err
			}

			if bf != nil {
				saveToFile(f.(*filterFetcher).LogFileName(from, to), bf)
			}

			return bf, nil
		}
		return bf, nil
	}

	if err := b.prime(ctx, chunk); err != nil {
		bf, err := loadFromRepo(ctx, from, to, b.loaders)
		if err != nil {
			return nil, err
		}
		saveToFile(b.fetchers[chunk].LogFileName(from, to), bf)
		return bf, nil
	}

	f, _ := b.lru.Get(chunk)
	return f.(*filterFetcher).LoadFilter(from, to)
}

func (b *BlockFilterManager) prime(ctx context.Context, chunkStart persist.BlockNumber) error {
	if _, ok := b.fetchers[chunkStart]; !ok {
		b.fetchers[chunkStart] = newFilterFetcher(b.loaders, b.outDir, chunkStart)
	}

	f := b.fetchers[chunkStart]

	if f.done == nil {
		f.mu.Lock()
		defer f.mu.Unlock()

		f.done = make(chan error)
		f.LoadChunk(ctx, chunkStart, b.blocksPerLogFile)

		if err := <-f.done; err != nil {
			f.done = nil
			return err
		}

		b.lru.Add(chunkStart, f)
		return nil
	}

	<-f.done
	return nil
}

func (b *BlockFilterManager) Close() {
	os.RemoveAll(b.outDir)
}

type filterFetcher struct {
	mu      *sync.Mutex
	loaders *dataloader.Loaders
	outDir  string
	done    chan error
	chunk   persist.BlockNumber
}

func newFilterFetcher(loaders *dataloader.Loaders, baseDir string, chunk persist.BlockNumber) *filterFetcher {
	var mu sync.Mutex

	outDir, err := os.MkdirTemp(baseDir, "*")
	if err != nil {
		panic(err)
	}

	return &filterFetcher{
		mu:      &mu,
		loaders: loaders,
		outDir:  outDir,
		chunk:   chunk,
	}
}

func (f *filterFetcher) LoadChunk(ctx context.Context, chunkStart persist.BlockNumber, blocksPerLogFile int) {
	defer close(f.done)

	to := chunkStart + persist.BlockNumber(chunkSize)

	wp := workerpool.New(fetcherWorkerSize)

	for block := chunkStart; block < to; block += persist.BlockNumber(blocksPerLogFile) {
		b := block
		wp.Submit(func() {
			bf, err := loadFromRepo(ctx, b, b+persist.BlockNumber(blocksPerLogFile), f.loaders)
			if err != nil {
				f.done <- err
			}

			if bf != nil {
				err := saveToFile(f.LogFileName(b, b+persist.BlockNumber(blocksPerLogFile)), bf)
				if err != nil {
					f.done <- err
				}
			}
		})
	}

	wp.StopWait()
}

func (f *filterFetcher) LoadFilter(from, to persist.BlockNumber) (*bloom.BloomFilter, error) {
	return loadFromFile(f.LogFileName(from, to))
}

func (f *filterFetcher) LogFileName(from, to persist.BlockNumber) string {
	return logFileName(f.outDir, from, to)
}

func (f *filterFetcher) DeleteChunk() error {
	return os.RemoveAll(f.outDir)
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

func loadFromRepo(ctx context.Context, from, to persist.BlockNumber, loaders *dataloader.Loaders) (*bloom.BloomFilter, error) {
	data, err := loaders.BlockFilter.Load(sqlc.GetBlockFilterBatchParams{
		FromBlock: from,
		ToBlock:   to,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var bf bloom.BloomFilter
	err = bf.UnmarshalJSON(data.BloomFilter)
	if err != nil {
		panic(err)
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
