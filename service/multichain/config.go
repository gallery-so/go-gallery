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
	ContractsFetcher
	ContractsOwnerFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractAddressAndOwnerFetcher
	TokensContractFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
	Verifier
}

type TezosProvider struct {
	ContractsOwnerFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
	Verifier
}

type OptimismProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}

type ArbitrumProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}

type PoapProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}

type ZoraProvider struct {
	ContractsFetcher
	ContractsOwnerFetcher
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}

type BaseProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}

type PolygonProvider struct {
	TokenDescriptorsFetcher
	TokenMetadataFetcher
	TokensContractFetcher
	TokensIncrementalOwnerFetcher
	TokensOwnerFetcher
}
