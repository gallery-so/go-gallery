package indexer

import (
	"context"

	"github.com/mikeydub/go-gallery/service/persist"
)

type DBHook[T any] func(ctx context.Context, it []T) error

func newContractHooks(repo persist.ContractRepository) []DBHook[persist.Contract] {
	return []DBHook[persist.Contract]{
		func(ctx context.Context, it []persist.Contract) error {
			return repo.BulkUpsert(ctx, it)
		},
	}
}

func newTokenHooks() []DBHook[persist.Token] {
	return []DBHook[persist.Token]{}
}
