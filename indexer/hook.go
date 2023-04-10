package indexer

import (
	"context"
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/persist"
)

type DBHook[T any] func(ctx context.Context, it []T) error

func newContractHooks(repo persist.ContractRepository, ethClient *ethclient.Client, httpClient *http.Client, ownerStats *sync.Map) []DBHook[persist.Contract] {
	return []DBHook[persist.Contract]{
		func(ctx context.Context, it []persist.Contract) error {
			return repo.BulkUpsert(ctx, fillContractFields(ctx, it, repo, httpClient, ethClient, ownerStats))
		},
	}
}

func newTokenHooks() []DBHook[persist.Token] {
	return []DBHook[persist.Token]{}
}
