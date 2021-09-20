package infra

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
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

// TokenContractMetadata represents a token contract's metadata
type TokenContractMetadata struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Logo   string `json:"logo"`
}

type uriWithMetadata struct {
	uri string
	md  persist.TokenMetadata
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
func GetTokenContractMetadata(address string, pRuntime *runtime.Runtime) (TokenContractMetadata, error) {
	result := &TokenContractMetadata{}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return TokenContractMetadata{}, err
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

// GetTokenMetadata parses and returns the NFT metadata for a given token URI
// TODO handle when the URI is an SVG or image
// TODO handle when the URI points directly to a file instead of a JSON metadata
func GetTokenMetadata(tokenURI string) (persist.TokenMetadata, error) {
	switch {
	case strings.HasPrefix(tokenURI, "data:application/json;base64,"):
		// decode the base64 encoded json
		decoded, err := base64.StdEncoding.DecodeString(tokenURI[len("data:application/json;base64,"):])
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		metadata := persist.TokenMetadata{}
		err = json.Unmarshal(decoded, &metadata)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		return metadata, nil
	case strings.HasPrefix(tokenURI, "ipfs://"):
		strip := strings.TrimPrefix(tokenURI, "ipfs://")
		again := strings.TrimPrefix(strip, "ipfs/")

		url := fmt.Sprintf("https://ipfs.io/ipfs/%s", again)
		resp, err := http.Get(url)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		defer resp.Body.Close()
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		metadata := persist.TokenMetadata{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		return metadata, nil
	case strings.HasPrefix(tokenURI, "https://") || strings.HasPrefix(tokenURI, "http://"):
		resp, err := http.Get(tokenURI)
		if err != nil {
			return persist.TokenMetadata{}, err
		}
		defer resp.Body.Close()
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		// parse the json
		metadata := persist.TokenMetadata{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		return metadata, nil
	default:
		return persist.TokenMetadata{}, nil
	}
}

// GetERC721TokensForWallet returns the ERC721 token for the given wallet address
func GetERC721TokensForWallet(pCtx context.Context, pAddress string, pFromBlock string, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "GetERC721TokensForWallet"})
	allTransfers, err := getAllTransfersForWallet(pCtx, pAddress, pFromBlock, pRuntime)

	// map of token contract address + token ID => token to keep track of ownership seeing as
	// tokens will appear more than once in the transfers
	allTokens := map[string]*persist.Token{}
	// all the tokens owned by `address`
	ownedTokens := []*persist.Token{}

	// channel that will receive complete tokens from goroutines that will fill out token data
	tokenChan := make(chan *persist.Token)
	// channel that will receive errors from goroutines that will fill out token data
	errChan := make(chan error)

	// map of token contract address + token ID => uriWithMetadata to prevent duplicate calls to the
	// blockchain for retrieving token URI
	tokenDetails := &sync.Map{}
	// map of token contract address => token metadata to prevent duplicate calls to the
	// blockchain for retrieving token metadata
	contractMetadatas := &sync.Map{}

	// spin up a goroutine for each transfer
	for _, t := range allTransfers {
		go func(transfer Transfer) {
			// required data for a token
			if transfer.ERC721TokenID == "" {
				errChan <- errors.New("no token ID found for token")
				return
			}
			if transfer.RawContract.Address == "" {
				errChan <- errors.New("no contract address found for token")
				return
			}

			if _, ok := contractMetadatas.Load(transfer.RawContract.Address); !ok {
				metadata, err := GetTokenContractMetadata(transfer.RawContract.Address, pRuntime)
				if err != nil {
					logger.WithFields(logrus.Fields{"section": "GetTokenContractMetadata", "contract": transfer.RawContract.Address}).Error(err)
					errChan <- err
					return
				}
				// spin up a goroutine for each contract to retrieve all other tokens from that contract
				// and store them in the db
				// go GetERC721TokensForContract(pCtx, transfer.RawContract.Address, pFromBlock, pRuntime)
				contractMetadatas.Store(transfer.RawContract.Address, metadata)
			}
			if _, ok := tokenDetails.Load(transfer.RawContract.Address + transfer.ERC721TokenID); !ok {
				uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
				if err != nil {
					logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": transfer.RawContract.Address, "tokenID": transfer.ERC721TokenID}).Error(err)
					errChan <- err
					return
				}
				metadata, err := GetTokenMetadata(uri)
				if err != nil {
					logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
					errChan <- err
					return
				}

				tokenDetails.Store(transfer.RawContract.Address+transfer.ERC721TokenID, uriWithMetadata{uri, metadata})
			}

			genericMetadata, _ := contractMetadatas.Load(transfer.RawContract.Address)
			metadata := genericMetadata.(TokenContractMetadata)
			genericURI, _ := tokenDetails.Load(transfer.RawContract.Address + transfer.ERC721TokenID)
			uri := genericURI.(uriWithMetadata)

			// get token ID in non-prefixed hex format from big int
			tokenID, err := util.NormalizeHex(transfer.ERC721TokenID)
			if err != nil {
				errChan <- err
				return
			}
			blockNum, err := util.NormalizeHex(transfer.ERC721TokenID)
			if err != nil {
				errChan <- err
				return
			}

			token := &persist.Token{
				TokenContract: persist.TokenContract{
					Address:   strings.ToLower(transfer.RawContract.Address),
					TokenName: metadata.Name,
					Symbol:    metadata.Symbol,
				},
				TokenID:        tokenID,
				TokenURI:       uri.uri,
				OwnerAddress:   strings.ToLower(transfer.To),
				PreviousOwners: []string{strings.ToLower(transfer.From)},
				LastBlockNum:   blockNum,
				TokenMetadata:  uri.md,
			}
			tokenChan <- token
		}(t)
	}

	for i := 0; i < len(allTransfers); i++ {
		select {
		case t := <-tokenChan:
			// add token to map of tokens if not there, otherwise update owner history, last block num,
			// and current owner
			if it, ok := allTokens[t.TokenContract.Address+t.TokenID]; ok {
				ownerHistory := append(t.PreviousOwners, it.PreviousOwners...)
				it.PreviousOwners = ownerHistory
				it.OwnerAddress = t.OwnerAddress
				it.LastBlockNum = t.LastBlockNum
			} else {
				allTokens[t.TokenContract.Address+t.TokenID] = t
			}
		case err := <-errChan:
			logger.Error(err)

		}
	}

	allResult := make([]*persist.Token, len(allTokens))
	i := 0
	for _, v := range allTokens {
		// add every token to the result to be upserted in db
		allResult[i] = v
		// only add token to owned tokens if it is owned by the wallet
		if strings.EqualFold(v.OwnerAddress, pAddress) {
			ownedTokens = append(ownedTokens, v)
		}
		i++
	}

	go func() {
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, allResult, pRuntime); err != nil {
			logger.Error(err)
		}
	}()

	return ownedTokens, nil
}

// GetERC721TokensForContract returns the ERC721 token for the given contract address
func GetERC721TokensForContract(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "GetERC721TokensForContract"})
	allTransfers, err := GetContractTransfers(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	sortByBlockNumber(allTransfers)

	contractMetadata, err := GetTokenContractMetadata(address, pRuntime)
	if err != nil {
		return nil, err
	}

	// map of tokenID => token
	tokens := map[string]*persist.Token{}

	// channel receiving fully filled tokens from goroutines
	tokenChan := make(chan *persist.Token)
	// channel receiving errors from goroutines
	errChan := make(chan error)

	// map of tokenID => uriWithMetadata to prevent duplicate queries
	uris := &sync.Map{}

	for _, t := range allTransfers {
		go func(transfer Transfer) {

			if transfer.ERC721TokenID == "" {
				errChan <- errors.New("no token ID found for token")
				return
			}
			if _, ok := uris.Load(transfer.ERC721TokenID); !ok {
				uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
				if err != nil {
					logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": transfer.RawContract.Address, "tokenID": transfer.ERC721TokenID}).Error(err)
					errChan <- err
					return
				}
				metadata, err := GetTokenMetadata(uri)
				if err != nil {
					logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
					errChan <- err
					return
				}
				uris.Store(transfer.ERC721TokenID, uriWithMetadata{uri, metadata})
			}

			tokenID, err := util.NormalizeHex(transfer.ERC721TokenID)
			if err != nil {
				errChan <- err
				return
			}

			blockNum, err := util.NormalizeHex(transfer.ERC721TokenID)
			if err != nil {
				errChan <- err
				return
			}

			genericURI, _ := uris.Load(transfer.ERC721TokenID)
			uri := genericURI.(uriWithMetadata)

			token := &persist.Token{
				TokenContract: persist.TokenContract{
					Address:   strings.ToLower(transfer.RawContract.Address),
					TokenName: contractMetadata.Name,
					Symbol:    contractMetadata.Symbol,
				},
				TokenID:        tokenID,
				TokenURI:       uri.uri,
				OwnerAddress:   strings.ToLower(transfer.To),
				PreviousOwners: []string{strings.ToLower(transfer.From)},
				LastBlockNum:   blockNum,
				TokenMetadata:  uri.md,
			}
			tokenChan <- token
		}(t)

	}

	for i := 0; i < len(allTransfers); i++ {
		select {
		case token := <-tokenChan:
			if it, ok := tokens[token.TokenID]; ok {
				// add token to map of tokens if not there, otherwise update owner history, last block num,
				// and current owner
				owners := append(token.PreviousOwners, it.PreviousOwners...)
				it.OwnerAddress = token.OwnerAddress
				it.PreviousOwners = owners
				it.LastBlockNum = token.LastBlockNum
			} else {
				tokens[token.TokenID] = token
			}
		case err := <-errChan:
			logger.Error(err)
		}
	}

	// add every token to the result to be upserted in db
	result := make([]*persist.Token, len(tokens))
	i := 0
	for _, v := range tokens {
		result[i] = v
		i++
	}

	// spin up a subscription to listen for new transfers
	if err = WatchTransfers(pCtx, address, pRuntime); err != nil {
		return nil, err
	}

	go func() {
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, result, pRuntime); err != nil {
			logger.Error(err)
		}
	}()

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
	sortByBlockNumber(allTransfers)
	return allTransfers, nil
}

