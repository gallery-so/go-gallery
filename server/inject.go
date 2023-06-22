//go:build wireinject
// +build wireinject

package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/wire"
	"github.com/jackc/pgx/v4/pgxpool"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/alchemy"
	"github.com/mikeydub/go-gallery/service/multichain/eth"
	"github.com/mikeydub/go-gallery/service/multichain/infura"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/task"
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
type polygonProviderList []any
type arbitrumProviderList []any
type optimismProvider struct{ *alchemy.Provider }
type polygonProvider struct{ *alchemy.Provider }
type arbitrumProvider struct{ *alchemy.Provider }

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
func NewMultichainProvider(ctx context.Context, envFunc func()) (*multichain.Provider, func()) {
	wire.Build(
		setEnv,
		wire.Value(&http.Client{Timeout: 0}), // HTTP client shared between providers
		defaultChainOverrides,
		task.NewClient,
		newCommunitiesCache,
		postgres.NewRepositories,
		dbConnSet,
		newSendTokensFunc,
		wire.Struct(new(multichain.Provider), "*"),
		// Add additional chains here
		newMultichainSet,
		ethProviderSet,
		tezosProviderSet,
		optimismProviderSet,
		poapProviderSet,
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
func ethProviderSet(envInit, *cloudtasks.Client, *http.Client) ethProviderList {
	wire.Build(
		rpc.NewEthClient,
		ethProvidersConfig,
		// Add providers for Ethereum here
		newIndexerProvider,
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
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.ContractsFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.OpenSeaChildContractFetcher), util.ToPointer(openseaProvider)),
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
	cf multichain.ContractsFetcher,
	cr multichain.ContractRefresher,
	tmf multichain.TokenMetadataFetcher,
	tdf multichain.TokenDescriptorsFetcher,
	osccf multichain.OpenSeaChildContractFetcher,
) ethProviderList {
	return ethProviderList{nr, v, tof, toc, cf, cr, tmf, tdf, osccf}
}

// tezosProviderSet is a wire injector that creates the set of Tezos providers
func tezosProviderSet(envInit, *http.Client) tezosProviderList {
	wire.Build(
		tezosProvidersConfig,
		// Add providers for Tezos here
		newTzktProvider,
		newObjktProvider,
		tezosFallbackProvider,
	)
	return tezosProviderList{}
}

// tezosProvidersConfig is a wire injector that binds multichain interfaces to their concrete Tezos implementations
func tezosProvidersConfig(tezosProvider multichain.SyncWithContractEvalFallbackProvider) tezosProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(tezosProvider)),
		tezosRequirements,
	)
	return nil
}

// tezosRequirements is the set of provider interfaces required for Tezos
func tezosRequirements(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) tezosProviderList {
	return tezosProviderList{tof, toc}
}

// optimismProviderSet is a wire injector that creates the set of Optimism providers
func optimismProviderSet(*http.Client) optimismProviderList {
	wire.Build(
		optimismProvidersConfig,
		// Add providers for Optimism here
		newOptimismProvider,
	)
	return optimismProviderList{}
}

// optimismProvidersConfig is a wire injector that binds multichain interfaces to their concrete Optimism implementations
func optimismProvidersConfig(optimismProvider *optimismProvider) optimismProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(optimismProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(optimismProvider)),
		optimismRequirements,
	)
	return nil
}

// optimismRequirements is the set of provider interfaces required for Optimism
func optimismRequirements(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) optimismProviderList {
	return optimismProviderList{tof, toc}
}

// arbitrumProviderSet is a wire injector that creates the set of Arbitrum providers
func arbitrumProviderSet(*http.Client) arbitrumProviderList {
	wire.Build(
		arbitrumProvidersConfig,
		// Add providers for Optimism here
		newArbitrumProvider,
	)
	return arbitrumProviderList{}
}

// arbitrumProvidersConfig is a wire injector that binds multichain interfaces to their concrete Arbitrum implementations
func arbitrumProvidersConfig(arbitrumProvider *arbitrumProvider) arbitrumProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(arbitrumProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(arbitrumProvider)),
		arbitrumRequirements,
	)
	return nil
}

// arbitrumRequirements is the set of provider interfaces required for Arbitrum
func arbitrumRequirements(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) arbitrumProviderList {
	return arbitrumProviderList{tof, toc}
}

// poapProviderSet is a wire injector that creates the set of POAP providers
func poapProviderSet(envInit, *http.Client) poapProviderList {
	wire.Build(
		poapProvidersConfig,
		// Add providers for POAP here
		newPoapProvider,
	)
	return poapProviderList{}
}

// poapProvidersConfig is a wire injector that binds multichain interfaces to their concrete POAP implementations
func poapProvidersConfig(poapProvider *poap.Provider) poapProviderList {
	wire.Build(
		wire.Bind(new(multichain.NameResolver), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(poapProvider)),
		poapRequirements,
	)
	return nil
}

// poapRequirements is the set of provider interfaces required for POAP
func poapRequirements(
	nr multichain.NameResolver,
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) poapProviderList {
	return poapProviderList{nr, tof, toc}
}

// polygonProviderSet is a wire injector that creates the set of polygon providers
func polygonProviderSet(*http.Client) polygonProviderList {
	wire.Build(
		polygonProvidersConfig,
		// Add providers for POAP here
		newPolygonProvider,
	)
	return polygonProviderList{}
}

