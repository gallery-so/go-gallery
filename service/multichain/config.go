package multichain

import (
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
	ContractRefresher
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
	Verifier
}

type TezosProvider struct {
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	Verifier
}

type OptimismProvider struct {
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
}

type ArbitrumProvider struct {
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
}

type PoapProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
}

type ZoraProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
}

type BaseProvider struct {
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
}

type PolygonProvider struct {
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokensByTokenIdentifiersFetcher
}
