package infra

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

// Transfers represents the transfers for a given rpc response
type Transfers struct {
	Transfers []Transfer `json:"transfers"`
}

// Transfer represents a transfer from the RPC response
type Transfer struct {
	Category      string   `json:"category"`
	BlockNumber   string   `json:"blockNum"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Value         float64  `json:"value"`
	ERC721TokenID string   `json:"erc721TokenId"`
	Asset         string   `json:"asset"`
	Hash          string   `json:"hash"`
	RawContract   Contract `json:"rawContract"`
}

// Contract represents a contract that is interacted with during a transfer
type Contract struct {
	Address string `json:"address"`
	Value   string `json:"value"`
	Decimal string `json:"decimal"`
}

// TokenMetadata represents a token contract's metadata
type TokenMetadata struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Logo   string `json:"logo"`
}

// GetTransfersFrom returns the transfers from the given address
func GetTransfersFrom(address string, fromBlock string, pRuntime *runtime.Runtime) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["fromAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func GetTransfersTo(address string, fromBlock string, pRuntime *runtime.Runtime) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["toAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetContractTransfers returns the transfers for a given contract
func GetContractTransfers(address string, fromBlock string, pRuntime *runtime.Runtime) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["contractAddresses"] = []string{address}
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetTokenContractMetadata returns the metadata for a given contract (without URI)
func GetTokenContractMetadata(address string, pRuntime *runtime.Runtime) (TokenMetadata, error) {
	result := &TokenMetadata{}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return TokenMetadata{}, err
	}

	return *result, nil
}

// GetTokenURI returns metadata URI for a given token address
func GetTokenURI(address string, tokenID string, pRuntime *runtime.Runtime) (string, error) {

	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721Metadata(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return "", err
	}

	i := new(big.Int)
	_, success := i.SetString(tokenID[2:], 16)
	if !success {
		return "", errors.New("invalid hex string")
	}

	tokenURI, err := instance.TokenURI(&bind.CallOpts{}, i)
	if err != nil {
		return "", err
	}

	return tokenURI, nil

}

// GetERC721TokensForWallet returns the ERC721 token for the given wallet address
func GetERC721TokensForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*persist.ERC721, error) {
	allTransfers, err := getAllTransfersForWallet(pCtx, address, fromBlock, pRuntime)

	allTokens := map[string]*persist.ERC721{}
	ownedTokens := []*persist.ERC721{}

	allChan := make(chan *persist.ERC721)

	uris := &sync.Map{}
	metadatas := &sync.Map{}

	for _, t := range allTransfers {
		go func(transfer Transfer) {
			if transfer.ERC721TokenID == "" {
				return
			}
			if transfer.RawContract.Address == "" {
				return
			}

			if _, ok := metadatas.Load(transfer.RawContract.Address); !ok {
				metadata, err := GetTokenContractMetadata(transfer.RawContract.Address, pRuntime)
				if err != nil {
					return // nil, err
				}
				metadatas.Store(transfer.RawContract.Address, metadata)
			}
			if _, ok := uris.Load(transfer.RawContract.Address + transfer.ERC721TokenID); !ok {
				uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
				if err != nil {
					return // nil, err
				}
				uris.Store(transfer.RawContract.Address+transfer.ERC721TokenID, uri)
			}

			genericMetadata, _ := metadatas.Load(transfer.RawContract.Address)
			metadata := genericMetadata.(TokenMetadata)
			genericURI, _ := uris.Load(transfer.RawContract.Address + transfer.ERC721TokenID)
			uri := genericURI.(string)
			token := &persist.ERC721{
				TokenContract: persist.TokenContract{
					Address:   strings.ToLower(transfer.RawContract.Address),
					TokenName: metadata.Name,
					Symbol:    metadata.Symbol,
				},
				TokenID:        transfer.ERC721TokenID,
				TokenURI:       uri,
				OwnerAddress:   strings.ToLower(transfer.To),
				PreviousOwners: []string{strings.ToLower(transfer.From)},
				LastBlockNum:   transfer.BlockNumber,
			}
			allChan <- token
		}(t)
	}

	for i := 0; i < len(allTransfers); i++ {
		select {
		case all := <-allChan:
			if it, ok := allTokens[all.TokenContract.Address+all.TokenID]; ok {
				ownerHistory := append(all.PreviousOwners, it.PreviousOwners...)
				it.PreviousOwners = ownerHistory
				it.OwnerAddress = all.OwnerAddress
				it.LastBlockNum = all.LastBlockNum
			} else {
				allTokens[all.TokenContract.Address+all.TokenID] = all
			}
		}
	}

	allResult := make([]*persist.ERC721, len(allTokens))
	i := 0
	for _, v := range allTokens {
		go GetERC721TokensForContract(pCtx, v.TokenContract.Address, fromBlock, pRuntime)
		allResult[i] = v
		if strings.EqualFold(v.OwnerAddress, address) {
			ownedTokens = append(ownedTokens, v)
		}
		i++
	}

	if err = persist.ERC721BulkUpsert(pCtx, allResult, pRuntime); err != nil {
		return nil, err
	}

	return ownedTokens, nil
}

// GetERC721TokensForContract returns the ERC721 token for the given contract address
func GetERC721TokensForContract(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*persist.ERC721, error) {

	allTransfers, err := GetContractTransfers(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	sort.Slice(allTransfers, func(i, j int) bool {
		b1, ok := new(big.Int).SetString(allTransfers[i].BlockNumber[2:], 16)
		if !ok || b1.IsUint64() {
			panic("invalid block number")
			return false
		}
		b2, ok := new(big.Int).SetString(allTransfers[j].BlockNumber[2:], 16)
		if !ok || !b2.IsUint64() {
			panic("invalid block number")
			return false
		}
		return b1.Uint64() < b2.Uint64()
	})

	contractMetadata, err := GetTokenContractMetadata(address, pRuntime)
	if err != nil {
		return nil, err
	}

	tokens := map[string]*persist.ERC721{}

	tokenChan := make(chan *persist.ERC721)

	uris := &sync.Map{}
	for _, t := range allTransfers {
		go func(transfer Transfer) {

			if transfer.ERC721TokenID == "" {
				return
			}
			if _, ok := uris.Load(transfer.ERC721TokenID); !ok {
				uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
				if err != nil {
					return // nil, err
				}
				uris.Store(transfer.ERC721TokenID, uri)
			}

			genericURI, _ := uris.Load(transfer.ERC721TokenID)
			uri := genericURI.(string)
			token := &persist.ERC721{
				TokenContract: persist.TokenContract{
					Address:   strings.ToLower(transfer.RawContract.Address),
					TokenName: contractMetadata.Name,
					Symbol:    contractMetadata.Symbol,
				},
				TokenID:        transfer.ERC721TokenID,
				TokenURI:       uri,
				OwnerAddress:   strings.ToLower(transfer.To),
				PreviousOwners: []string{strings.ToLower(transfer.From)},
				LastBlockNum:   transfer.BlockNumber,
			}
			tokenChan <- token
		}(t)

	}

	for i := 0; i < len(allTransfers); i++ {
		select {
		case token := <-tokenChan:
			if it, ok := tokens[token.TokenID]; ok {
				owners := append(token.PreviousOwners, it.PreviousOwners...)
				it.OwnerAddress = token.OwnerAddress
				it.PreviousOwners = owners
				it.LastBlockNum = token.LastBlockNum
			} else {
				tokens[token.TokenID] = token
			}
		}
	}

	result := make([]*persist.ERC721, len(tokens))
	i := 0
	for _, v := range tokens {
		result[i] = v
		i++
	}

	if err = persist.ERC721BulkUpsert(pCtx, result, pRuntime); err != nil {
		return nil, err
	}

	return result, nil
}

func getAllTransfersForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]Transfer, error) {
	from, err := GetTransfersFrom(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	to, err := GetTransfersTo(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}
	allTransfers := append(from, to...)
	sort.Slice(allTransfers, func(i, j int) bool {
		b1, ok := new(big.Int).SetString(allTransfers[i].BlockNumber[2:], 16)
		if !ok || b1.IsUint64() {
			panic("invalid block number")
			return false
		}
		b2, ok := new(big.Int).SetString(allTransfers[j].BlockNumber[2:], 16)
		if !ok || !b2.IsUint64() {
			panic("invalid block number")
			return false
		}
		return b1.Uint64() < b2.Uint64()
	})
	return allTransfers, nil
}

// NewSubscription sets up a new subscription for a set of addresses from a given block with the given topics
func NewSubscription(pCtx context.Context, fromBlock string, addresses []common.Address, topics [][]common.Hash, pRuntime *runtime.Runtime) error {
	i := new(big.Int)
	_, ok := i.SetString(fromBlock[2:], 16)
	if !ok {
		return errors.New("invalid block number")
	}
	sub, err := pRuntime.InfraClients.ETHClient.SubscribeFilterLogs(pCtx, ethereum.FilterQuery{FromBlock: i, Addresses: addresses, Topics: topics}, pRuntime.InfraClients.SubLogs)
	if err != nil {
		return err
	}
	go func() {
		select {
		case err := <-sub.Err():
			if err != nil {
				log.Error(err.Error())
				sub.Unsubscribe()
			}
		}
	}()
	return nil
}
