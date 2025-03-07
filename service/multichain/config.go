package multichain

import (
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
)

type ProviderLookup map[persist.Chain]any

type ChainProvider struct {
	Ethereum *EthereumProvider
	Tezos    *TezosProvider
	Optimism *OptimismProvider
	Arbitrum *ArbitrumProvider
	Poap     *PoapProvider
	Zora     *ZoraProvider
	Base     *BaseProvider
	Polygon  *PolygonProvider
}

type EthereumProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
	common.Verifier
}

type TezosProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
	common.Verifier
}

type OptimismProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
}

type ArbitrumProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
}

type PoapProvider struct {
	common.TokenDescriptorsFetcher
	common.TokenMetadataFetcher
	common.TokensIncrementalOwnerFetcher
	common.TokenIdentifierOwnerFetcher
}

type ZoraProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
}

type BaseProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
}

type PolygonProvider struct {
	common.ContractFetcher
	common.TokenDescriptorsFetcher
	common.TokenIdentifierOwnerFetcher
	common.TokenMetadataBatcher
	common.TokenMetadataFetcher
	common.TokensByTokenIdentifiersFetcher
	common.TokensIncrementalContractFetcher
	common.TokensIncrementalOwnerFetcher
}
