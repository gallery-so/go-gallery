//go:build wireinject
// +build wireinject

package server

import (
	"context"
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

type SetEnv struct{}
type ethProviderList []any
type tezosProviderList []any
type optimismProviderList []any
type poapProviderList []any
type polygonProviderList []any
type optimismProvider struct{ *alchemy.Provider }
type polygonProvider struct{ *alchemy.Provider }

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
func NewMultichainProvider(ctx context.Context) *multichain.Provider {
	wire.Build(
		defaultChainOverrides,
		task.NewClient,
		rpc.NewEthClient,
		newCache,
		newIndexerProvider,
		newOptimismProvider,
		opensea.NewProvider,
		ethFallbackProvider,
		tezosFallbackProvider,
		newPoapProvider,
		newPolygonProvider,
		multichain.NewProvider,
		postgres.MustCreateClient,
		postgres.NewRepositories,
		postgres.NewPgxClient,
		db.New,
		newMultichain,
		ethProvidersConfig,
		tezosProvidersConfig,
		optimismProvidersConfig,
		poapProvidersConfig,
		polygonProvidersConfig,
		wire.Value([]postgres.ConnectionOption{}),
		wire.Value(&http.Client{Timeout: 0}),
		wire.Bind(new(db.DBTX), util.ToPointer(NewPgxClient(setEnv()))),
	)
	return nil
}

func NewPgxClient(SetEnv) *pgxpool.Pool {
	return postgres.NewPgxClient()
}

func setEnv() SetEnv {
	SetDefaults()
	return SetEnv{}
}

func ethProvidersConfig(indexerProvider *eth.Provider, openseaProvider *opensea.Provider, fallbackProvider multichain.SyncFailureFallbackProvider) ethProviderList {
	wire.Build(
		wire.Bind(new(multichain.NameResolver), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.Verifier), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(fallbackProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(openseaProvider)),
		wire.Bind(new(multichain.ContractRefresher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenMetadataFetcher), util.ToPointer(indexerProvider)),
		wire.Bind(new(multichain.TokenDescriptorsFetcher), util.ToPointer(indexerProvider)),
		ethProviders,
	)
	return nil
}

func ethProviders(
	nr multichain.NameResolver,
	v multichain.Verifier,
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
	cr multichain.ContractRefresher,
	tmf multichain.TokenMetadataFetcher,
	tdf multichain.TokenDescriptorsFetcher,
) ethProviderList {
	return ethProviderList{nr, v, tof, toc, cr, tmf, tdf}
}

func tezosProvidersConfig(tezosProvider multichain.SyncWithContractEvalFallbackProvider) tezosProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(tezosProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(tezosProvider)),
		tezosProviders,
	)
	return nil
}

func tezosProviders(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) tezosProviderList {
	return tezosProviderList{tof, toc}
}

func optimismProvidersConfig(optimismProvider *optimismProvider) optimismProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(optimismProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(optimismProvider)),
		optimismProviders,
	)
	return nil
}

func optimismProviders(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) optimismProviderList {
	return optimismProviderList{tof, toc}
}

func poapProvidersConfig(poapProvider *poap.Provider) poapProviderList {
	wire.Build(
		wire.Bind(new(multichain.NameResolver), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(poapProvider)),
		poapProviders,
	)
	return nil
}

func poapProviders(
	nr multichain.NameResolver,
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) poapProviderList {
	return poapProviderList{nr, tof, toc}
}

func polygonProvidersConfig(polygonProvider *polygonProvider) polygonProviderList {
	wire.Build(
		wire.Bind(new(multichain.TokensOwnerFetcher), util.ToPointer(polygonProvider)),
		wire.Bind(new(multichain.TokensContractFetcher), util.ToPointer(polygonProvider)),
		polygonProviders,
	)
	return nil
}

func polygonProviders(
	tof multichain.TokensOwnerFetcher,
	toc multichain.TokensContractFetcher,
) polygonProviderList {
	return polygonProviderList{tof, toc}
}

// newMultichain is a wire provider that creates a multichain provider
func newMultichain(
	ethProviders ethProviderList,
	optimismProviders optimismProviderList,
	tezosProviders tezosProviderList,
	poapProviders poapProviderList,
	polygonProviders polygonProviderList,
) []any {
	providers := []any{}
	providers = append(providers, ethProviders...)
	providers = append(providers, optimismProviders...)
	providers = append(providers, tezosProviders...)
	providers = append(providers, poapProviders...)
	providers = append(providers, polygonProviders...)
	return providers
}

// defaultChainOverrides is a wire provider for chain overrides
func defaultChainOverrides() multichain.ChainOverrideMap {
	var ethChain = persist.ChainETH
	return multichain.ChainOverrideMap{
		persist.ChainPOAP:     &ethChain,
		persist.ChainOptimism: &ethChain,
		persist.ChainPolygon:  &ethChain,
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

func tezosFallbackProvider(httpClient *http.Client) multichain.SyncWithContractEvalFallbackProvider {
	wire.Build(
		newTzktProvider,
		newObjktProvider,
		tezosTokenEvalFunc,
		wire.Bind(new(multichain.SyncWithContractEvalPrimary), util.ToPointer(newTzktProvider(httpClient))),
		wire.Bind(new(multichain.SyncWithContractEvalSecondary), util.ToPointer(newObjktProvider())),
		wire.Struct(new(multichain.SyncWithContractEvalFallbackProvider), "*"),
	)
	return multichain.SyncWithContractEvalFallbackProvider{}
}

func newIndexerProvider(httpClient *http.Client, ethClient *ethclient.Client, taskClient *cloudtasks.Client) *eth.Provider {
	return eth.NewProvider(env.GetString("INDEXER_HOST"), httpClient, ethClient, taskClient)
}

func newTzktProvider(httpClient *http.Client) *tezos.Provider {
	return tezos.NewProvider(env.GetString("TEZOS_API_URL"), httpClient)
}

func newObjktProvider() *tezos.TezosObjktProvider {
	return tezos.NewObjktProvider(env.GetString("IPFS_URL"))
}

func tezosTokenEvalFunc() func(context.Context, multichain.ChainAgnosticToken) bool {
	return func(ctx context.Context, token multichain.ChainAgnosticToken) bool {
		return tezos.IsSigned(ctx, token) && tezos.ContainsTezosKeywords(ctx, token)
	}
}

func newPoapProvider(c *http.Client) *poap.Provider {
	return poap.NewProvider(c, env.GetString("POAP_API_KEY"), env.GetString("POAP_AUTH_TOKEN"))
}

func newOptimismProvider(c *http.Client) *optimismProvider {
	return &optimismProvider{alchemy.NewProvider(persist.ChainOptimism, c)}
}

func newPolygonProvider(c *http.Client) *polygonProvider {
	return &polygonProvider{alchemy.NewProvider(persist.ChainPolygon, c)}
}

// TODO: CHANGE THIS
func newCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}
