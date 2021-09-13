package infra

import (
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/rpc"
)

// RPC is a wrapper for the RPC client
type RPC struct {
	Client *rpc.Client
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
	Address string `json:"address"`
	Name    string `json:"name"`
	Symbol  string `json:"symbol"`
	TokenID string `json:"tokenId"`
	Owner   string `json:"owner"`
}

// NewRPC creates a new RPC client
func NewRPC() *RPC {
	client, err := rpc.Dial(os.Getenv("ALCHEMY_URL"))
	if err != nil {
		panic(err)
	}
	return &RPC{
		Client: client,
	}
}

// GetTransfersFrom returns the transfers from the given address
func (r *RPC) GetTransfersFrom(address string) ([]*Transfer, error) {
	result := &GetTransfersResponse{}

	opts := map[string]interface{}{}
	opts["fromAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.Client.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func (r *RPC) GetTransfersTo(address string) ([]*Transfer, error) {
	result := &GetTransfersResponse{}

	opts := map[string]interface{}{}
	opts["toAddress"] = address
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	err := r.Client.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	return result.Result.Transfers, nil
}

// GetTokenMetadata returns metadata for a given token address
func (r *RPC) GetTokenMetadata(address string) (*TokenMetadata, error) {
	result := &GetMetadataResponse{}

	err := r.Client.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return nil, err
	}

	return result.Result, nil

}

// GetERC721Tokens returns the ERC721 token for the given
func (r *RPC) GetERC721Tokens(address string) ([]*ERC721, error) {
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
	metadatas := map[string]*TokenMetadata{}
	for _, t := range allTransfers {
		if t.ERC721TokenID != "" {
			if t.RawContract.Address == "" {
				continue
			}
			if _, ok := tokens[t.RawContract.Address]; !ok {
				metadata, err := r.GetTokenMetadata(t.RawContract.Address)
				if err != nil {
					return nil, err
				}
				metadatas[t.RawContract.Address] = metadata
			}
			if t.To == address {
				tokens[t.RawContract.Address+t.ERC721TokenID] = &ERC721{
					Address: t.RawContract.Address,
					Name:    metadatas[t.RawContract.Address].Name,
					Symbol:  metadatas[t.RawContract.Address].Symbol,
					TokenID: t.ERC721TokenID,
					Owner:   t.To,
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
