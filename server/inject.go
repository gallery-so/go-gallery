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
	"github.com/mikeydub/go-gallery/service/multichain/alchemy"
	"github.com/mikeydub/go-gallery/service/multichain/eth"
	"github.com/mikeydub/go-gallery/service/multichain/infura"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/reservoir"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
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

type ethProviderList []any
type tezosProviderList []any
type optimismProviderList []any
type poapProviderList []any
type zoraProviderList []any
type baseProviderList []any
type polygonProviderList []any
type arbitrumProviderList []any
type tokenMetadataCache redis.Cache

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
func NewMultichainProvider(ctx context.Context, envFunc func()) (*multichain.Provider, func()) {
	wire.Build(
		setEnv,
		wire.Value(&http.Client{Timeout: 0}), // HTTP client shared between providers
		task.NewClient,
		newCommunitiesCache,
		newTokenMetadataCache,
		postgres.NewRepositories,
		dbConnSet,
		tokenmanage.New,
		newSubmitBatch,
		wire.Struct(new(multichain.Provider), "*"),
		// Add additional chains here
		newMultichainSet,
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

// ethProviderSet is a wire injector that creates the set of Ethereum providers
func ethProviderSet(envInit, *task.Client, *http.Client, *tokenMetadataCache) ethProviderList {
	wire.Build(
		rpc.NewEthClient,
		ethProvidersConfig,
		wire.Value(persist.ChainETH),
		// Add providers for Ethereum here
		eth.NewProvider,
		ethFallbackProvider,
		opensea.NewProvider,
	)
	return ethProviderList{}
}

// ethProvidersConfig is a wire injector that binds multichain interfaces to their concrete Ethereum implementations
func ethProvidersConfig(indexerProvider *eth.Provider, openseaProvider *opensea.Provider, fallbackProvider multichain.SyncFailureFallbackProvider) ethProviderList {
	wire.Build(
		wire.Bind(new(multichain.NameResolver), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.Verifier), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.ContractsFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(indexerProvider)),
		ethRequirements,
	)
	return nil
}

// ethRequirements is the set of provider interfaces required for Ethereum
func ethRequirements(
	nr multichain.NameResolver,
	v multichain.Verifier,
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	ticf multichain.TokensIncrementalContractFetcher,
	cf multichain.ContractsFetcher,
	cr multichain.ContractRefresher,
	tmf multichain.TokenMetadataFetcher,
	tcof multichain.ContractsOwnerFetcher,
	tdf multichain.TokenDescriptorsFetcher,
) ethProviderList {
	return ethProviderList{nr, v, tof, toc, tiof, ticf, cf, cr, tmf, tcof, tdf}
}

// tezosProviderSet is a wire injector that creates the set of Tezos providers
func tezosProviderSet(envInit, *http.Client) tezosProviderList {
	wire.Build(
		tezosProvidersConfig,
		// Add providers for Tezos here
		tezosFallbackProvider,
	)
	return tezosProviderList{}
}

// tezosProvidersConfig is a wire injector that binds multichain interfaces to their concrete Tezos implementations
func tezosProvidersConfig(tezosProvider multichain.SyncWithContractEvalFallbackProvider) tezosProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(tezosProvider)),
		tezosRequirements,
	)
	return nil
}

// tezosRequirements is the set of provider interfaces required for Tezos
func tezosRequirements(
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	ticf multichain.TokensIncrementalContractFetcher,
	toc multichain.TokensContractFetcher,
	tmf multichain.TokenMetadataFetcher,
	tcof multichain.ContractsOwnerFetcher,
) tezosProviderList {
	return tezosProviderList{tof, tiof, ticf, toc, tmf, tcof}
}

// optimismProviderSet is a wire injector that creates the set of Optimism providers
func optimismProviderSet(*http.Client, *tokenMetadataCache) optimismProviderList {
	wire.Build(
		optimismProvidersConfig,
		wire.Value(persist.ChainOptimism),
		// Add providers for Optimism here
		newAlchemyProvider,
		opensea.NewProvider,
	)
	return optimismProviderList{}
}

// optimismProvidersConfig is a wire injector that binds multichain interfaces to their concrete Optimism implementations
func optimismProvidersConfig(alchemyProvider *alchemy.Provider, openseaProvider *opensea.Provider) optimismProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(alchemyProvider)),
		optimismRequirements,
	)
	return nil
}

