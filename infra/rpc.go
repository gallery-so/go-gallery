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

type tokenWithBlockNumber struct {
	token *persist.Token
	block string
}

// GetTransfersFrom returns the transfers from the given address
func GetTransfersFrom(pAddress, pFromBlock string, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
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
	if len(result.Transfers) == 1000 {
		it, err := GetTransfersFrom(pAddress, pFromBlock, result.PageKey, pRuntime)
		if err != nil {
			return nil, err
		}
		result.Transfers = append(result.Transfers, it...)
	}

	return result.Transfers, nil
}

// GetTransfersTo returns the transfers to the given address
func GetTransfersTo(pAddress, pFromBlock string, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
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
	if len(result.Transfers) == 1000 {
		it, err := GetTransfersTo(pAddress, pFromBlock, result.PageKey, pRuntime)
		if err != nil {
			return nil, err
		}
		result.Transfers = append(result.Transfers, it...)
	}

	return result.Transfers, nil
}

// GetContractTransfers returns the transfers for a given contract
func GetContractTransfers(pAddress, pFromBlock string, pPageKey string, pRuntime *runtime.Runtime) ([]*Transfer, error) {
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
	if len(result.Transfers) == 1000 {
		it, err := GetContractTransfers(pAddress, pFromBlock, result.PageKey, pRuntime)
		if err != nil {
			return nil, err
		}
		result.Transfers = append(result.Transfers, it...)
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

	if strings.HasPrefix(tokenURI, "data:application/json;base64,") {
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
	} else if strings.Contains(tokenURI, "ipfs") {

		first := strings.TrimPrefix(tokenURI, "https://ipfs.io")
		second := strings.TrimPrefix(first, "/")
		third := strings.TrimPrefix(second, "ipfs://")
		final := strings.TrimPrefix(third, "ipfs/")

		it, err := pRuntime.IPFS.Cat(final)
		if err != nil {
			return nil, err
		}
		defer it.Close()

		buf := &bytes.Buffer{}
		_, err = io.Copy(buf, it)
		if err != nil {
			return nil, err
		}
		metadata := map[string]interface{}{}
		err = json.Unmarshal(buf.Bytes(), &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	} else if strings.HasPrefix(tokenURI, "https://") || strings.HasPrefix(tokenURI, "http://") {
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
	} else {
		return nil, errors.New("invalid token URI")
	}

}

// getTokensFromBCForWallet returns the ERC721 token for the given wallet address
func getTokensFromBCForWallet(pCtx context.Context, pAddress string, pPageNumber int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForWallet"})
	allTransfers, err := getAllTransfersForWallet(pCtx, pAddress, pFromBlock, pRuntime)
	if err != nil {
		return nil, err
	}
	total, start, end := setupPagination(pPageNumber, pageLength, len(allTransfers))

	// all the tokens owned by `address`
	ownedTokens := []*persist.Token{}

	// channel that will receive complete tokens from goroutines that will fill out token data
	tokenChan := make(chan *tokenWithBlockNumber)
	// channel that will receive errors from goroutines that will fill out token data
	errChan := make(chan error)

	// map of token contract address + token ID => uriWithMetadata to prevent duplicate calls to the
	// blockchain for retrieving token URI
	tokenDetails := &sync.Map{}

	finalBlockNum := "0"
	mu := &sync.Mutex{}
	// spin up a goroutine for each transfer
	for i := start; i < end; i++ {

		if allTransfers[i].ERC721TokenID == "" {
			if len(allTransfers) > end+i {
				end++
				continue
			} else {
				total--
				continue
			}
		}

		go func(transfer *Transfer) {
			token, blockNum, err := processWalletTransfer(tokenDetails, transfer, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			if blockNum > finalBlockNum {
				finalBlockNum = blockNum
			}
			mu.Unlock()
			tokenChan <- &tokenWithBlockNumber{token: token, block: blockNum}
		}(allTransfers[i])
	}

	// map of token contract address + token ID => token to keep track of ownership seeing as
	// tokens will appear more than once in the transfers
	allTokens := receieveTransfers(tokenChan, errChan, total, logger)

	for _, v := range allTokens {
		// add every token to the result to be upserted in db
		// only add token to owned tokens if it is owned by the wallet
		if strings.EqualFold(v.OwnerAddress, pAddress) {
			ownedTokens = append(ownedTokens, v)
		}
	}

	go func() {
		user, _ := persist.UserGetByAddress(pCtx, pAddress, pRuntime)
		if user != nil {
			for _, v := range allTokens {
				v.OwnerUserID = user.ID
			}
		}
		logger.Debug("Updating tokens in DB ", allTokens)
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, allTokens, pRuntime); err != nil {
			logger.WithError(err).Error("failed to upsert tokens")
		}
		if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
			Address:         pAddress,
			LastSyncedBlock: finalBlockNum,
		}, pRuntime); err != nil {
			logger.WithError(err).Error("failed to update account")
		}
		if err = updateContractsForTransfers(pCtx, allTransfers, pRuntime); err != nil {
			logger.WithError(err).Error("error updating contracts for transfers")
		}
		if pQueueUpdate {
			queueUpdateForWallet(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		}
	}()

	return ownedTokens, nil
}

// getTokensFromBCForContract returns the ERC721 token for the given contract address
func getTokensFromBCForContract(pCtx context.Context, pAddress string, pPageNumber int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForContract"})
	allTransfers, err := GetContractTransfers(pAddress, pFromBlock, "", pRuntime)
	if err != nil {
		return nil, err
	}
	total, start, end := setupPagination(pPageNumber, pageLength, len(allTransfers))

	// channel receiving fully filled tokens from goroutines
	tokenChan := make(chan *tokenWithBlockNumber)
	// channel receiving errors from goroutines
	errChan := make(chan error)

	// map of tokenID => uriWithMetadata to prevent duplicate queries
	uris := &sync.Map{}

	finalBlockNum := "0"
	mu := &sync.Mutex{}

	for i := start; i < end; i++ {
		if allTransfers[i].ERC721TokenID == "" {
			if len(allTransfers) > end+i {
				end++
				continue
			} else {
				total--
				continue
			}
		}
		go func(transfer *Transfer) {
			token, blockNum, err := processContractTransfers(uris, transfer, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			mu.Lock()
			if blockNum > finalBlockNum {
				finalBlockNum = blockNum
			}
			mu.Unlock()
			tokenChan <- &tokenWithBlockNumber{token: token, block: blockNum}
		}(allTransfers[i])
	}

	// map of tokenID => token
	tokens := receieveTransfers(tokenChan, errChan, total, logger)

	go func() {
		// update DB
		if err = persist.TokenBulkUpsert(pCtx, tokens, pRuntime); err != nil {
			logger.WithError(err).Error("failed to upsert tokens")
		}
		if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
			Address:         pAddress,
			LastSyncedBlock: finalBlockNum,
		}, pRuntime); err != nil {
			logger.WithError(err).Error("failed to update account")
		}
		if err = updateContractsForTransfers(pCtx, allTransfers, pRuntime); err != nil {
			logger.WithError(err).Error("error updating contracts for transfers")
		}
		if pQueueUpdate {
			queueUpdateForContract(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		}

	}()

	return tokens, nil
}

func getAllTransfersForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*Transfer, error) {

	from, err := GetTransfersFrom(address, fromBlock, "", pRuntime)
	if err != nil {
		return nil, err
	}

	to, err := GetTransfersTo(address, fromBlock, "", pRuntime)
	if err != nil {
		return nil, err
	}
	allTransfers := append(from, to...)
	return allTransfers, nil
}

