package postgres

import (
	"context"

	"github.com/bits-and-blooms/bloom"
	db "github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AddressFilterRepository struct {
	Queries *db.Queries
}

func (r *AddressFilterRepository) Add(ctx context.Context, from, to persist.BlockNumber, bf *bloom.BloomFilter) error {
	data, err := bf.MarshalJSON()
	if err != nil {
		return err
	}
	return r.Queries.AddAddressFilter(ctx, db.AddAddressFilterParams{
		ID:          persist.GenerateID(),
		FromBlock:   from,
		ToBlock:     to,
		BloomFilter: data,
	})
}

func (r *AddressFilterRepository) BulkUpsert(ctx context.Context, filters map[persist.BlockRange]*bloom.BloomFilter) error {
	ids := make([]string, len(filters))
	fromBlocks := make([]int64, len(filters))
	toBlocks := make([]int64, len(filters))
	bfs := make([][]byte, len(filters))

	var i int
	for key, bf := range filters {
		data, err := bf.MarshalJSON()
		if err != nil {
			return err
		}

		fromBlock, toBlock := key[0], key[1]

		ids[i] = persist.GenerateID().String()
		fromBlocks[i] = fromBlock.BigInt().Int64()
		toBlocks[i] = toBlock.BigInt().Int64()
		bfs[i] = data

		i++
	}

	return r.Queries.BulkUpsertAddressFilters(ctx, db.BulkUpsertAddressFiltersParams{
		ID:          ids,
		FromBlock:   fromBlocks,
		ToBlock:     toBlocks,
		BloomFilter: bfs,
	})
}