// optimismRequirements is the set of provider interfaces required for Optimism
func optimismRequirements(
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	toc multichain.TokensContractFetcher,
	tmf multichain.TokenMetadataFetcher,
) optimismProviderList {
	return optimismProviderList{tof, toc, tiof, tmf}
}

// arbitrumProviderSet is a wire injector that creates the set of Arbitrum providers
func arbitrumProviderSet(*http.Client, *tokenMetadataCache) arbitrumProviderList {
	wire.Build(
		arbitrumProvidersConfig,
		wire.Value(persist.ChainArbitrum),
		// Add providers for Optimism here
		newAlchemyProvider,
		opensea.NewProvider,
	)
	return arbitrumProviderList{}
}

// arbitrumProvidersConfig is a wire injector that binds multichain interfaces to their concrete Arbitrum implementations
func arbitrumProvidersConfig(alchemyProvider *alchemy.Provider, openseaProvider *opensea.Provider) arbitrumProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(alchemyProvider)),
		arbitrumRequirements,
	)
	return nil
}

// arbitrumRequirements is the set of provider interfaces required for Arbitrum
func arbitrumRequirements(
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	toc multichain.TokensContractFetcher,
	tmf multichain.TokenMetadataFetcher,
	tdf multichain.TokenDescriptorsFetcher,
) arbitrumProviderList {
	return arbitrumProviderList{tof, toc, tiof, tmf, tdf}
}

// poapProviderSet is a wire injector that creates the set of POAP providers
func poapProviderSet(envInit, *http.Client) poapProviderList {
	wire.Build(
		poapProvidersConfig,
		// Add providers for POAP here
		poap.NewProvider,
	)
	return poapProviderList{}
}

// poapProvidersConfig is a wire injector that binds multichain interfaces to their concrete POAP implementations
func poapProvidersConfig(poapProvider *poap.Provider) poapProviderList {
	wire.Build(
		wire.Bind(new(multichain.NameResolver), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(poapProvider)),
		poapRequirements,
	)
	return nil
}

// poapRequirements is the set of provider interfaces required for POAP
func poapRequirements(
	nr multichain.NameResolver,
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	toc multichain.TokensContractFetcher,
	tmf multichain.TokenMetadataFetcher,
) poapProviderList {
	return poapProviderList{nr, tof, tiof, toc, tmf}
}

// zoraProviderSet is a wire injector that creates the set of zora providers
func zoraProviderSet(envInit, *http.Client) zoraProviderList {
	wire.Build(
		zoraProvidersConfig,
		// Add providers for Zora here
		zora.NewProvider,
	)
	return zoraProviderList{}
}

// zoraProvidersConfig is a wire injector that binds multichain interfaces to their concrete zora implementations
func zoraProvidersConfig(zoraProvider *zora.Provider) zoraProviderList {
	wire.Build(
		wire.Bind(new(multichain.ContractsFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokensIncrementalContractFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.ContractsOwnerFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(zoraProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(zoraProvider)),
		zoraRequirements,
	)
	return nil
}

// zoraRequirements is the set of provider interfaces required for zora
func zoraRequirements(
	nr multichain.ContractsFetcher,
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	ticf multichain.TokensIncrementalContractFetcher,
	toc multichain.TokensContractFetcher,
	tcof multichain.ContractsOwnerFetcher,
	tmf multichain.TokenMetadataFetcher,
	tdf multichain.TokenDescriptorsFetcher,
) zoraProviderList {
	return zoraProviderList{nr, tof, tiof, ticf, toc, tcof, tmf, tdf}
}

func baseProviderSet(*http.Client) baseProviderList {
	wire.Build(
		baseProvidersConfig,
		wire.Value(persist.ChainBase),
		// Add providers for Base here
		reservoir.NewProvider,
	)
	return baseProviderList{}
}

// baseProvidersConfig is a wire injector that binds multichain interfaces to their concrete base implementations
func baseProvidersConfig(baseProvider *reservoir.Provider) baseProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(baseProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(baseProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(baseProvider)),
		baseRequirements,
	)
	return nil
}

// zoraRequirements is the set of provider interfaces required for zora
func baseRequirements(
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	tdf multichain.TokenDescriptorsFetcher,
) baseProviderList {
	return baseProviderList{tof, tiof, tdf}
}

// polygonProviderSet is a wire injector that creates the set of polygon providers
func polygonProviderSet(*http.Client, *tokenMetadataCache) polygonProviderList {
	wire.Build(
		polygonProvidersConfig,
		wire.Value(persist.ChainPolygon),
		// Add providers for Polygon here
		newAlchemyProvider,
		reservoir.NewProvider,
	)
	return polygonProviderList{}
}

// polygonProvidersConfig is a wire injector that binds multichain interfaces to their concrete Polygon implementations
func polygonProvidersConfig(alchemyProvider *alchemy.Provider, reservoirProvider *reservoir.Provider) polygonProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensIncrementalOwnerFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(alchemyProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(reservoirProvider)),
		polygonRequirements,
	)
	return nil
}