func sortByBlockNumber(transfers []*tokenWithBlockNumber) {
	sort.Slice(transfers, func(i, j int) bool {
		b1, err := util.NormalizeHexInt(transfers[i].block)
		if err != nil {
			return false
		}
		b2, err := util.NormalizeHexInt(transfers[j].block)
		if err != nil {
			return false
		}
		return b1.Cmp(b2) == -1
	})
}

func setupPagination(pPageNumber, pPagesize, pLenTotal int) (int, int, int) {
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
	end := start + total
	return total, start, end
}

func processWalletTransfer(tokenDetails *sync.Map, transfer *Transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {
	// required data for a token
	if transfer.ERC721TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}
	if transfer.RawContract.Address == "" {
		return nil, "", errors.New("no contract address found for token")
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
		}

		tokenDetails.Store(transfer.RawContract.Address+transfer.ERC721TokenID, uriWithMetadata{uri, metadata})
	}

	genericURI, _ := tokenDetails.Load(transfer.RawContract.Address + transfer.ERC721TokenID)
	uri := genericURI.(uriWithMetadata)

	// get token ID in non-prefixed hex format from big int
	tokenID, err := util.NormalizeHexString(transfer.ERC721TokenID)
	if err != nil {
		return nil, "", err
	}
	blockNum, err := util.NormalizeHexString(transfer.BlockNumber)
	if err != nil {
		return nil, "", err
	}

	token := &persist.Token{
		ContractAddress: strings.ToLower(transfer.RawContract.Address),
		TokenID:         tokenID,
		TokenURI:        uri.uri,
		OwnerAddress:    strings.ToLower(transfer.To),
		PreviousOwners:  []string{strings.ToLower(transfer.From)},
		TokenMetadata:   uri.md,
	}
	return token, blockNum, nil
}

