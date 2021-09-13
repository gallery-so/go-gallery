package infra

import (
	"errors"
	"math/big"
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/mikeydub/go-gallery/contracts"
)

// Clients is a wrapper for the alchemy clients necessary for json RPC and contract interaction
type Clients struct {
	RPCClient *rpc.Client
	ETHClient *ethclient.Client
}

// GetTransfersResponse represents the response from the RPC call for getting transfers
type GetTransfersResponse struct {
	Result *Transfers `json:"result"`
}

// GetMetadataResponse represents a response from the RPC call for getting token metadata
type GetMetadataResponse struct {
	Result *TokenMetadata `json:"result"`
}

// Transfers represents the transfers for a given rpc response
type Transfers struct {
	Transfers []*Transfer `json:"transfers"`
}

// Transfer represents a transfer from the RPC response
type Transfer struct {
	Category      string   `json:"category"`
	BlockNumber   int      `json:"blockNum"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Value         string   `json:"value"`
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

// ERC721 represents an ERC721 token
type ERC721 struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	TokenID  string `json:"tokenId"`
	TokenURI string `json:"tokenURI"`
	Owner    string `json:"owner"`
}

// NewRPC creates a new RPC client
func NewRPC() *Clients {
	client, err := rpc.Dial(os.Getenv("ALCHEMY_URL"))
	if err != nil {
		panic(err)
	}
	ethClient, err := ethclient.Dial(os.Getenv("ALCHEMY_URL"))
	if err != nil {
		panic(err)
	}
	return &Clients{
		RPCClient: client,
		ETHClient: ethClient,
	}
}

// GetTransfersFrom returns the transfers from the given address
func (r *Clients) GetTransfersFrom(address string) ([]*Transfer, error) {
	result := &GetTransfersResponse{}

	opts := map[string]interface{}{}
	opts["fromAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func (r *Clients) GetTransfersTo(address string) ([]*Transfer, error) {
	result := &GetTransfersResponse{}

	opts := map[string]interface{}{}
	opts["toAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Result.Transfers, nil
}

// GetTokenContractMetadata returns the metadata for a given contract (without URI)
func (r *Clients) GetTokenContractMetadata(address string) (*TokenMetadata, error) {
	result := &GetMetadataResponse{}

	err := r.RPCClient.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return nil, err
	}

	return result.Result, nil
}

// GetTokenURI returns metadata URI for a given token address
func (r *Clients) GetTokenURI(address string, tokenID string) (string, error) {

	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721Metadata(contract, r.ETHClient)
	if err != nil {
		return "", err
	}

	i := new(big.Int)
	i, success := i.SetString(tokenID, 16)
	if !success {
		return "", errors.New("tokenID is not a valid hex string")
	}
	tokenURI, err := instance.TokenURI(&bind.CallOpts{}, i)
	if err != nil {
		return "", err
	}

	return tokenURI, nil

}

// GetERC721Tokens returns the ERC721 token for the given
func (r *Clients) GetERC721Tokens(address string) ([]*ERC721, error) {
	from, err := r.GetTransfersFrom(address)
	if err != nil {
		return nil, err
	}
	to, err := r.GetTransfersTo(address)
	if err != nil {
		return nil, err
	}
	allTransfers := append(from, to...)

	sort.Slice(allTransfers, func(i, j int) bool {
		return allTransfers[i].BlockNumber < allTransfers[j].BlockNumber
	})

	tokens := map[string]*ERC721{}
	uris := map[string]string{}
	metadatas := map[string]*TokenMetadata{}
	for _, t := range allTransfers {
		if t.ERC721TokenID != "" {
			if t.RawContract.Address == "" {
				continue
			}
			if _, ok := tokens[t.RawContract.Address]; !ok {
				metadata, err := r.GetTokenContractMetadata(t.RawContract.Address)
				if err != nil {
					return nil, err
				}
				metadatas[t.RawContract.Address] = metadata
			}
			if t.To == address {
				if _, ok := uris[t.RawContract.Address+t.ERC721TokenID]; !ok {
					uri, err := r.GetTokenURI(t.RawContract.Address, t.ERC721TokenID)
					if err != nil {
						return nil, err
					}
					uris[t.RawContract.Address+t.ERC721TokenID] = uri
				}
				tokens[t.RawContract.Address+t.ERC721TokenID] = &ERC721{
					Address:  t.RawContract.Address,
					Name:     metadatas[t.RawContract.Address].Name,
					Symbol:   metadatas[t.RawContract.Address].Symbol,
					TokenID:  t.ERC721TokenID,
					TokenURI: uris[t.RawContract.Address+t.ERC721TokenID],
					Owner:    t.To,
				}
			} else {
				delete(tokens, t.RawContract.Address+t.ERC721TokenID)
			}
		}
	}
	result := []*ERC721{}
	for _, v := range tokens {
		result = append(result, v)
	}

	return result, nil
}