// WatchTransfers watches for transfers from the given contract address with an optional list of tokenIDs
func WatchTransfers(pCtx context.Context, pContractAddress string, pRuntime *runtime.Runtime) error {

	// already subscribed, no need to error though
	if _, ok := pRuntime.InfraClients.TransferLogs[pContractAddress]; ok {
		return nil
	}

	contract := common.HexToAddress(pContractAddress)
	instance, err := contracts.NewIERC721(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return err
	}

	sub, err := instance.WatchTransfer(&bind.WatchOpts{Context: pCtx}, pRuntime.InfraClients.TransferLogs[pContractAddress], nil, nil, nil)
	if err != nil {
		return err
	}
	go HandleFutureTransfers(pCtx, pContractAddress, sub, pRuntime)
	return nil
}

// HandleFutureTransfers continually updates the database with new transfers from the given contract address
// with the given subscription
func HandleFutureTransfers(pCtx context.Context, pContractAddress string, pSub event.Subscription, pRuntime *runtime.Runtime) error {
	logger := logrus.WithFields(logrus.Fields{"event": "transfer", "contract": pContractAddress})
	for {
		select {
		case event := <-pRuntime.InfraClients.TransferLogs[pContractAddress]:
			logger.Debug(event)
			tokens, err := persist.TokenGetByTokenID(pCtx, pContractAddress, event.TokenId.Text(16), pRuntime)
			if err != nil {
				logger.Error(err)
				pSub.Unsubscribe()
				return err
			}
			if len(tokens) > 1 {
				err := fmt.Errorf("multiple tokens found for tokenID %s", event.TokenId.Text(16))
				logger.Error(err)
				pSub.Unsubscribe()
				return err
			}
			if len(tokens) == 0 {
				err := fmt.Errorf("no tokens found for tokenID %s", event.TokenId.Text(16))
				logger.Error(err)
				pSub.Unsubscribe()
				return err
			}
			token := tokens[0]
			update := &persist.TokenUpdateWithTransfer{
				OwnerAddress:   strings.ToLower(event.To.Hex()),
				PreviousOwners: append(token.PreviousOwners, token.OwnerAddress),
				LastBlockNum:   fmt.Sprintf("0x%x", event.Raw.BlockNumber),
			}
			if err = persist.TokenUpdateByID(pCtx, token.ID, update, pRuntime); err != nil {
				logger.Error(err)
				pSub.Unsubscribe()
				return err
			}
		case err := <-pSub.Err():
			if err != nil {
				logger.Error(err)
				pSub.Unsubscribe()
				return err
			}
		}
	}
}

func sortByBlockNumber(transfers []Transfer) {
	sort.Slice(transfers, func(i, j int) bool {
		b1, ok := new(big.Int).SetString(transfers[i].BlockNumber[2:], 16)
		if !ok || !b1.IsUint64() {
			return false
		}
		b2, ok := new(big.Int).SetString(transfers[j].BlockNumber[2:], 16)
		if !ok || !b2.IsUint64() {
			return false
		}
		return b1.Uint64() < b2.Uint64()
	})
}
