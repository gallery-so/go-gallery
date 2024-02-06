//go:build wireinject
// +build wireinject

package server

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/wire"
	"github.com/jackc/pgx/v4/pgxpool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/indexer"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/multichain/tzkt"
	"github.com/mikeydub/go-gallery/service/multichain/zora"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
)

// envInit is a type returned after setting up the environment
// Adding envInit as a dependency to a provider will ensure that the environment is set up prior
// to calling the provider
type envInit struct{}

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
func NewMultichainProvider(ctx context.Context, envFunc func()) (*multichain.Provider, func()) {
	wire.Build(
		setEnv,
		wire.Value(&http.Client{Timeout: 0}), // HTTP client shared between providers
		newCommunitiesCache,
		postgres.NewRepositories,
		dbConnSet,
		wire.Struct(new(multichain.ChainProvider), "*"),
		multichainProviderSet,
		// Add additional chains here
		ethProviderSet,
		tezosProviderSet,
		optimismProviderSet,
		poapProviderSet,
		zoraProviderSet,
		baseProviderSet,
		polygonProviderSet,
		arbitrumProviderSet,
	)
	return nil, nil
}

// dbConnSet is a wire provider set for initializing a postgres connection
var dbConnSet = wire.NewSet(
	newPqClient,
	newPgxClient,
	newQueries,
)

func setEnv(f func()) envInit {
	f()
	return envInit{}
}

func newPqClient(e envInit) (*sql.DB, func()) {
	pq := postgres.MustCreateClient()
	return pq, func() { pq.Close() }
}

func newPgxClient(envInit) (*pgxpool.Pool, func()) {
	pgx := postgres.NewPgxClient()
	return pgx, func() { pgx.Close() }
}

func newQueries(p *pgxpool.Pool) *db.Queries {
	return db.New(p)
}

func multichainProviderSet(context.Context, *postgres.Repositories, *db.Queries, *redis.Cache, *multichain.ChainProvider) *multichain.Provider {
	wire.Build(
		wire.Struct(new(multichain.Provider), "*"),
		newSubmitBatch,
		tokenmanage.New,
		task.NewClient,
		newProviderLookup,
	)
	return nil
}

func newProviderLookup(p *multichain.ChainProvider) multichain.ProviderLookup {
	return multichain.ProviderLookup{
		persist.ChainETH:      p.Ethereum,
		persist.ChainTezos:    p.Tezos,
		persist.ChainOptimism: p.Optimism,
		persist.ChainArbitrum: p.Arbitrum,
		persist.ChainPOAP:     p.Poap,
		persist.ChainZora:     p.Zora,
		persist.ChainBase:     p.Base,
		persist.ChainPolygon:  p.Polygon,
	}
}

func ethProviderSet(envInit, *http.Client) *multichain.EthereumProvider {
	wire.Build(
		ethProvidersConfig,
		rpc.NewEthClient,
		wire.Value(persist.ChainETH),
		indexer.NewProvider,
		newOpenseaProvider,
	)
	return nil
}

func newOpenseaProvider(*http.Client, persist.Chain) *opensea.Provider {
	wire.Build(
		reservoir.NewProvider,
		opensea.NewProvider,
	)
	return nil
}

func ethProvidersConfig(
	indexerProvider *indexer.Provider,
	openseaProvider *opensea.Provider,
) *multichain.EthereumProvider {
	wire.Build(
		wire.Struct(new(multichain.EthereumProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.CustomMetadataFetcher), util.ToPointer(indexerProvider)),
	)
	return nil
}

func tezosProviderSet(envInit, *http.Client) *multichain.TezosProvider {
	wire.Build(
		tezosProvidersConfig,
		tezos.NewProvider,
		tzkt.NewProvider,
	)
	return nil
}

func tezosProvidersConfig(tezosProvider *tezos.Provider, tzktProvider *tzkt.Provider) *multichain.TezosProvider {
	wire.Build(
		wire.Struct(new(multichain.TezosProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(tzktProvider)),
	)
	return nil
}

func optimismProviderSet(*http.Client) *multichain.OptimismProvider {
	wire.Build(
		optimismProvidersConfig,
		wire.Value(persist.ChainOptimism),
		newOpenseaProvider,
	)
	return nil
}

func optimismProvidersConfig(openseaProvider *opensea.Provider) *multichain.OptimismProvider {
	wire.Build(
		wire.Struct(new(multichain.OptimismProvider), "*"),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
	)
	return nil
}

func arbitrumProviderSet(*http.Client) *multichain.ArbitrumProvider {
	wire.Build(
		arbitrumProvidersConfig,
		wire.Value(persist.ChainArbitrum),
		newOpenseaProvider,
	)
	return nil
}

func arbitrumProvidersConfig(openseaProvider *opensea.Provider) *multichain.ArbitrumProvider {
	wire.Build(
		wire.Struct(new(multichain.ArbitrumProvider), "*"),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
	)
	return nil
}

func poapProviderSet(envInit, *http.Client) *multichain.PoapProvider {
	wire.Build(
		poapProvidersConfig,
		poap.NewProvider,
	)
	return nil
}

func poapProvidersConfig(poapProvider *poap.Provider) *multichain.PoapProvider {
	wire.Build(
		wire.Struct(new(multichain.PoapProvider), "*"),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(poapProvider)),
	)
	return nil
}

func zoraProviderSet(envInit, *http.Client) *multichain.ZoraProvider {
	wire.Build(
		zoraProvidersConfig,
		wire.Value(persist.ChainZora),
		zora.NewProvider,
		newOpenseaProvider,
	)
	return nil
}

func zoraProvidersConfig(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) *multichain.ZoraProvider {
	wire.Build(
		wire.Struct(new(multichain.ZoraProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
	)
	return nil
}

func baseProviderSet(*http.Client) *multichain.BaseProvider {
	wire.Build(
		baseProvidersConfig,
		wire.Value(persist.ChainBase),
		newOpenseaProvider,
	)
	return nil
}

func baseProvidersConfig(openseaProvider *opensea.Provider) *multichain.BaseProvider {
	wire.Build(
		wire.Struct(new(multichain.BaseProvider), "*"),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
	)
	return nil
}

func polygonProviderSet(*http.Client) *multichain.PolygonProvider {
	wire.Build(
		polygonProvidersConfig,
		wire.Value(persist.ChainPolygon),
		newOpenseaProvider,
	)
	return nil
}

func polygonProvidersConfig(openseaProvider *opensea.Provider) *multichain.PolygonProvider {
	wire.Build(
		wire.Struct(new(multichain.PolygonProvider), "*"),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(openseaProvider)),
	)
	return nil
}

func newCommunitiesCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}

func newSubmitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
