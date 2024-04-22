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
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	Verifier
}

type TezosProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
	Verifier
}

type OptimismProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
}

type ArbitrumProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
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
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
}

type BaseProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
}

type PolygonProvider struct {
	ContractFetcher
	ContractsCreatorFetcher
	TokenDescriptorsFetcher
	TokenIdentifierOwnerFetcher
	TokenMetadataBatcher
	TokenMetadataFetcher
	TokensByContractWalletFetcher
	TokensByTokenIdentifiersFetcher
	TokensIncrementalContractFetcher
	TokensIncrementalOwnerFetcher
}
