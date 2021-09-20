package infra

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/queue"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

const pageLength = 50

// Transfers represents the transfers for a given rpc response
type Transfers struct {
	PageKey   string      `json:"pageKey"`
	Transfers []*Transfer `json:"transfers"`
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
	md  map[string]interface{}
}

// GetTransfersFrom returns the transfers from the given address
func GetTransfersFrom(pAddress, pFromBlock string, pPageNumber int, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + pFromBlock
	opts["fromAddress"] = pAddress
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	if pPageKey != "" {
		opts["pageKey"] = pPageKey
	}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Total Transfers From: %d", len(result.Transfers))
	if len(result.Transfers) < pageLength*pPageNumber && len(result.Transfers) == 1000 {
		return GetTransfersFrom(pAddress, pFromBlock, pPageNumber, result.PageKey, pRuntime)
	}

	return result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func GetTransfersTo(pAddress, pFromBlock string, pPageNumber int, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + pFromBlock
	opts["toAddress"] = pAddress
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false
	if pPageKey != "" {
		opts["pageKey"] = pPageKey
	}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Total Transfers To: %d", len(result.Transfers))
	if len(result.Transfers) < pageLength*pPageNumber && len(result.Transfers) == 1000 {
		return GetTransfersTo(pAddress, pFromBlock, pPageNumber, result.PageKey, pRuntime)
	}

	return result.Transfers, nil
}

// GetContractTransfers returns the transfers for a given contract
func GetContractTransfers(pAddress, pFromBlock string, pPageNumber int, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
	result := &Transfers{}

	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + pFromBlock
	opts["contractAddresses"] = []string{pAddress}
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false

	if pPageKey != "" {
		opts["pageKey"] = pPageKey
	}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Total Transfers Contract: %d", len(result.Transfers))
	if len(result.Transfers) < pageLength*pPageNumber && len(result.Transfers) == 1000 {
		return GetTransfersTo(pAddress, pFromBlock, pPageNumber, result.PageKey, pRuntime)
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
func GetTokenURI(address, tokenID string, pRuntime *runtime.Runtime) (string, error) {

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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	tokenURI, err := instance.TokenURI(&bind.CallOpts{
		Context: ctx,
	}, i)
	if err != nil {
		return "", err
	}

	return tokenURI, nil

}

// GetTokenMetadata parses and returns the NFT metadata for a given token URI
// TODO timeout requests
// TODO handle when the URI is an SVG or image
// TODO handle when the URI points directly to a file instead of a JSON metadata
func GetTokenMetadata(tokenURI string, pRuntime *runtime.Runtime) (map[string]interface{}, error) {

	client := &http.Client{
		Timeout: time.Second * 2,
	}
	switch {
	case strings.HasPrefix(tokenURI, "data:application/json;base64,"):
		// decode the base64 encoded json
		decoded, err := base64.StdEncoding.DecodeString(tokenURI[len("data:application/json;base64,"):])
		if err != nil {
			return nil, err
		}

		metadata := map[string]interface{}{}
		err = json.Unmarshal(decoded, &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	case strings.HasPrefix(tokenURI, "ipfs://") || strings.HasPrefix(tokenURI, "https://ipfs.io/ipfs"):
		first := strings.TrimPrefix(tokenURI, "https://ipfs.io")
		second := strings.TrimPrefix(first, "/")
		third := strings.TrimPrefix(second, "ipfs://")
		final := strings.TrimPrefix(third, "ipfs/")

		it, err := pRuntime.IPFS.ObjectGet(final)
		if err != nil {
			return nil, err
		}
		metadata := map[string]interface{}{}
		err = json.Unmarshal([]byte(it.Data), &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	case strings.HasPrefix(tokenURI, "https://") || strings.HasPrefix(tokenURI, "http://"):
		resp, err := client.Get(tokenURI)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, resp.Body)
		if err != nil {
			return nil, err
		}

		// parse the json
		metadata := map[string]interface{}{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	default:
		return nil, errors.New("invalid token URI")
	}

}

// getTokensFromBCForWallet returns the ERC721 token for the given wallet address
func getTokensFromBCForWallet(pCtx context.Context, pAddress string, pPageNumber int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "GetERC721TokensForWallet"})
	allTransfers, err := getAllTransfersForWallet(pCtx, pAddress, pFromBlock, pPageNumber, pRuntime)
	if err != nil {
		return nil, err
	}
	total, start := setupPagination(pPageNumber, pageLength, len(allTransfers))

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

	finalBlockNum := "0"
	mu := &sync.Mutex{}
	// spin up a goroutine for each transfer
	for i := start; i < start+total; i++ {
		if allTransfers[i].ERC721TokenID == "" {
			if len(allTransfers) > total+i {
				start++
				continue
			} else {
				continue
			}
		}
		go func(transfer *Transfer) {
			token, blockNum, err := processWalletTransfer(contractMetadatas, tokenDetails, transfer, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			if blockNum > finalBlockNum {
				finalBlockNum = blockNum
			}
			mu.Unlock()
			tokenChan <- token
		}(allTransfers[i])
	}

	// map of token contract address + token ID => token to keep track of ownership seeing as
	// tokens will appear more than once in the transfers
	allTokens := receieveTransfers(tokenChan, errChan, total, logger)

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
		user, _ := persist.UserGetByAddress(pCtx, pAddress, pRuntime)
		if user != nil {
			for _, v := range allResult {
				v.OwnerUserID = user.ID
			}
		}
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, allResult, pRuntime); err != nil {
			logger.Error(err)
		}
		if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
			Address:         pAddress,
			LastSyncedBlock: finalBlockNum,
		}, pRuntime); err != nil {
			logger.Error(err)
		}
		if pQueueUpdate {
			queueUpdateForWallet(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		}
	}()

	return ownedTokens, nil
}

// getTokensFromBCForContract returns the ERC721 token for the given contract address
func getTokensFromBCForContract(pCtx context.Context, pAddress string, pPageNumber int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "GetERC721TokensForContract"})
	allTransfers, err := GetContractTransfers(pAddress, pFromBlock, pPageNumber, "", pRuntime)
	if err != nil {
		return nil, err
	}
	total, start := setupPagination(pPageNumber, pageLength, len(allTransfers))

	sortByBlockNumber(allTransfers)

	contractMetadata, err := GetTokenContractMetadata(pAddress, pRuntime)
	if err != nil {
		return nil, err
	}

	// channel receiving fully filled tokens from goroutines
	tokenChan := make(chan *persist.Token)
	// channel receiving errors from goroutines
	errChan := make(chan error)

	// map of tokenID => uriWithMetadata to prevent duplicate queries
	uris := &sync.Map{}

	finalBlockNum := "0"
	mu := &sync.Mutex{}

	for i := start; i < total+start; i++ {
		if allTransfers[i].ERC721TokenID == "" {
			if len(allTransfers) > total+i {
				start++
				continue
			} else {
				continue
			}
		}
		go func(transfer *Transfer) {
			token, blockNum, err := processContractTransfers(contractMetadata, uris, transfer, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			if blockNum > finalBlockNum {
				finalBlockNum = blockNum
			}
			mu.Unlock()
			tokenChan <- token
		}(allTransfers[i])
	}

	// map of tokenID => token
	tokens := receieveTransfers(tokenChan, errChan, total, logger)

	// add every token to the result to be upserted in db
	result := make([]*persist.Token, len(tokens))
	i := 0
	for _, v := range tokens {
		result[i] = v
		i++
	}

	go func() {
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, result, pRuntime); err != nil {
			logger.Error(err)
		}
		if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
			Address:         pAddress,
			LastSyncedBlock: finalBlockNum,
		}, pRuntime); err != nil {
			logger.Error(err)
		}
		if pQueueUpdate {
			queueUpdateForContract(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		}
	}()

	return result, nil
}

