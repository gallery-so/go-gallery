// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package server

import (
	"context"
	"database/sql"
	"github.com/google/wire"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/indexer"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/simplehash"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/multichain/tzkt"
	"github.com/mikeydub/go-gallery/service/multichain/wrapper"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/rpc/arweave"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"net/http"
)

// Injectors from inject.go:

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
func NewMultichainProvider(ctx context.Context, envFunc func()) (*multichain.Provider, func()) {
	serverEnvInit := setEnv(envFunc)
	db, cleanup := newPqClient(serverEnvInit)
	pool, cleanup2 := newPgxClient(serverEnvInit)
	repositories := postgres.NewRepositories(db, pool)
	queries := newQueries(pool)
	cache := newTokenManageCache()
	client := _wireClientValue
	ethereumProvider, cleanup3 := ethInjector(serverEnvInit, ctx, client)
	tezosProvider := tezosInjector(serverEnvInit, client)
	optimismProvider, cleanup4 := optimismInjector(ctx, client)
	arbitrumProvider, cleanup5 := arbitrumInjector(ctx, client)
	poapProvider := poapInjector(serverEnvInit, client)
	zoraProvider, cleanup6 := zoraInjector(serverEnvInit, ctx, client)
	baseProvider, cleanup7 := baseInjector(ctx, client)
	polygonProvider, cleanup8 := polygonInjector(ctx, client)
	chainProvider := &multichain.ChainProvider{
		Ethereum: ethereumProvider,
		Tezos:    tezosProvider,
		Optimism: optimismProvider,
		Arbitrum: arbitrumProvider,
		Poap:     poapProvider,
		Zora:     zoraProvider,
		Base:     baseProvider,
		Polygon:  polygonProvider,
	}
	provider := multichainProviderInjector(ctx, repositories, queries, cache, chainProvider)
	return provider, func() {
		cleanup8()
		cleanup7()
		cleanup6()
		cleanup5()
		cleanup4()
		cleanup3()
		cleanup2()
		cleanup()
	}
}

var (
	_wireClientValue = &http.Client{Timeout: 0}
)

func multichainProviderInjector(contextContext context.Context, repositories *postgres.Repositories, queries *coredb.Queries, cache *redis.Cache, chainProvider *multichain.ChainProvider) *multichain.Provider {
	submitTokensF := submitTokenBatchInjector(contextContext, cache)
	providerLookup := newProviderLookup(chainProvider)
	provider := &multichain.Provider{
		Repos:        repositories,
		Queries:      queries,
		SubmitTokens: submitTokensF,
		Chains:       providerLookup,
	}
	return provider
}

func customMetadataHandlersInjector() *multichain.CustomMetadataHandlers {
	client := rpc.NewEthClient()
	shell := ipfs.NewShell()
	goarClient := arweave.NewClient()
	customMetadataHandlers := multichain.NewCustomMetadataHandlers(client, shell, goarClient)
	return customMetadataHandlers
}

func ethInjector(serverEnvInit envInit, contextContext context.Context, client *http.Client) (*multichain.EthereumProvider, func()) {
	chain := _wireChainValue
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := ethSyncPipelineInjector(contextContext, client, chain, provider)
	ethclientClient := rpc.NewEthClient()
	indexerProvider := indexer.NewProvider(client, ethclientClient)
	ethereumProvider := ethProviderInjector(contextContext, syncPipelineWrapper, indexerProvider, provider)
	return ethereumProvider, func() {
		cleanup()
	}
}

var (
	_wireChainValue = persist.ChainETH
)

func ethProviderInjector(ctx context.Context, syncPipeline *wrapper.SyncPipelineWrapper, indexerProvider *indexer.Provider, simplehashProvider *simplehash.Provider) *multichain.EthereumProvider {
	ethereumProvider := &multichain.EthereumProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             syncPipeline,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		Verifier:                         indexerProvider,
	}
	return ethereumProvider
}

func ethSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func tezosInjector(serverEnvInit envInit, client *http.Client) *multichain.TezosProvider {
	provider := tezos.NewProvider()
	tzktProvider := tzkt.NewProvider(client)
	tezosProvider := tezosProviderInjector(provider, tzktProvider)
	return tezosProvider
}

func tezosProviderInjector(tezosProvider *tezos.Provider, tzktProvider *tzkt.Provider) *multichain.TezosProvider {
	multichainTezosProvider := &multichain.TezosProvider{
		ContractsCreatorFetcher:          tzktProvider,
		TokenDescriptorsFetcher:          tzktProvider,
		TokenMetadataFetcher:             tzktProvider,
		TokensIncrementalContractFetcher: tzktProvider,
		TokensIncrementalOwnerFetcher:    tzktProvider,
		TokenIdentifierOwnerFetcher:      tzktProvider,
		Verifier:                         tezosProvider,
	}
	return multichainTezosProvider
}

func optimismInjector(contextContext context.Context, client *http.Client) (*multichain.OptimismProvider, func()) {
	chain := _wirePersistChainValue
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := optimismSyncPipelineInjector(contextContext, client, chain, provider)
	optimismProvider := optimismProviderInjector(syncPipelineWrapper, provider)
	return optimismProvider, func() {
		cleanup()
	}
}

var (
	_wirePersistChainValue = persist.ChainOptimism
)

func optimismProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, simplehashProvider *simplehash.Provider) *multichain.OptimismProvider {
	optimismProvider := &multichain.OptimismProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             syncPipeline,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
	}
	return optimismProvider
}

func optimismSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func arbitrumInjector(contextContext context.Context, client *http.Client) (*multichain.ArbitrumProvider, func()) {
	chain := _wireChainValue2
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := arbitrumSyncPipelineInjector(contextContext, client, chain, provider)
	arbitrumProvider := arbitrumProviderInjector(syncPipelineWrapper, provider)
	return arbitrumProvider, func() {
		cleanup()
	}
}

var (
	_wireChainValue2 = persist.ChainArbitrum
)

func arbitrumProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, simplehashProvider *simplehash.Provider) *multichain.ArbitrumProvider {
	arbitrumProvider := &multichain.ArbitrumProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             syncPipeline,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
	}
	return arbitrumProvider
}

func arbitrumSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func poapInjector(serverEnvInit envInit, client *http.Client) *multichain.PoapProvider {
	provider := poap.NewProvider(client)
	poapProvider := poapProviderInjector(provider)
	return poapProvider
}

func poapProviderInjector(poapProvider *poap.Provider) *multichain.PoapProvider {
	multichainPoapProvider := &multichain.PoapProvider{
		TokenDescriptorsFetcher:       poapProvider,
		TokenMetadataFetcher:          poapProvider,
		TokensIncrementalOwnerFetcher: poapProvider,
		TokenIdentifierOwnerFetcher:   poapProvider,
	}
	return multichainPoapProvider
}

func zoraInjector(serverEnvInit envInit, contextContext context.Context, client *http.Client) (*multichain.ZoraProvider, func()) {
	chain := _wireChainValue3
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := zoraSyncPipelineInjector(contextContext, client, chain, provider)
	zoraProvider := zoraProviderInjector(syncPipelineWrapper, provider)
	return zoraProvider, func() {
		cleanup()
	}
}

var (
	_wireChainValue3 = persist.ChainZora
)

func zoraProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, simplehashProvider *simplehash.Provider) *multichain.ZoraProvider {
	zoraProvider := &multichain.ZoraProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             syncPipeline,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
	}
	return zoraProvider
}

func zoraSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func baseInjector(contextContext context.Context, client *http.Client) (*multichain.BaseProvider, func()) {
	chain := _wireChainValue4
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := baseSyncPipelineInjector(contextContext, client, chain, provider)
	baseProvider := baseProvidersInjector(syncPipelineWrapper, provider)
	return baseProvider, func() {
		cleanup()
	}
}

var (
	_wireChainValue4 = persist.ChainBase
)

func baseProvidersInjector(syncPipeline *wrapper.SyncPipelineWrapper, simplehashProvider *simplehash.Provider) *multichain.BaseProvider {
	baseProvider := &multichain.BaseProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             syncPipeline,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
	}
	return baseProvider
}

func baseSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func polygonInjector(contextContext context.Context, client *http.Client) (*multichain.PolygonProvider, func()) {
	chain := _wireChainValue5
	provider := simplehash.NewProvider(chain, client)
	syncPipelineWrapper, cleanup := polygonSyncPipelineInjector(contextContext, client, chain, provider)
	polygonProvider := polygonProvidersInjector(syncPipelineWrapper, provider)
	return polygonProvider, func() {
		cleanup()
	}
}

var (
	_wireChainValue5 = persist.ChainPolygon
)

func polygonProvidersInjector(syncPipeline *wrapper.SyncPipelineWrapper, simplehashProvider *simplehash.Provider) *multichain.PolygonProvider {
	polygonProvider := &multichain.PolygonProvider{
		ContractFetcher:                  simplehashProvider,
		ContractsCreatorFetcher:          simplehashProvider,
		TokenDescriptorsFetcher:          simplehashProvider,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokenMetadataFetcher:             simplehashProvider,
		TokensByContractWalletFetcher:    syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
	}
	return polygonProvider
}

func polygonSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, simplehashProvider *simplehash.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	customMetadataHandlers := customMetadataHandlersInjector()
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      simplehashProvider,
		TokensIncrementalOwnerFetcher:    simplehashProvider,
		TokensIncrementalContractFetcher: simplehashProvider,
		TokenMetadataBatcher:             simplehashProvider,
		TokensByTokenIdentifiersFetcher:  simplehashProvider,
		TokensByContractWalletFetcher:    simplehashProvider,
		CustomMetadataWrapper:            customMetadataHandlers,
	}
	return syncPipelineWrapper, func() {
	}
}

func submitTokenBatchInjector(contextContext context.Context, cache *redis.Cache) multichain.SubmitTokensF {
	client := task.NewClient(contextContext)
	tokenmanageTickToken := tickToken()
	manager := tokenmanage.New(contextContext, client, cache, tokenmanageTickToken)
	submitTokensF := submitBatch(manager)
	return submitTokensF
}

// inject.go:

// envInit is a type returned after setting up the environment
// Adding envInit as a dependency to a provider will ensure that the environment is set up prior
// to calling the provider
type envInit struct{}

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

func newQueries(p *pgxpool.Pool) *coredb.Queries {
	return coredb.New(p)
}

// New chains must be added here
func newProviderLookup(p *multichain.ChainProvider) multichain.ProviderLookup {
	return multichain.ProviderLookup{persist.ChainETH: p.Ethereum, persist.ChainTezos: p.Tezos, persist.ChainOptimism: p.Optimism, persist.ChainArbitrum: p.Arbitrum, persist.ChainPOAP: p.Poap, persist.ChainZora: p.Zora, persist.ChainBase: p.Base, persist.ChainPolygon: p.Polygon}
}

func newTokenManageCache() *redis.Cache {
	return redis.NewCache(redis.TokenManageCache)
}

func tickToken() tokenmanage.TickToken { return nil }

func submitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