// polygonProvidersConfig is a wire injector that binds multichain interfaces to their concrete Polygon implementations
func polygonProvidersConfig(polygonProvider *polygonProvider) polygonProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(polygonProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(polygonProvider)),
		polygonRequirements,
	)
	return nil
}

// polygonRequirements is the set of provider interfaces required for Polygon
func polygonRequirements(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) polygonProviderList {
	return polygonProviderList{tof, toc}
}

// newMultichain is a wire provider that creates a multichain provider
func newMultichainSet(
	ethProviders ethProviderList,
	optimismProviders optimismProviderList,
	tezosProviders tezosProviderList,
	poapProviders poapProviderList,
	polygonProviders polygonProviderList,
	arbitrumProviders arbitrumProviderList,
) map[persist.Chain][]any {
	// Dedupes providers by pointer address because
	// providers may not be hashable
	dedupe := func(providers []any) []any {
		seen := map[string]bool{}
		deduped := []any{}
		for _, p := range providers {
			if addr := fmt.Sprintf("%p", p); !seen[addr] {
				seen[addr] = true
				deduped = append(deduped, p)
			}
		}
		return deduped
	}

	chainToProviders := map[persist.Chain][]any{}
	chainToProviders[persist.ChainETH] = dedupe(ethProviders)
	chainToProviders[persist.ChainOptimism] = dedupe(optimismProviders)
	chainToProviders[persist.ChainTezos] = dedupe(tezosProviders)
	chainToProviders[persist.ChainPOAP] = dedupe(poapProviders)
	chainToProviders[persist.ChainPolygon] = dedupe(polygonProviders)
	chainToProviders[persist.ChainArbitrum] = dedupe(arbitrumProviders)
	return chainToProviders
}

// defaultChainOverrides is a wire provider for chain overrides
func defaultChainOverrides() multichain.ChainOverrideMap {
	var ethChain = persist.ChainETH
	return multichain.ChainOverrideMap{
		persist.ChainPOAP:     &ethChain,
		persist.ChainOptimism: &ethChain,
		persist.ChainPolygon:  &ethChain,
		persist.ChainArbitrum: &ethChain,
	}
}

func ethFallbackProvider(httpClient *http.Client) multichain.SyncFailureFallbackProvider {
	wire.Build(
		alchemy.NewProvider,
		infura.NewProvider,
		wire.Value(persist.ChainETH),
		wire.Bind(new(multichain.SyncFailurePrimary), util.ToPointer(alchemy.NewProvider(persist.ChainETH, httpClient))),
		wire.Bind(new(multichain.SyncFailureSecondary), util.ToPointer(infura.NewProvider(httpClient))),
		wire.Struct(new(multichain.SyncFailureFallbackProvider), "*"),
	)
	return multichain.SyncFailureFallbackProvider{}
}

func tezosFallbackProvider(httpClient *http.Client, tzktProvider *tezos.Provider, objktProvider *tezos.TezosObjktProvider) multichain.SyncWithContractEvalFallbackProvider {
	wire.Build(
		tezosTokenEvalFunc,
		wire.Bind(new(multichain.SyncWithContractEvalPrimary), util.ToPointer(tzktProvider)),
		wire.Bind(new(multichain.SyncWithContractEvalSecondary), util.ToPointer(objktProvider)),
		wire.Struct(new(multichain.SyncWithContractEvalFallbackProvider), "*"),
	)
	return multichain.SyncWithContractEvalFallbackProvider{}
}

func newIndexerProvider(e envInit, httpClient *http.Client, ethClient *ethclient.Client, taskClient *cloudtasks.Client) *eth.Provider {
	return eth.NewProvider(env.GetString("INDEXER_HOST"), httpClient, ethClient, taskClient)
}

func newTzktProvider(e envInit, httpClient *http.Client) *tezos.Provider {
	return tezos.NewProvider(env.GetString("TEZOS_API_URL"), httpClient)
}

func newObjktProvider(e envInit) *tezos.TezosObjktProvider {
	return tezos.NewObjktProvider(env.GetString("IPFS_URL"))
}

func tezosTokenEvalFunc() func(context.Context, multichain.ChainAgnosticToken) bool {
	return func(ctx context.Context, token multichain.ChainAgnosticToken) bool {
		return tezos.IsSigned(ctx, token) && tezos.ContainsTezosKeywords(ctx, token)
	}
}

func newPoapProvider(e envInit, c *http.Client) *poap.Provider {
	return poap.NewProvider(c, env.GetString("POAP_API_KEY"), env.GetString("POAP_AUTH_TOKEN"))
}

func newOptimismProvider(c *http.Client) *optimismProvider {
	return &optimismProvider{alchemy.NewProvider(persist.ChainOptimism, c)}
}

func newPolygonProvider(c *http.Client) *polygonProvider {
	return &polygonProvider{alchemy.NewProvider(persist.ChainPolygon, c)}
}

func newArbitrumProvider(c *http.Client) *arbitrumProvider {
	return &arbitrumProvider{alchemy.NewProvider(persist.ChainArbitrum, c)}
}

func newCommunitiesCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}

func newSendTokensFunc(ctx context.Context, taskClient *cloudtasks.Client) multichain.SendTokens {
	return func(ctx context.Context, t task.TokenProcessingUserMessage) error {
		return task.CreateTaskForTokenProcessing(ctx, taskClient, t)
	}
}
