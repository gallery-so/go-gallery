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
	cache := newCommunitiesCache()
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
	client := task.NewClient(contextContext)
	manager := tokenmanage.New(contextContext, client, cache)
	submitTokensF := newSubmitBatch(manager)
	providerLookup := newProviderLookup(chainProvider)
	provider := &multichain.Provider{
		Repos:        repositories,
		Queries:      queries,
		SubmitTokens: submitTokensF,
		Chains:       providerLookup,
	}
	return provider
}

func customMetadataHandlersInjector(alchemyProvider *alchemy.Provider) *multichain.CustomMetadataHandlers {
	client := rpc.NewEthClient()
	shell := ipfs.NewShell()
	goarClient := arweave.NewClient()
	customMetadataHandlers := multichain.NewCustomMetadataHandlers(client, shell, goarClient, alchemyProvider)
	return customMetadataHandlers
}

func ethInjector(serverEnvInit envInit, contextContext context.Context, client *http.Client) (*multichain.EthereumProvider, func()) {
	ethclientClient := rpc.NewEthClient()
	provider := indexer.NewProvider(client, ethclientClient)
	chain := _wireChainValue
	openseaProvider, cleanup := opensea.NewProvider(contextContext, client, chain)
	alchemyProvider := alchemy.NewProvider(client, chain)
	syncPipelineWrapper, cleanup2 := ethSyncPipelineInjector(contextContext, client, chain, openseaProvider, alchemyProvider)
	contractFetcher := ethContractFetcherInjector(openseaProvider, alchemyProvider)
	tokenDescriptorsFetcher := ethTokenDescriptorsFetcherInjector(openseaProvider, alchemyProvider)
	tokenMetadataFetcher := ethTokenMetadataFetcherInjector(openseaProvider, alchemyProvider)
	ethereumProvider := ethProviderInjector(contextContext, provider, syncPipelineWrapper, contractFetcher, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return ethereumProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wireChainValue = persist.ChainETH
)

func ethProviderInjector(ctx context.Context, indexerProvider *indexer.Provider, syncPipeline *wrapper.SyncPipelineWrapper, contractFetcher multichain.ContractFetcher, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.EthereumProvider {
	ethereumProvider := &multichain.EthereumProvider{
		ContractRefresher:                indexerProvider,
		ContractFetcher:                  contractFetcher,
		ContractsOwnerFetcher:            indexerProvider,
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
		Verifier:                         indexerProvider,
	}
	return ethereumProvider
}

func ethSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := ethTokenIdentifierOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalOwnerFetcher := ethTokensIncrementalOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalContractFetcher := ethTokensContractFetcherInjector(openseaProvider, alchemyProvider)
	tokensByTokenIdentifiersFetcher := ethTokenByTokenIdentifiersFetcherInjector(openseaProvider, alchemyProvider)
	customMetadataHandlers := customMetadataHandlersInjector(alchemyProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             alchemyProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func ethTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalContractFetcher
}

func ethTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokenIdentifierOwnerFetcher
}

func ethTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalOwnerFetcher
}

func ethContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.ContractFetcher {
	contractFetcher := multiContractFetcherProvider(alchemyProvider, openseaProvider)
	return contractFetcher
}

func ethTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(alchemyProvider, openseaProvider)
	return tokenMetadataFetcher
}

func ethTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(alchemyProvider, openseaProvider)
	return tokenDescriptorsFetcher
}

func ethTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(alchemyProvider, openseaProvider)
	return tokensByTokenIdentifiersFetcher
}

func tezosInjector(serverEnvInit envInit, client *http.Client) *multichain.TezosProvider {
	provider := tezos.NewProvider()
	tzktProvider := tzkt.NewProvider(client)
	tezosProvider := tezosProviderInjector(provider, tzktProvider)
	return tezosProvider
}

