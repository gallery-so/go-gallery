//go:build wireinject
// +build wireinject

package server

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/wire"
	"github.com/jackc/pgx/v4/pgxpool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/multichain/simplehash"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/multichain/wrapper"
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
		wire.Value(http.DefaultClient), // HTTP client shared between providers
		postgres.NewRepositories,
		dbConnSet,
		wire.Struct(new(multichain.ChainProvider), "*"),
		tokenProcessingSubmitterInjector,
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

func newTokenManageCache() *redis.Cache {
	return redis.NewCache(redis.TokenManageCache)
}

func multichainProviderInjector(ctx context.Context, repos *postgres.Repositories, q *db.Queries, chainProvider *multichain.ChainProvider, submitter *tokenmanage.TokenProcessingSubmitter) *multichain.Provider {
	panic(wire.Build(
		wire.Struct(new(multichain.Provider), "*"),
		wire.Bind(new(tokenmanage.Submitter), util.ToPointer(submitter)),
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

func customMetadataHandlersInjector() *multichain.CustomMetadataHandlers {
	panic(wire.Build(
		multichain.NewCustomMetadataHandlers,
		rpc.NewEthClient,
		ipfs.NewShell,
		arweave.NewClient,
	))
}

func ethInjector(envInit, context.Context, *http.Client) (*multichain.EthereumProvider, func()) {
	panic(wire.Build(
		rpc.NewEthClient,
		wire.Value(persist.ChainETH),
		ethProviderInjector,
		ethSyncPipelineInjector,
		ethVerifierInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func ethVerifierInjector(ethClient *ethclient.Client) *eth.Verifier {
	panic(wire.Build(wire.Struct(new(eth.Verifier), "*")))
}

func ethProviderInjector(
	ctx context.Context,
	syncPipeline *wrapper.SyncPipelineWrapper,
	verifier *eth.Verifier,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.EthereumProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.EthereumProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.Verifier), util.ToPointer(verifier)),
	))
}

func ethSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func tezosInjector(envInit, *http.Client) *multichain.TezosProvider {
	wire.Build(
		tezosProviderInjector,
		wire.Value(persist.ChainTezos),
		tezos.NewProvider,
		simplehash.NewProvider,
	)
	return nil
}

func tezosProviderInjector(tezosProvider *tezos.Provider, simplehashProvider *simplehash.Provider) *multichain.TezosProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.TezosProvider), "*"),
		wire.Bind(new(multichain.Verifier), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
	))
}

func optimismInjector(context.Context, *http.Client) (*multichain.OptimismProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainOptimism),
		optimismProviderInjector,
		optimismSyncPipelineInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func optimismProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.OptimismProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.OptimismProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func optimismSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func arbitrumInjector(context.Context, *http.Client) (*multichain.ArbitrumProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainArbitrum),
		arbitrumProviderInjector,
		arbitrumSyncPipelineInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func arbitrumProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.ArbitrumProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.ArbitrumProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func arbitrumSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
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

func zoraInjector(envInit, context.Context, *http.Client) (*multichain.ZoraProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainZora),
		zoraProviderInjector,
		zoraSyncPipelineInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func zoraProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.ZoraProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.ZoraProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func zoraSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func baseInjector(context.Context, *http.Client) (*multichain.BaseProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainBase),
		baseProvidersInjector,
		baseSyncPipelineInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func baseProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.BaseProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.BaseProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func baseSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func polygonInjector(context.Context, *http.Client) (*multichain.PolygonProvider, func()) {
	panic(wire.Build(
		wire.Value(persist.ChainPolygon),
		polygonProvidersInjector,
		polygonSyncPipelineInjector,
		simplehash.NewProvider,
		reservoir.NewProvider,
	))
}

func polygonProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	reservoirProvider *reservoir.Provider,
) *multichain.PolygonProvider {
	panic(wire.Build(
		wire.Struct(new(multichain.PolygonProvider), "*"),
		wire.Bind(new(multichain.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.MintStatuser), util.ToPointer(reservoirProvider)),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(simplehashProvider)),
	))
}

func polygonSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
) (*wrapper.SyncPipelineWrapper, func()) {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(multichain.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(multichain.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func tokenProcessingSubmitterInjector(context.Context) *tokenmanage.TokenProcessingSubmitter {
	panic(wire.Build(
		wire.Struct(new(tokenmanage.TokenProcessingSubmitter), "*"),
		task.NewClient,
		wire.Struct(new(tokenmanage.Registry), "*"),
		newTokenManageCache,
	))
}
