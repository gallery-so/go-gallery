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
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/alchemy"
	"github.com/mikeydub/go-gallery/service/multichain/indexer"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/multichain/tzkt"
	"github.com/mikeydub/go-gallery/service/multichain/wrapper"
	"github.com/mikeydub/go-gallery/service/multichain/zora"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/rpc/arweave"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
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
		multichainProviderInjector,
		ethInjector,
		tezosInjector,
		optimismInjector,
		poapInjector,
		zoraInjector,
		baseInjector,
		polygonInjector,
		arbitrumInjector,
	)
	return nil, nil
}

// customMetadataHandlerSet is a wire provider set for initializing custom metadata handlers
var customMetadataHandlerSet = wire.NewSet(
	rpc.NewEthClient,
	ipfs.NewShell,
	arweave.NewClient,
	media.NewCustomMetadataHandlers,
)

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

func multichainProviderInjector(context.Context, *postgres.Repositories, *db.Queries, *redis.Cache, *multichain.ChainProvider) *multichain.Provider {
	wire.Build(
		wire.Struct(new(multichain.Provider), "*"),
		newSubmitBatch,
		tokenmanage.New,
		task.NewClient,
		newProviderLookup,
		customMetadataHandlerSet,
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

type (
	contractFetcherA                  multichain.ContractFetcher
	contractFetcherB                  multichain.ContractFetcher
	tokenMetadataFetcherA             multichain.TokenMetadataFetcher
	tokenMetadataFetcherB             multichain.TokenMetadataFetcher
	tokenDescriptorsFetcherA          multichain.TokenDescriptorsFetcher
	tokenDescriptorsFetcherB          multichain.TokenDescriptorsFetcher
	tokenIdentifierOwnerFetcherA      multichain.TokenIdentifierOwnerFetcher
	tokenIdentifierOwnerFetcherB      multichain.TokenIdentifierOwnerFetcher
	tokensIncrementalOwnerFetcherA    multichain.TokensIncrementalOwnerFetcher
	tokensIncrementalOwnerFetcherB    multichain.TokensIncrementalOwnerFetcher
	tokensIncrementalContractFetcherA multichain.TokensIncrementalContractFetcher
	tokensIncrementalContractFetcherB multichain.TokensIncrementalContractFetcher
)

func multiContractFetcherProvider(a contractFetcherA, b contractFetcherB) multichain.ContractFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithContractFetchers(a, b))
}

func multiTokenMetadataFetcherProvider(a tokenMetadataFetcherA, b tokenMetadataFetcherB) multichain.TokenMetadataFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokenMetadataFetchers(a, b))
}

func multiTokenDescriptorsFetcherProvider(a tokenDescriptorsFetcherA, b tokenDescriptorsFetcherB) multichain.TokenDescriptorsFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokenDescriptorsFetchers(a, b))
}

func multiTokenIdentifierOwnerFetcherProvider(a tokenIdentifierOwnerFetcherA, b tokenIdentifierOwnerFetcherB) multichain.TokenIdentifierOwnerFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokenIdentifierOwnerFetchers(a, b))
}

func multiTokensIncrementalOwnerFetcherProvider(a tokensIncrementalOwnerFetcherA, b tokensIncrementalOwnerFetcherB) multichain.TokensIncrementalOwnerFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokensIncrementalOwnerFetchers(a, b))
}

func multiTokensIncrementalContractFetcherProvider(a tokensIncrementalContractFetcherA, b tokensIncrementalContractFetcherB) multichain.TokensIncrementalContractFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokensIncrementalContractFetchers(a, b))
}

func ethInjector(envInit, context.Context, *http.Client) *multichain.EthereumProvider {
	panic(wire.Build(
		rpc.NewEthClient,
		wire.Value(persist.ChainETH),
		indexer.NewProvider,
		alchemy.NewProvider,
		opensea.NewProvider,
		ethProviderInjector,
		ethSyncPipelineInjector,
		ethContractFetcherInjector,
		ethTokenMetadataFetcherInjector,
		ethTokenDescriptorsFetcherInjector,
	))
}

