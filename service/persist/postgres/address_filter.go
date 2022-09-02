package postgres

import (
	"context"

	"github.com/bits-and-blooms/bloom"
	sqlc "github.com/mikeydub/go-gallery/db/sqlc/indexergen"
	"github.com/mikeydub/go-gallery/service/persist"
)

type AddressFilterRepository struct {
	Queries *sqlc.Queries
}

func (r *AddressFilterRepository) BulkUpsert(ctx context.Context, filters map[[2]persist.BlockNumber]*bloom.BloomFilter) error {
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

	return r.Queries.BulkUpsertAddressFilters(ctx, sqlc.BulkUpsertAddressFiltersParams{
		ID:          ids,
		FromBlock:   fromBlocks,
		ToBlock:     toBlocks,
		BloomFilter: bfs,
	})
}