// polygonRequirements is the set of provider interfaces required for Polygon
func polygonRequirements(
	tof multichain.TokensOwnerFetcher,
	tiof multichain.TokensIncrementalOwnerFetcher,
	toc multichain.TokensContractFetcher,
	tmf multichain.TokenMetadataFetcher,
) polygonProviderList {
	return polygonProviderList{tof, tiof, toc, tmf}
}

// dedupe removes duplicate providers based on provider ID
func dedupe(providers []any) []any {
	seen := map[string]bool{}
	deduped := []any{}
	for _, p := range providers {
		if id := p.(multichain.Configurer).GetBlockchainInfo().ProviderID; !seen[id] {
			seen[id] = true
			deduped = append(deduped, p)
		}
	}
	return deduped
}

// newMultichain is a wire provider that creates a multichain provider
func newMultichainSet(
	ethProviders ethProviderList,
	optimismProviders optimismProviderList,
	tezosProviders tezosProviderList,
	poapProviders poapProviderList,
	zoraProviders zoraProviderList,
	baseProviders baseProviderList,
	polygonProviders polygonProviderList,
	arbitrumProviders arbitrumProviderList,
) map[persist.Chain][]any {
	chainToProviders := map[persist.Chain][]any{}
	chainToProviders[persist.ChainETH] = dedupe(ethProviders)
	chainToProviders[persist.ChainOptimism] = dedupe(optimismProviders)
	chainToProviders[persist.ChainTezos] = dedupe(tezosProviders)
	chainToProviders[persist.ChainPOAP] = dedupe(poapProviders)
	chainToProviders[persist.ChainZora] = dedupe(zoraProviders)
	chainToProviders[persist.ChainBase] = dedupe(baseProviders)
	chainToProviders[persist.ChainPolygon] = dedupe(polygonProviders)
	chainToProviders[persist.ChainArbitrum] = dedupe(arbitrumProviders)
	return chainToProviders
}

func ethFallbackProvider(httpClient *http.Client, cache *tokenMetadataCache) multichain.SyncFailureFallbackProvider {
	wire.Build(
		wire.Value(persist.ChainETH),
		infura.NewProvider,
		newAlchemyProvider,
		wire.Bind(new(multichain.SyncFailurePrimary), new(*alchemy.Provider)),
		wire.Bind(new(multichain.SyncFailureSecondary), new(*infura.Provider)),
		wire.Struct(new(multichain.SyncFailureFallbackProvider), "*"),
	)
	return multichain.SyncFailureFallbackProvider{}
}

func tezosFallbackProvider(e envInit, httpClient *http.Client) multichain.SyncWithContractEvalFallbackProvider {
	wire.Build(
		tezos.NewProvider,
		tezos.NewObjktProvider,
		tezosTokenEvalFunc,
		wire.Bind(new(multichain.SyncWithContractEvalPrimary), new(*tezos.Provider)),
		wire.Bind(new(multichain.SyncWithContractEvalSecondary), new(*tezos.TezosObjktProvider)),
		wire.Struct(new(multichain.SyncWithContractEvalFallbackProvider), "*"),
	)
	return multichain.SyncWithContractEvalFallbackProvider{}
}

func tezosTokenEvalFunc() func(context.Context, multichain.ChainAgnosticToken) bool {
	return func(ctx context.Context, token multichain.ChainAgnosticToken) bool {
		return tezos.IsSigned(ctx, token) && tezos.ContainsTezosKeywords(ctx, token)
	}
}

func newAlchemyProvider(httpClient *http.Client, chain persist.Chain, cache *tokenMetadataCache) *alchemy.Provider {
	c := redis.Cache(*cache)
	return alchemy.NewProvider(chain, httpClient, util.ToPointer(c))
}

func newCommunitiesCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}

func newTokenMetadataCache() *tokenMetadataCache {
	cache := redis.NewCache(redis.TokenProcessingMetadataCache)
	return util.ToPointer(tokenMetadataCache(*cache))
}

func newSubmitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
