//go:build wireinject
// +build wireinject

package multichain

import (
	"context"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/google/wire"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/multichain/custom"
	"github.com/mikeydub/go-gallery/service/multichain/poap"
	"github.com/mikeydub/go-gallery/service/multichain/simplehash"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/multichain/wrapper"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc/arweave"
	"github.com/mikeydub/go-gallery/service/rpc/ipfs"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/tokenmanage"
	"github.com/mikeydub/go-gallery/util"
)

// NewMultichainProvider is a wire injector that sets up a multichain provider instance
// ethClient.Client and task.Client are expensive to initialize so they're passed as an arg.
func NewMultichainProvider(context.Context, *postgres.Repositories, *db.Queries, *ethclient.Client, *task.Client, *redis.Cache) *Provider {
	panic(wire.Build(
		wire.Value(http.DefaultClient), // HTTP client shared between providers
		wire.Struct(new(ChainProvider), "*"),
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
	))
}

func multichainProviderInjector(ctx context.Context, repos *postgres.Repositories, q *db.Queries, chainProvider *ChainProvider, submitter *tokenmanage.TokenProcessingSubmitter) *Provider {
	panic(wire.Build(
		wire.Struct(new(Provider), "*"),
		wire.Bind(new(tokenmanage.Submitter), util.ToPointer(submitter)),
		newProviderLookup,
	))
}

// New chains must be added here
func newProviderLookup(p *ChainProvider) ProviderLookup {
	return ProviderLookup{
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

func customMetadataHandlersInjector(ethCleint *ethclient.Client) *custom.CustomMetadataHandlers {
	panic(wire.Build(
		custom.NewCustomMetadataHandlers,
		ipfs.NewShell,
		arweave.NewClient,
	))
}

func ethInjector(context.Context, *http.Client, *ethclient.Client) *EthereumProvider {
	panic(wire.Build(
		wire.Value(persist.ChainETH),
		ethProviderInjector,
		ethSyncPipelineInjector,
		ethVerifierInjector,
		simplehash.NewProvider,
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
) *EthereumProvider {
	panic(wire.Build(
		wire.Struct(new(EthereumProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.Verifier), util.ToPointer(verifier)),
	))
}

func ethSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func tezosInjector(*http.Client) *TezosProvider {
	wire.Build(
		tezosProviderInjector,
		wire.Value(persist.ChainTezos),
		tezos.NewProvider,
		simplehash.NewProvider,
	)
	return nil
}

func tezosProviderInjector(tezosProvider *tezos.Provider, simplehashProvider *simplehash.Provider) *TezosProvider {
	panic(wire.Build(
		wire.Struct(new(TezosProvider), "*"),
		wire.Bind(new(common.Verifier), util.ToPointer(tezosProvider)),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
	))
}

func optimismInjector(context.Context, *http.Client, *ethclient.Client) *OptimismProvider {
	panic(wire.Build(
		wire.Value(persist.ChainOptimism),
		simplehash.NewProvider,
		optimismProviderInjector,
		optimismSyncPipelineInjector,
	))
}

func optimismProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
) *OptimismProvider {
	panic(wire.Build(
		wire.Struct(new(OptimismProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func optimismSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func arbitrumInjector(context.Context, *http.Client, *ethclient.Client) *ArbitrumProvider {
	panic(wire.Build(
		wire.Value(persist.ChainArbitrum),
		simplehash.NewProvider,
		arbitrumProviderInjector,
		arbitrumSyncPipelineInjector,
	))
}

func arbitrumProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
) *ArbitrumProvider {
	panic(wire.Build(
		wire.Struct(new(ArbitrumProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func arbitrumSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func poapInjector(*http.Client) *PoapProvider {
	panic(wire.Build(
		poapProviderInjector,
		poap.NewProvider,
	))
}

func poapProviderInjector(poapProvider *poap.Provider) *PoapProvider {
	panic(wire.Build(
		wire.Struct(new(PoapProvider), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(poapProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(poapProvider)),
	))
}

func zoraInjector(context.Context, *http.Client, *ethclient.Client) *ZoraProvider {
	panic(wire.Build(
		wire.Value(persist.ChainZora),
		simplehash.NewProvider,
		zoraProviderInjector,
		zoraSyncPipelineInjector,
	))
}

func zoraProviderInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
) *ZoraProvider {
	panic(wire.Build(
		wire.Struct(new(ZoraProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func zoraSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func baseInjector(context.Context, *http.Client, *ethclient.Client) *BaseProvider {
	panic(wire.Build(
		wire.Value(persist.ChainBase),
		simplehash.NewProvider,
		baseProvidersInjector,
		baseSyncPipelineInjector,
	))
}

func baseProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *BaseProvider {
	panic(wire.Build(
		wire.Struct(new(BaseProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
	))
}

func baseSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func polygonInjector(context.Context, *http.Client, *ethclient.Client) *PolygonProvider {
	panic(wire.Build(
		wire.Value(persist.ChainPolygon),
		simplehash.NewProvider,
		polygonProvidersInjector,
		polygonSyncPipelineInjector,
	))
}

func polygonProvidersInjector(
	syncPipeline *wrapper.SyncPipelineWrapper,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *PolygonProvider {
	panic(wire.Build(
		wire.Struct(new(PolygonProvider), "*"),
		wire.Bind(new(common.ContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(syncPipeline)),
		wire.Bind(new(common.TokenDescriptorsFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.ContractsCreatorFetcher), util.ToPointer(simplehashProvider)),
	))
}

func polygonSyncPipelineInjector(
	ctx context.Context,
	httpClient *http.Client,
	chain persist.Chain,
	simplehashProvider *simplehash.Provider,
	ethClient *ethclient.Client,
) *wrapper.SyncPipelineWrapper {
	panic(wire.Build(
		wire.Struct(new(wrapper.SyncPipelineWrapper), "*"),
		wire.Bind(new(common.TokenIdentifierOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalOwnerFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensIncrementalContractFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokenMetadataBatcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByContractWalletFetcher), util.ToPointer(simplehashProvider)),
		wire.Bind(new(common.TokensByTokenIdentifiersFetcher), util.ToPointer(simplehashProvider)),
		customMetadataHandlersInjector,
	))
}

func tokenProcessingSubmitterInjector(context.Context, *task.Client, *redis.Cache) *tokenmanage.TokenProcessingSubmitter {
	panic(wire.Build(
		wire.Struct(new(tokenmanage.TokenProcessingSubmitter), "*"),
		wire.Struct(new(tokenmanage.Registry), "*"),
	))
}
