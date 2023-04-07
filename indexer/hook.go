package indexer

import (
	"context"
	"net/http"

	"github.com/mikeydub/go-gallery/service/persist"
)

type DBHook[T any] func(ctx context.Context, it []T) error

func newContractHooks(repo persist.ContractRepository, httpClient *http.Client) []DBHook[persist.Contract] {
	return []DBHook[persist.Contract]{
		func(ctx context.Context, it []persist.Contract) error {
			return repo.BulkUpsert(ctx, fillContractFields(ctx, it, repo, httpClient))
		},
	}
}

func newTokenHooks() []DBHook[persist.Token] {
	return []DBHook[persist.Token]{}
}