func tezosProviderInjector(tezosProvider *tezos.Provider, tzktProvider *tzkt.Provider) *multichain.TezosProvider {
	multichainTezosProvider := &multichain.TezosProvider{
		ContractsOwnerFetcher:            tzktProvider,
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
	provider, cleanup := opensea.NewProvider(contextContext, client, chain)
	alchemyProvider := alchemy.NewProvider(client, chain)
	syncPipelineWrapper, cleanup2 := optimismSyncPipelineInjector(contextContext, client, chain, provider, alchemyProvider)
	tokenDescriptorsFetcher := optimisimTokenDescriptorsFetcherInjector(provider, alchemyProvider)
	tokenMetadataFetcher := optimismTokenMetadataFetcherInjector(provider, alchemyProvider)
	optimismProvider := optimismProviderInjector(syncPipelineWrapper, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return optimismProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wirePersistChainValue = persist.ChainOptimism
)

func optimismProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.OptimismProvider {
	optimismProvider := &multichain.OptimismProvider{
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
	}
	return optimismProvider
}

func optimismSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := optimismTokenIdentifierOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalOwnerFetcher := optimismTokensIncrementalOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalContractFetcher := optimismTokensContractFetcherInjector(openseaProvider, alchemyProvider)
	tokensByTokenIdentifiersFetcher := optmismTokenByTokenIdentifiersFetcherInjector(openseaProvider, alchemyProvider)
	customMetadataHandlers := customMetadataHandlersInjector(alchemyProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             alchemyProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func optimismTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalContractFetcher
}

func optimismTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokenIdentifierOwnerFetcher
}

func optimismTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalOwnerFetcher
}

func optimismTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(alchemyProvider, openseaProvider)
	return tokenMetadataFetcher
}

func optimisimTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(alchemyProvider, openseaProvider)
	return tokenDescriptorsFetcher
}

func optmismTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(alchemyProvider, openseaProvider)
	return tokensByTokenIdentifiersFetcher
}

func arbitrumInjector(contextContext context.Context, client *http.Client) (*multichain.ArbitrumProvider, func()) {
	chain := _wireChainValue2
	provider, cleanup := opensea.NewProvider(contextContext, client, chain)
	alchemyProvider := alchemy.NewProvider(client, chain)
	syncPipelineWrapper, cleanup2 := arbitrumSyncPipelineInjector(contextContext, client, chain, provider, alchemyProvider)
	tokenDescriptorsFetcher := arbitrumTokenDescriptorsFetcherInjector(provider, alchemyProvider)
	tokenMetadataFetcher := arbitrumTokenMetadataFetcherInjector(provider, alchemyProvider)
	arbitrumProvider := arbitrumProviderInjector(syncPipelineWrapper, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return arbitrumProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wireChainValue2 = persist.ChainArbitrum
)

func arbitrumProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.ArbitrumProvider {
	arbitrumProvider := &multichain.ArbitrumProvider{
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
	}
	return arbitrumProvider
}

func arbitrumSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := arbitrumTokenIdentifierOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalOwnerFetcher := arbitrumTokensIncrementalOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalContractFetcher := arbitrumTokensContractFetcherInjector(openseaProvider, alchemyProvider)
	tokensByTokenIdentifiersFetcher := arbitrumTokenByTokenIdentifiersFetcherInjector(openseaProvider, alchemyProvider)
	customMetadataHandlers := customMetadataHandlersInjector(alchemyProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             alchemyProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func arbitrumTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(alchemyProvider, openseaProvider)
	return tokenMetadataFetcher
}

func arbitrumTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(alchemyProvider, openseaProvider)
	return tokenDescriptorsFetcher
}

func arbitrumTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalContractFetcher
}

func arbitrumTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokenIdentifierOwnerFetcher
}

func arbitrumTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalOwnerFetcher
}

func arbitrumTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(alchemyProvider, openseaProvider)
	return tokensByTokenIdentifiersFetcher
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
	provider, cleanup := opensea.NewProvider(contextContext, client, chain)
	zoraProvider := zora.NewProvider(client)
	syncPipelineWrapper, cleanup2 := zoraSyncPipelineInjector(contextContext, client, chain, provider, zoraProvider)
	contractFetcher := zoraContractFetcherInjector(provider, zoraProvider)
	tokenDescriptorsFetcher := zoraTokenDescriptorsFetcherInjector(provider, zoraProvider)
	tokenMetadataFetcher := zoraTokenMetadataFetcherInjector(provider, zoraProvider)
	multichainZoraProvider := zoraProviderInjector(syncPipelineWrapper, zoraProvider, contractFetcher, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return multichainZoraProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wireChainValue3 = persist.ChainZora
)

func zoraProviderInjector(syncPipeline *wrapper.SyncPipelineWrapper, zoraProvider *zora.Provider, contractFetcher multichain.ContractFetcher, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.ZoraProvider {
	multichainZoraProvider := &multichain.ZoraProvider{
		ContractFetcher:                  contractFetcher,
		ContractsOwnerFetcher:            zoraProvider,
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
	}
	return multichainZoraProvider
}

func zoraTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(openseaProvider, zoraProvider)
	return tokenMetadataFetcher
}

func zoraTokenDescriptorsFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(openseaProvider, zoraProvider)
	return tokenDescriptorsFetcher
}

func zoraContractFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.ContractFetcher {
	contractFetcher := multiContractFetcherProvider(openseaProvider, zoraProvider)
	return contractFetcher
}

func zoraSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, zoraProvider *zora.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := zoraTokenIdentifierOwnerFetcherInjector(openseaProvider, zoraProvider)
	tokensIncrementalOwnerFetcher := zoraTokensIncrementalOwnerFetcherInjector(openseaProvider, zoraProvider)
	tokensIncrementalContractFetcher := zoraTokensContractFetcherInjector(openseaProvider, zoraProvider)
	tokensByTokenIdentifiersFetcher := zoraTokenByTokenIdentifiersFetcherInjector(openseaProvider, zoraProvider)
	customMetadataHandlers := zoraCustomMetadataHandlersInjector(openseaProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             zoraProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func zoraCustomMetadataHandlersInjector(openseaProvider *opensea.Provider) *multichain.CustomMetadataHandlers {
	client := rpc.NewEthClient()
	shell := ipfs.NewShell()
	goarClient := arweave.NewClient()
	customMetadataHandlers := multichain.NewCustomMetadataHandlers(client, shell, goarClient, openseaProvider)
	return customMetadataHandlers
}

func zoraTokensContractFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(openseaProvider, zoraProvider)
	return tokensIncrementalContractFetcher
}

func zoraTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(openseaProvider, zoraProvider)
	return tokenIdentifierOwnerFetcher
}

func zoraTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(openseaProvider, zoraProvider)
	return tokensIncrementalOwnerFetcher
}

func zoraTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, zoraProvider *zora.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(openseaProvider, zoraProvider)
	return tokensByTokenIdentifiersFetcher
}

func baseInjector(contextContext context.Context, client *http.Client) (*multichain.BaseProvider, func()) {
	chain := _wireChainValue4
	provider, cleanup := opensea.NewProvider(contextContext, client, chain)
	alchemyProvider := alchemy.NewProvider(client, chain)
	syncPipelineWrapper, cleanup2 := baseSyncPipelineInjector(contextContext, client, chain, provider, alchemyProvider)
	tokenDescriptorsFetcher := baseTokenDescriptorFetcherInjector(provider, alchemyProvider)
	tokenMetadataFetcher := baseTokenMetadataFetcherInjector(provider, alchemyProvider)
	baseProvider := baseProvidersInjector(syncPipelineWrapper, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return baseProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wireChainValue4 = persist.ChainBase
)

func baseProvidersInjector(syncPipeline *wrapper.SyncPipelineWrapper, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.BaseProvider {
	baseProvider := &multichain.BaseProvider{
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
	}
	return baseProvider
}

func baseSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := baseTokenIdentifierOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalOwnerFetcher := baseTokensIncrementalOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalContractFetcher := baseTokensContractFetcherInjector(openseaProvider, alchemyProvider)
	tokensByTokenIdentifiersFetcher := baseTokenByTokenIdentifiersFetcherInjector(openseaProvider, alchemyProvider)
	customMetadataHandlers := customMetadataHandlersInjector(alchemyProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             alchemyProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func baseTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(alchemyProvider, openseaProvider)
	return tokenMetadataFetcher
}

func baseTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(alchemyProvider, openseaProvider)
	return tokenDescriptorsFetcher
}

func baseTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalContractFetcher
}

func baseTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokenIdentifierOwnerFetcher
}

func baseTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalOwnerFetcher
}

func baseTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(alchemyProvider, openseaProvider)
	return tokensByTokenIdentifiersFetcher
}

func polygonInjector(contextContext context.Context, client *http.Client) (*multichain.PolygonProvider, func()) {
	chain := _wireChainValue5
	provider, cleanup := opensea.NewProvider(contextContext, client, chain)
	alchemyProvider := alchemy.NewProvider(client, chain)
	syncPipelineWrapper, cleanup2 := polygonSyncPipelineInjector(contextContext, client, chain, provider, alchemyProvider)
	tokenDescriptorsFetcher := polygonTokenDescriptorFetcherInjector(provider, alchemyProvider)
	tokenMetadataFetcher := polygonTokenMetadataFetcherInjector(provider, alchemyProvider)
	polygonProvider := polygonProvidersInjector(syncPipelineWrapper, tokenDescriptorsFetcher, tokenMetadataFetcher)
	return polygonProvider, func() {
		cleanup2()
		cleanup()
	}
}

var (
	_wireChainValue5 = persist.ChainPolygon
)

