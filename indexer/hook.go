package indexer

import (
	"context"
	"net/http"
	"sync"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
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

func newTokenHooks(tasks *gcptasks.Client, bQueries *coredb.Queries) []DBHook[persist.Token] {
	return []DBHook[persist.Token]{
		func(ctx context.Context, it []persist.Token, statsID persist.DBID) error {
			wallets, _ := util.Map(it, func(t persist.Token) (string, error) {
				return t.OwnerAddress.String(), nil
			})
			chains, _ := util.Map(it, func(t persist.Token) (int32, error) {
				return int32(t.Chain), nil
			})
			users, err := bQueries.GetUsersForWallets(ctx, coredb.GetUsersForWalletsParams{
				WalletAddresses: wallets,
				Chains:          chains,
			})
			if err != nil {
				return err
			}

			addressToUser := make(map[persist.ChainAddress]persist.DBID)
			for _, u := range users {
				addressToUser[persist.NewChainAddress(u.Wallet.Address, u.Wallet.Chain)] = u.User.ID
			}

			tokensForUser := make(map[persist.DBID][]persist.TokenUniqueIdentifiers)
			for _, t := range it {
				if u, ok := addressToUser[persist.NewChainAddress(persist.Address(t.OwnerAddress), t.Chain)]; ok {
					tokensForUser[u] = append(tokensForUser[u], persist.TokenUniqueIdentifiers{
						Chain:           t.Chain,
						ContractAddress: persist.Address(t.ContractAddress),
						TokenID:         t.TokenID,
						OwnerAddress:    persist.Address(t.OwnerAddress),
					})
				}
			}

			for userID, tids := range tokensForUser {
				err = task.CreateTaskForUserTokenProcessing(ctx, task.TokenProcessingUserTokensMessage{
					UserID:           userID,
					TokenIdentifiers: tids,
				}, tasks)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}