func ethProviderInjector(
	ctx context.Context,
	indexerProvider *indexer.Provider,
	syncPipeline *wrapper.SyncPipelineWrapper,
	contractFetcher multichain.ContractFetcher,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.EthereumProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.EthereumProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func ethSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		ethTokenIdentifierOwnerFetcherInjector,
		ethTokensIncrementalOwnerFetcherInjector,
		ethTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func ethTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func ethTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func ethTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func ethContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.ContractFetcher {
	panic(wire.Build(
		multiContractFetcherProvider,
		wire.Bind(new(contractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(contractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func ethTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func ethTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func tezosInjector(envInit, *http.Client) *multichain.TezosProvider {
	wire.Build(
		tezosProviderInjector,
		tezos.NewProvider,
		tzkt.NewProvider,
	)
	return nil
}

func tezosProviderInjector(tezosProvider *tezos.Provider, tzktProvider *tzkt.Provider) *multichain.TezosProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.TezosProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(tzktProvider)),
	))
}

func optimismInjector(context.Context, *http.Client) *multichain.OptimismProvider {
	panic(wire.Build(
		wire.Value(persist.ChainOptimism),
		optimismProviderInjector,
		opensea.NewProvider,
		alchemy.NewProvider,
		optimismSyncPipelineInjector,
		optimisimTokenDescriptorsFetcherInjector,
		optimismTokenMetadataFetcherInjector,
	))
}

func optimismProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.OptimismProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.OptimismProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func optimismSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		optimismTokenIdentifierOwnerFetcherInjector,
		optimismTokensIncrementalOwnerFetcherInjector,
		optimismTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func optimismTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func optimismTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func optimismTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func optimismTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func optimisimTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func arbitrumInjector(context.Context, *http.Client) *multichain.ArbitrumProvider {
	panic(wire.Build(
		arbitrumProviderInjector,
		wire.Value(persist.ChainArbitrum),
		opensea.NewProvider,
		alchemy.NewProvider,
		arbitrumSyncPipelineInjector,
		arbitrumTokenDescriptorsFetcherInjector,
		arbitrumTokenMetadataFetcherInjector,
	))
}

func arbitrumProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.ArbitrumProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.ArbitrumProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func arbitrumSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		arbitrumTokenIdentifierOwnerFetcherInjector,
		arbitrumTokensIncrementalOwnerFetcherInjector,
		arbitrumTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func arbitrumTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func arbitrumTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func arbitrumTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func arbitrumTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func arbitrumTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func poapInjector(envInit, *http.Client) *multichain.PoapProvider {
	panic(wire.Build(
		poapProviderInjector,
		poap.NewProvider,
	))
}

func poapProviderInjector(poapProvider *poap.Provider) *multichain.PoapProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.PoapProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(poapProvider)),
	))
}

func zoraInjector(envInit, context.Context, *http.Client) *multichain.ZoraProvider {
	panic(wire.Build(
		zoraProviderInjector,
		wire.Value(persist.ChainZora),
		zora.NewProvider,
		opensea.NewProvider,
		zoraSyncPipelineInjector,
		zoraContractFetcherInjector,
		zoraTokenDescriptorsFetcherInjector,
		zoraTokenMetadataFetcherInjector,
	))
}

func zoraProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	zoraProvider *zora.Provider,
	contractFetcher multichain.ContractFetcher,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.ZoraProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.ZoraProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func zoraTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(zoraProvider)),
	))
}

func zoraTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(zoraProvider)),
	))
}

func zoraContractFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.ContractFetcher {
	panic(wire.Build(
		multiContractFetcherProvider,
		wire.Bind(new(contractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(contractFetcherB), util.ToPointer(zoraProvider)),
	))
}

func zoraSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, zoraProvider *zora.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(zoraProvider)),
		zoraTokenIdentifierOwnerFetcherInjector,
		zoraTokensIncrementalOwnerFetcherInjector,
		zoraTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func zoraTokensContractFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(zoraProvider)),
	))
}

func zoraTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(zoraProvider)),
	))
}

func zoraTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(zoraProvider)),
	))
}

func baseInjector(context.Context, *http.Client) *multichain.BaseProvider {
	panic(wire.Build(
		baseProvidersInjector,
		wire.Value(persist.ChainBase),
		opensea.NewProvider,
		alchemy.NewProvider,
		baseSyncPipelineInjector,
		baseTokenDescriptorFetcherInjector,
		baseTokenMetadataFetcherInjector,
	))
}

func baseProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.BaseProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.BaseProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func baseSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		baseTokenIdentifierOwnerFetcherInjector,
		baseTokensIncrementalOwnerFetcherInjector,
		baseTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func baseTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func baseTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func baseTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func baseTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func baseTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonInjector(context.Context, *http.Client) *multichain.PolygonProvider {
	panic(wire.Build(
		polygonProvidersInjector,
		wire.Value(persist.ChainPolygon),
		opensea.NewProvider,
		alchemy.NewProvider,
		polygonSyncPipelineInjector,
		polygonTokenDescriptorFetcherInjector,
		polygonTokenMetadataFetcherInjector,
	))
}

func polygonSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		polygonTokenIdentifierOwnerFetcherInjector,
		polygonTokensIncrementalOwnerFetcherInjector,
		polygonTokensContractFetcherInjector,
		wrapper.NewFillInWrapper,
	))
}

func polygonTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(alchemyProvider)),
	))
}

func polygonProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.PolygonProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.PolygonProvider), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
	))
}

func newCommunitiesCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}

func newSubmitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