func getAllTransfersForWallet(pCtx context.Context, address string, fromBlock string, pPageNumber int, pRuntime *runtime.Runtime) ([]*Transfer, error) {
	from, err := GetTransfersFrom(address, fromBlock, pPageNumber, "", pRuntime)
	if err != nil {
		return nil, err
	}

	to, err := GetTransfersTo(address, fromBlock, pPageNumber, "", pRuntime)
	if err != nil {
		return nil, err
	}
	allTransfers := append(from, to...)
	sortByBlockNumber(allTransfers)
	return allTransfers, nil
}

func sortByBlockNumber(transfers []*Transfer) {
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

func setupPagination(pPageNumber, pPagesize, pLenTotal int) (int, int) {
	total := pPagesize
	start := pPageNumber - 1*pPagesize
	if pPageNumber == 0 {
		start = 0
		total = pLenTotal
	} else {
		if start < 0 {
			start = 0
		}
		if pLenTotal < start+total {
			total = pLenTotal
			start = total - pPagesize
			if start < 0 {
				start = 0
			}
		}
	}
	return total, start
}

func processWalletTransfer(contractMetadatas, tokenDetails *sync.Map, transfer *Transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {
	// required data for a token
	if transfer.ERC721TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}
	if transfer.RawContract.Address == "" {
		return nil, "", errors.New("no contract address found for token")
	}

	if _, ok := contractMetadatas.Load(transfer.RawContract.Address); !ok {
		metadata, err := GetTokenContractMetadata(transfer.RawContract.Address, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenContractMetadata", "contract": transfer.RawContract.Address}).Error(err)
			return nil, "", err
		}

		contractMetadatas.Store(transfer.RawContract.Address, metadata)
	}
	if _, ok := tokenDetails.Load(transfer.RawContract.Address + transfer.ERC721TokenID); !ok {
		uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": transfer.RawContract.Address, "tokenID": transfer.ERC721TokenID}).Error(err)
			return nil, "", err
		}
		metadata, err := GetTokenMetadata(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
			return nil, "", err
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
		return nil, "", err
	}
	blockNum, err := util.NormalizeHex(transfer.BlockNumber)
	if err != nil {
		return nil, "", err
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
		TokenMetadata:  uri.md,
	}
	return token, blockNum, nil
}

func receieveTransfers(tokenChan chan *persist.Token, errChan chan error, total int, logger *logrus.Entry) map[string]*persist.Token {

	allTokens := map[string]*persist.Token{}
out:
	for i := 0; i < total; i++ {
		select {
		case t := <-tokenChan:
			// add token to map of tokens if not there, otherwise update owner history, last block num,
			// and current owner
			if it, ok := allTokens[t.TokenContract.Address+t.TokenID]; ok {
				ownerHistory := append(t.PreviousOwners, it.PreviousOwners...)
				it.PreviousOwners = ownerHistory
				it.OwnerAddress = t.OwnerAddress
			} else {
				allTokens[t.TokenContract.Address+t.TokenID] = t
			}
		case err := <-errChan:
			logger.Error(err)
		case <-time.After(time.Second * 5):
			logger.Error("timed out waiting for token data")
			break out
		}
	}
	return allTokens
}

func processContractTransfers(contractMetadata TokenContractMetadata, uris *sync.Map, transfer *Transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {

	if transfer.ERC721TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}

	if _, ok := uris.Load(transfer.ERC721TokenID); !ok {
		uri, err := GetTokenURI(transfer.RawContract.Address, transfer.ERC721TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": transfer.RawContract.Address, "tokenID": transfer.ERC721TokenID}).Error(err)
			return nil, "", err
		}
		metadata, err := GetTokenMetadata(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
			return nil, "", err
		}
		uris.Store(transfer.ERC721TokenID, uriWithMetadata{uri, metadata})
	}

	tokenID, err := util.NormalizeHex(transfer.ERC721TokenID)
	if err != nil {
		return nil, "", err
	}

	blockNum, err := util.NormalizeHex(transfer.BlockNumber)
	if err != nil {
		return nil, "", err
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
		TokenMetadata:  uri.md,
	}
	return token, blockNum, nil
}

func queueUpdateForWallet(pCtx context.Context, pQueue *queue.Queue, pWalletAddress string, pLastBlock string, pRuntime *runtime.Runtime) {
	pQueue.AddJob(queue.Job{
		Name: "UpdateWallet",
		Action: func() error {
			_, err := getTokensFromBCForWallet(pCtx, pWalletAddress, 0, pLastBlock, false, pRuntime)
			return err
		},
	})
}
func queueUpdateForContract(pCtx context.Context, pQueue *queue.Queue, pContractAddress string, pLastBlock string, pRuntime *runtime.Runtime) {
	pQueue.AddJob(queue.Job{
		Name: "UpdateContract",
		Action: func() error {
			_, err := getTokensFromBCForContract(pCtx, pContractAddress, 0, pLastBlock, false, pRuntime)
			return err
		},
	})
}
