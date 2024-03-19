//go:build wireinject
// +build wireinject

package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/google/wire"
	"github.com/jackc/pgx/v4/pgxpool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/alchemy"
	"github.com/mikeydub/go-gallery/service/multichain/indexer"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/simplehash"
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
	"github.com/mikeydub/go-gallery/util/retry"
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
		newTokenManageCache,
		postgres.NewRepositories,
		dbConnSet,
		newOpenseaLimiter,   // needs to be a singleton
		newReservoirLimiter, // needs to be a singleton
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

type openseaLimiter limiters.KeyRateLimiter

// Dumb forward method to satisfy the retry.Limiter interface
func (o *openseaLimiter) ForKey(ctx context.Context, key string) (bool, time.Duration, error) {
	return (*limiters.KeyRateLimiter)(o).ForKey(ctx, key)
}

type reservoirLimiter limiters.KeyRateLimiter

// Dumb forward method to satisfy the retry.Limiter interface
func (r *reservoirLimiter) ForKey(ctx context.Context, key string) (bool, time.Duration, error) {
	return (*limiters.KeyRateLimiter)(r).ForKey(ctx, key)
}

func newOpenseaLimiter(ctx context.Context, c *redis.Cache) *openseaLimiter {
	l := limiters.NewKeyRateLimiter(ctx, c, "retryer:opensea", 300, time.Minute)
	return util.ToPointer(openseaLimiter(*l))
}

func newReservoirLimiter(ctx context.Context, c *redis.Cache) *reservoirLimiter {
	l := limiters.NewKeyRateLimiter(ctx, c, "retryer:reservoir", 120, time.Minute)
	return util.ToPointer(reservoirLimiter(*l))
}

func openseaProviderInjector(ctx context.Context, c *http.Client, chain persist.Chain, l *openseaLimiter) (*opensea.Provider, func()) {
	panic(wire.Build(
		opensea.NewProvider,
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
	))
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
	panic(wire.Build(
		wire.Struct(new(multichain.Provider), "*"),
		submitTokenBatchInjector,
		newProviderLookup,
	))
}

// New chains must be added here
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

// This is a workaround for wire because wire wouldn't know which value to inject for args of the same type
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
	tokensByTokenIdentifiersFetcherA  multichain.TokensByTokenIdentifiersFetcher
	tokensByTokenIdentifiersFetcherB  multichain.TokensByTokenIdentifiersFetcher
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

func multiTokenByTokenIdentifiersFetcherProvider(a tokensByTokenIdentifiersFetcherA, b tokensByTokenIdentifiersFetcherB) multichain.TokensByTokenIdentifiersFetcher {
	return wrapper.NewMultiProviderWrapper(wrapper.MultiProviderWapperOptions.WithTokenByTokenIdentifiersFetchers(a, b))
}

func customMetadataHandlersInjector(alchemyProvider *alchemy.Provider) *multichain.CustomMetadataHandlers {
	panic(wire.Build(
		multichain.NewCustomMetadataHandlers,
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(alchemyProvider)),
		rpc.NewEthClient,
		ipfs.NewShell,
		arweave.NewClient,
	))
}

func ethInjector(envInit, context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.EthereumProvider, func()) {
	panic(wire.Build(
		rpc.NewEthClient,
		wire.Value(persist.ChainETH),
		indexer.NewProvider,
		simplehash.NewProvider,
		// alchemy.NewProvider,
		// openseaProviderInjector,
		ethProviderInjector,
		// ethSyncPipelineInjector,
		// ethContractFetcherInjector,
		// ethTokenMetadataFetcherInjector,
		// ethTokenDescriptorsFetcherInjector,
	))
}

func ethProviderInjector(
	ctx context.Context,
	indexerProvider *indexer.Provider,
	simplehashProvider *simplehash.Provider,
	// syncPipeline *wrapper.SyncPipelineWrapper,
	// contractFetcher multichain.ContractFetcher,
	// tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher,
	// tokenMetadataFetcher multichain.TokenMetadataFetcher,
) *multichain.EthereumProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.EthereumProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(simplehashProvider)),
	))
}

func ethSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	alchemyProvider *alchemy.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		ethTokenIdentifierOwnerFetcherInjector,
		ethTokensIncrementalOwnerFetcherInjector,
		ethTokensContractFetcherInjector,
		ethTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		customMetadataHandlersInjector,
	))
}

func ethTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.ContractFetcher {
	panic(wire.Build(
		multiContractFetcherProvider,
		wire.Bind(new(contractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(contractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(openseaProvider)),
	))
}

func ethTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(openseaProvider)),
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

func optimismInjector(context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.OptimismProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainOptimism),
		optimismProviderInjector,
		openseaProviderInjector,
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
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
	))
}

func optimismSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	alchemyProvider *alchemy.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		optimismTokenIdentifierOwnerFetcherInjector,
		optimismTokensIncrementalOwnerFetcherInjector,
		optimismTokensContractFetcherInjector,
		optmismTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		customMetadataHandlersInjector,
	))
}

func optimismTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func optimismTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func optimismTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func optimismTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(openseaProvider)),
	))
}

func optimisimTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(openseaProvider)),
	))
}

func optmismTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumInjector(context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.ArbitrumProvider, func()) {
	panic(wire.Build(
		arbitrumProviderInjector,
		wire.Value(persist.ChainArbitrum),
		openseaProviderInjector,
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
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
	))
}

func arbitrumSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	alchemyProvider *alchemy.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		arbitrumTokenIdentifierOwnerFetcherInjector,
		arbitrumTokensIncrementalOwnerFetcherInjector,
		arbitrumTokensContractFetcherInjector,
		arbitrumTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		customMetadataHandlersInjector,
	))
}

func arbitrumTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func arbitrumTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(openseaProvider)),
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

func zoraInjector(envInit, context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.ZoraProvider, func()) {
	panic(wire.Build(
		zoraProviderInjector,
		wire.Value(persist.ChainZora),
		zora.NewProvider,
		openseaProviderInjector,
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
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
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

func zoraSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	zoraProvider *zora.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		zoraTokenIdentifierOwnerFetcherInjector,
		zoraTokensIncrementalOwnerFetcherInjector,
		zoraTokensContractFetcherInjector,
		zoraTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		zoraCustomMetadataHandlersInjector,
	))
}

func zoraCustomMetadataHandlersInjector(openseaProvider *opensea.Provider) *multichain.CustomMetadataHandlers {
	panic(wire.Build(
		multichain.NewCustomMetadataHandlers,
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(openseaProvider)),
		rpc.NewEthClient,
		ipfs.NewShell,
		arweave.NewClient,
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

func zoraTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(openseaProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(zoraProvider)),
	))
}

func baseInjector(context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.BaseProvider, func()) {
	panic(wire.Build(
		baseProvidersInjector,
		wire.Value(persist.ChainBase),
		openseaProviderInjector,
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
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
	))
}

func baseSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	alchemyProvider *alchemy.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		baseTokenIdentifierOwnerFetcherInjector,
		baseTokensIncrementalOwnerFetcherInjector,
		baseTokensContractFetcherInjector,
		baseTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		customMetadataHandlersInjector,
	))
}

func baseTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(openseaProvider)),
	))
}

func baseTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(openseaProvider)),
	))
}

func baseTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func baseTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func baseTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func baseTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(openseaProvider)),
	))
}

func polygonInjector(context.Context, *http.Client, *openseaLimiter, *reservoirLimiter) (*multichain.PolygonProvider, func()) {
	panic(wire.Build(
		polygonProvidersInjector,
		wire.Value(persist.ChainPolygon),
		openseaProviderInjector,
		alchemy.NewProvider,
		polygonSyncPipelineInjector,
		polygonTokenDescriptorFetcherInjector,
		polygonTokenMetadataFetcherInjector,
	))
}

func polygonSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	openseaProvider *opensea.Provider,
	alchemyProvider *alchemy.Provider,
	l *reservoirLimiter,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(retry.Limiter), util.ToPointer(l)),
		polygonTokenIdentifierOwnerFetcherInjector,
		polygonTokensIncrementalOwnerFetcherInjector,
		polygonTokensContractFetcherInjector,
		polygonTokenByTokenIdentifiersFetcherInjector,
		wrapper.NewFillInWrapper,
		customMetadataHandlersInjector,
	))
}

func polygonTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	panic(wire.Build(
		multiTokenMetadataFetcherProvider,
		wire.Bind(new(tokenMetadataFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenMetadataFetcherB), util.ToPointer(openseaProvider)),
	))
}

func polygonTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	panic(wire.Build(
		multiTokenDescriptorsFetcherProvider,
		wire.Bind(new(tokenDescriptorsFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenDescriptorsFetcherB), util.ToPointer(openseaProvider)),
	))
}

func polygonTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	panic(wire.Build(
		multiTokensIncrementalContractFetcherProvider,
		wire.Bind(new(tokensIncrementalContractFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalContractFetcherB), util.ToPointer(openseaProvider)),
	))
}

func polygonTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	panic(wire.Build(
		multiTokenIdentifierOwnerFetcherProvider,
		wire.Bind(new(tokenIdentifierOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokenIdentifierOwnerFetcherB), util.ToPointer(openseaProvider)),
	))
}

func polygonTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	panic(wire.Build(
		multiTokensIncrementalOwnerFetcherProvider,
		wire.Bind(new(tokensIncrementalOwnerFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensIncrementalOwnerFetcherB), util.ToPointer(openseaProvider)),
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
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
	))
}

func polygonTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	panic(wire.Build(
		multiTokenByTokenIdentifiersFetcherProvider,
		wire.Bind(new(tokensByTokenIdentifiersFetcherA), util.ToPointer(alchemyProvider)),
		wire.Bind(new(tokensByTokenIdentifiersFetcherB), util.ToPointer(openseaProvider)),
	))
}

func newTokenManageCache() *redis.Cache {
	return redis.NewCache(redis.TokenManageCache)
}

func submitTokenBatchInjector(context.Context, *redis.Cache) multichain.SubmitTokensF {
	panic(wire.Build(
		submitBatch,
		tokenmanage.New,
		task.NewClient,
		tickToken,
	))
}

func tickToken() tokenmanage.TickToken { return nil }

func submitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