func polygonSyncPipelineInjector(ctx context.Context, httpClient *http.Client, chain persist.Chain, openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) (*wrapper.SyncPipelineWrapper, func()) {
	tokenIdentifierOwnerFetcher := polygonTokenIdentifierOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalOwnerFetcher := polygonTokensIncrementalOwnerFetcherInjector(openseaProvider, alchemyProvider)
	tokensIncrementalContractFetcher := polygonTokensContractFetcherInjector(openseaProvider, alchemyProvider)
	tokensByTokenIdentifiersFetcher := polygonTokenByTokenIdentifiersFetcherInjector(openseaProvider, alchemyProvider)
	customMetadataHandlers := customMetadataHandlersInjector(alchemyProvider)
	fillInWrapper, cleanup := wrapper.NewFillInWrapper(ctx, httpClient, chain)
	syncPipelineWrapper := &wrapper.SyncPipelineWrapper{
		Chain:                            chain,
		TokenIdentifierOwnerFetcher:      tokenIdentifierOwnerFetcher,
		TokensIncrementalOwnerFetcher:    tokensIncrementalOwnerFetcher,
		TokensIncrementalContractFetcher: tokensIncrementalContractFetcher,
		TokenMetadataBatcher:             alchemyProvider,
		TokensByTokenIdentifiersFetcher:  tokensByTokenIdentifiersFetcher,
		CustomMetadataWrapper:            customMetadataHandlers,
		FillInWrapper:                    fillInWrapper,
	}
	return syncPipelineWrapper, func() {
		cleanup()
	}
}

func polygonTokenMetadataFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenMetadataFetcher {
	tokenMetadataFetcher := multiTokenMetadataFetcherProvider(alchemyProvider, openseaProvider)
	return tokenMetadataFetcher
}

func polygonTokenDescriptorFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenDescriptorsFetcher {
	tokenDescriptorsFetcher := multiTokenDescriptorsFetcherProvider(alchemyProvider, openseaProvider)
	return tokenDescriptorsFetcher
}

func polygonTokensContractFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalContractFetcher {
	tokensIncrementalContractFetcher := multiTokensIncrementalContractFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalContractFetcher
}

func polygonTokenIdentifierOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokenIdentifierOwnerFetcher {
	tokenIdentifierOwnerFetcher := multiTokenIdentifierOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokenIdentifierOwnerFetcher
}

func polygonTokensIncrementalOwnerFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensIncrementalOwnerFetcher {
	tokensIncrementalOwnerFetcher := multiTokensIncrementalOwnerFetcherProvider(alchemyProvider, openseaProvider)
	return tokensIncrementalOwnerFetcher
}

func polygonProvidersInjector(syncPipeline *wrapper.SyncPipelineWrapper, tokenDescriptorsFetcher multichain.TokenDescriptorsFetcher, tokenMetadataFetcher multichain.TokenMetadataFetcher) *multichain.PolygonProvider {
	polygonProvider := &multichain.PolygonProvider{
		TokenDescriptorsFetcher:          tokenDescriptorsFetcher,
		TokenMetadataFetcher:             tokenMetadataFetcher,
		TokensIncrementalContractFetcher: syncPipeline,
		TokensIncrementalOwnerFetcher:    syncPipeline,
		TokenIdentifierOwnerFetcher:      syncPipeline,
		TokenMetadataBatcher:             syncPipeline,
		TokensByTokenIdentifiersFetcher:  syncPipeline,
	}
	return polygonProvider
}

func polygonTokenByTokenIdentifiersFetcherInjector(openseaProvider *opensea.Provider, alchemyProvider *alchemy.Provider) multichain.TokensByTokenIdentifiersFetcher {
	tokensByTokenIdentifiersFetcher := multiTokenByTokenIdentifiersFetcherProvider(alchemyProvider, openseaProvider)
	return tokensByTokenIdentifiersFetcher
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

func newProviderLookup(p *multichain.ChainProvider) multichain.ProviderLookup {
	return multichain.ProviderLookup{persist.ChainETH: p.Ethereum, persist.ChainTezos: p.Tezos, persist.ChainOptimism: p.Optimism, persist.ChainArbitrum: p.Arbitrum, persist.ChainPOAP: p.Poap, persist.ChainZora: p.Zora, persist.ChainBase: p.Base, persist.ChainPolygon: p.Polygon}
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

func newCommunitiesCache() *redis.Cache {
	return redis.NewCache(redis.CommunitiesCache)
}

func newSubmitBatch(tm *tokenmanage.Manager) multichain.SubmitTokensF {
	return tm.SubmitBatch
}