func receieveTransfers(tokenChan chan *tokenWithBlockNumber, errChan chan error, total int, logger *logrus.Entry) []*persist.Token {

	allTokens := []*tokenWithBlockNumber{}
out:
	for i := 0; i < total; i++ {
		select {
		case t := <-tokenChan:
			allTokens = append(allTokens, t)
		case err := <-errChan:
			logger.WithError(err).Error("failed to receive token")
		case <-time.After(time.Second * 5):
			logger.Error("timed out waiting for token data")
			break out
		}
	}

	sortByBlockNumber(allTokens)
	tokenMap := map[string]*persist.Token{}

	for _, t := range allTokens {
		if token, ok := tokenMap[t.token.ContractAddress+t.token.TokenID]; !ok {
			tokenMap[t.token.ContractAddress+t.token.TokenID] = t.token
		} else {
			token.PreviousOwners = append(token.PreviousOwners, t.token.PreviousOwners...)
			token.OwnerAddress = t.token.OwnerAddress
		}
	}
	allResult := make([]*persist.Token, len(tokenMap))
	i := 0
	for _, v := range tokenMap {
		allResult[i] = v
		i++
	}

	return allResult
}

func processContractTransfers(uris *sync.Map, transfer *Transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {

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

	tokenID, err := util.NormalizeHexString(transfer.ERC721TokenID)
	if err != nil {
		return nil, "", err
	}

	blockNum, err := util.NormalizeHexString(transfer.BlockNumber)
	if err != nil {
		return nil, "", err
	}

	genericURI, _ := uris.Load(transfer.ERC721TokenID)
	uri := genericURI.(uriWithMetadata)

	token := &persist.Token{
		ContractAddress: strings.ToLower(transfer.RawContract.Address),
		TokenID:         tokenID,
		TokenURI:        uri.uri,
		OwnerAddress:    strings.ToLower(transfer.To),
		PreviousOwners:  []string{strings.ToLower(transfer.From)},
		TokenMetadata:   uri.md,
	}
	return token, blockNum, nil
}

func updateContractsForTransfers(pCtx context.Context, pTranfsers []*Transfer, pRuntime *runtime.Runtime) error {
	transferToBlock := map[string]*big.Int{}

	for _, transfer := range pTranfsers {
		newBlock, err := util.NormalizeHexInt(transfer.BlockNumber)
		if err != nil {
			return err
		}
		if block, ok := transferToBlock[transfer.RawContract.Address]; ok {
			cmp := newBlock.Cmp(block)
			if cmp == 1 {
				transferToBlock[transfer.RawContract.Address] = block
			} else {
				transferToBlock[transfer.RawContract.Address] = newBlock
			}
		} else {
			transferToBlock[transfer.RawContract.Address] = newBlock
		}
	}
	for k, v := range transferToBlock {
		contractMetadata, err := GetTokenContractMetadata(k, pRuntime)
		if err != nil {
			return err
		}
		asContract, err := metadataToContract(contractMetadata, k, v.Text(16))
		if err != nil {
			return err
		}
		err = persist.ContractUpsertByAddress(pCtx, k, asContract, pRuntime)
		if err != nil {
			return err
		}
	}
	return nil
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

func metadataToContract(metadata TokenContractMetadata, address string, blockNum string) (*persist.Contract, error) {
	bn, err := util.NormalizeHexString(blockNum)
	if err != nil {
		return nil, err
	}
	return &persist.Contract{
		Address:         strings.ToLower(address),
		TokenName:       metadata.Name,
		Symbol:          metadata.Symbol,
		LastSyncedBlock: bn,
	}, nil
}
