package site

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/spf13/viper"
)

const transferHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

// Figure31Integration manages the Figure31 site event.
type Figure31Integration struct {
	UserID         persist.DBID
	CollectionID   persist.DBID
	ContractAddr   common.Address
	ArtistAddr     common.Address
	ColumnCount    int
	CollectionSize int

	logs chan types.Log
	l    *dataloader.Loaders
	p    *multichain.Provider
	r    *persist.Repositories
	q    *pgxpool.Pool
	e    *ethclient.Client
}

// Figure31IntegrationInput contains the input params to configure the Figure31 integration.
type Figure31IntegrationInput struct {
	UserID         persist.DBID
	CollectionID   persist.DBID
	ContractAddr   string
	ArtistAddr     string
	ColumnCount    int
	CollectionSize int
}

// NewFigure31Integration returns a new Figure31 site integration
func NewFigure31Integration(loaders *dataloader.Loaders, provider *multichain.Provider, repos *persist.Repositories, pgx *pgxpool.Pool, input Figure31IntegrationInput) *Figure31Integration {
	ethClient, err := ethclient.Dial(viper.GetString("RPC_URL"))

	if err != nil {
		panic(err)
	}

	return &Figure31Integration{
		UserID:         input.UserID,
		CollectionID:   input.CollectionID,
		ContractAddr:   common.HexToAddress(input.ContractAddr),
		ArtistAddr:     common.HexToAddress(input.ArtistAddr),
		ColumnCount:    input.ColumnCount,
		CollectionSize: input.CollectionSize,
		logs:           make(chan types.Log),
		l:              loaders,
		p:              provider,
		r:              repos,
		q:              pgx,
		e:              ethClient,
	}
}

// Start listens for transfer events from the project's contracts and syncs the target collection.
func (i *Figure31Integration) Start(ctx context.Context) {
	logger.For(ctx).Info("starting Figure31 integration")

	query := ethereum.FilterQuery{Addresses: []common.Address{i.ContractAddr}, Topics: [][]common.Hash{
		{common.HexToHash(transferHash)}},
	}

	subscription, err := i.e.SubscribeFilterLogs(ctx, query, i.logs)
	if err != nil {
		panic(err)
	}
	defer subscription.Unsubscribe()

	go i.Schedule(ctx) // Run sync at fixed intervals

	for {
		<-i.logs
		<-time.After(2 * time.Minute) // Give providers a chance to catch up

		err = i.SyncCollection(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}

		err := i.AddToEarlyAccess(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}
	}
}

// Schedule runs the sync routine at fixed intervals so that the wallet is kept up to date. The indexers
// are often a few blocks behind the latest block, meaning the latest transfer isn't accounted for yet.
func (i *Figure31Integration) Schedule(ctx context.Context) {
	for {
		<-time.After(5 * time.Minute)

		err := i.SyncCollection(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}

		err = i.AddToEarlyAccess(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}
	}

}

// SyncCollection syncs the user's wallet, and updates the collection.
func (i *Figure31Integration) SyncCollection(ctx context.Context) error {
	err := i.p.SyncTokens(ctx, i.UserID)
	if err != nil {
		return err
	}

	tokens, err := i.l.TokensByCollectionID.Load(i.CollectionID)
	if err != nil {
		return err
	}

	tokenMap := make([]persist.DBID, i.CollectionSize)
	for _, token := range tokens {
		mintID, err := strconv.ParseInt(token.TokenID.String, 16, 32)
		if err != nil {
			return err
		}
		tokenMap[mintID-1] = token.ID
	}

	collectionTokens := make([]persist.DBID, 0)
	whitespace := make([]int, 0)
	transferPtr := 0

	for _, tokenID := range tokenMap {
		switch tokenID {
		case "":
			whitespace = append(whitespace, transferPtr)
		default:
			collectionTokens = append(collectionTokens, tokenID)
			transferPtr++
		}
	}

	return i.r.CollectionRepository.UpdateTokens(ctx, i.CollectionID, i.UserID, persist.CollectionUpdateTokensInput{
		LastUpdated: persist.LastUpdatedTime(time.Now()),
		Tokens:      collectionTokens,
		Layout:      persist.TokenLayout{Columns: persist.NullInt32(i.ColumnCount), Whitespace: whitespace},
	})
}

// AddToEarlyAccess only adds addresses that received tokens transferred from the artist's wallet.
// Every wallet is added each time in case an event was missed when the server wasn't available.
func (i *Figure31Integration) AddToEarlyAccess(ctx context.Context) error {
	query := ethereum.FilterQuery{Addresses: []common.Address{i.ContractAddr}, Topics: [][]common.Hash{
		{common.HexToHash(transferHash)}, {i.ArtistAddr.Hash()}},
	}

	logs, err := i.e.FilterLogs(ctx, query)
	if err != nil {
		return err
	}

	addresses := make([]string, 0)

	for _, log := range logs {
		if !log.Removed {
			toAddr := common.HexToAddress(log.Topics[2].Hex())
			addresses = append(addresses, strings.ToLower(toAddr.Hex()))
		}
	}

	if len(addresses) > 0 {
		insertQuery := "INSERT INTO early_access (address) SELECT unnest($1::TEXT[]) ON CONFLICT DO NOTHING;"
		_, err = i.q.Exec(ctx, insertQuery, pq.Array(addresses))
		if err != nil {
			return err
		}
	}

	return nil
}
