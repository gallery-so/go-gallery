package indexer

import (
	"context"
	"net/http"
	"sync"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sourcegraph/conc/pool"
)

type DBHook[T any] func(ctx context.Context, it []T, statsID persist.DBID) error

func newContractHooks(queries *indexerdb.Queries, repo persist.ContractRepository, ethClient *ethclient.Client, httpClient *http.Client, ownerStats *sync.Map) []DBHook[persist.Contract] {
	return []DBHook[persist.Contract]{
		func(ctx context.Context, it []persist.Contract, statsID persist.DBID) error {
			upChan := make(chan []persist.Contract)
			go fillContractFields(ctx, it, queries, repo, httpClient, ethClient, ownerStats, upChan, statsID)
			p := pool.New().WithErrors().WithContext(ctx).WithMaxGoroutines(10)
			for up := range upChan {
				up := up
				p.Go(func(ctx context.Context) error {
					logger.For(ctx).Info("bulk upserting contracts")
					if err := repo.BulkUpsert(ctx, up); err != nil {
						return err
					}
					return nil
				})
			}
			return p.Wait()
		},
	}
}

func newTokenHooks() []DBHook[persist.Token] {
	return []DBHook[persist.Token]{}
}
