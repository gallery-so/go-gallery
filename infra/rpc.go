package infra

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
)

// Clients is a wrapper for the alchemy clients necessary for json RPC and contract interaction
type Clients struct {
	RPCClient *rpc.Client
	ETHClient *ethclient.Client
}

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

// NewRPC creates a new RPC client
func NewRPC() *Clients {
	client, err := rpc.Dial("wss://eth-mainnet.alchemyapi.io/v2/9jnT6CWDSYJGpQGMgG0tzJie829ChJjU")
	if err != nil {
		panic(err)
	}
	ethClient, err := ethclient.Dial("wss://eth-mainnet.alchemyapi.io/v2/9jnT6CWDSYJGpQGMgG0tzJie829ChJjU")
	if err != nil {
		panic(err)
	}
	return &Clients{
		RPCClient: client,
		ETHClient: ethClient,
	}
}

// GetTransfersFrom returns the transfers from the given address
func (r *Clients) GetTransfersFrom(address string, fromBlock string) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["fromAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func (r *Clients) GetTransfersTo(address string, fromBlock string) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["toAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetContractTransfers returns the transfers for a given contract
func (r *Clients) GetContractTransfers(address string, fromBlock string) ([]Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = fromBlock
	opts["contractAddresses"] = []string{address}
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Transfers, nil
}

// GetTokenContractMetadata returns the metadata for a given contract (without URI)
func (r *Clients) GetTokenContractMetadata(address string) (TokenMetadata, error) {
	result := &TokenMetadata{}

	err := r.RPCClient.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return TokenMetadata{}, err
	}

	return *result, nil
}

// GetTokenURI returns metadata URI for a given token address
func (r *Clients) GetTokenURI(address string, tokenID string) (string, error) {

	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721Metadata(contract, r.ETHClient)
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
func (r *Clients) GetERC721TokensForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*persist.ERC721, error) {
	allTransfers, err := getAllTransfersForWallet(pCtx, address, fromBlock, r, pRuntime)

	allTokens := map[string]*persist.ERC721{}
	ownedTokens := []*persist.ERC721{}

	allChan := make(chan *persist.ERC721)

	uris := &sync.Map{}
	metadatas := &sync.Map{}

	counter := int64(len(allTransfers))

	for _, t := range allTransfers {
		go func(transfer Transfer) {
			defer atomic.AddInt64(&counter, -1)
			if transfer.ERC721TokenID == "" {
				return
			}
			if transfer.RawContract.Address == "" {
				return
			}

			if _, ok := metadatas.Load(transfer.RawContract.Address); !ok {
				metadata, err := r.GetTokenContractMetadata(transfer.RawContract.Address)
				if err != nil {
					return // nil, err
				}
				metadatas.Store(transfer.RawContract.Address, metadata)
			}
			if _, ok := uris.Load(transfer.RawContract.Address + transfer.ERC721TokenID); !ok {
				uri, err := r.GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID)
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

	for {
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
		default:
		}
		if atomic.LoadInt64(&counter) == 0 {
			break
		}
	}

	allResult := make([]*persist.ERC721, len(allTokens))
	i := 0
	for _, v := range allTokens {
		go r.GetERC721TokensForContract(pCtx, v.TokenContract.Address, "0x0", pRuntime)
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
func (r *Clients) GetERC721TokensForContract(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*persist.ERC721, error) {

	allTransfers, err := r.GetContractTransfers(address, fromBlock)
	if err != nil {
		return nil, err
	}

	sort.Slice(allTransfers, func(i, j int) bool {
		return allTransfers[i].BlockNumber < allTransfers[j].BlockNumber
	})

	contractMetadata, err := r.GetTokenContractMetadata(address)
	if err != nil {
		return nil, err
	}

	tokens := map[string]*persist.ERC721{}

	tokenChan := make(chan *persist.ERC721)

	counter := int64(len(allTransfers))

	uris := &sync.Map{}
	for _, t := range allTransfers {
		go func(transfer Transfer) {
			defer atomic.AddInt64(&counter, -1)
			if transfer.ERC721TokenID == "" {
				return
			}
			if _, ok := uris.Load(transfer.ERC721TokenID); !ok {
				uri, err := r.GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID)
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

	for {
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
		default:
		}
		if atomic.LoadInt64(&counter) == 0 {
			break
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

func getAllTransfersForWallet(pCtx context.Context, address string, fromBlock string, r *Clients, pRuntime *runtime.Runtime) ([]Transfer, error) {
	from, err := r.GetTransfersFrom(address, fromBlock)
	if err != nil {
		return nil, err
	}

	to, err := r.GetTransfersTo(address, fromBlock)
	if err != nil {
		return nil, err
	}
	allTransfers := append(from, to...)
	sort.Slice(allTransfers, func(i, j int) bool {
		return allTransfers[i].BlockNumber < allTransfers[j].BlockNumber
	})
	return allTransfers, nil
}
