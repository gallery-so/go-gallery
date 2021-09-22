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
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/queue"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

// transfers represents the transfers for a given rpc response
type transfers struct {
	PageKey   string      `json:"pageKey"`
	Transfers []*transfer `json:"transfers"`
}

// transfer represents a transfer from the RPC response
type transfer struct {
	Category      string   `json:"category"`
	BlockNumber   string   `json:"blockNum"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Value         float64  `json:"value"`
	ERC721TokenID string   `json:"erc721TokenId"`
	Asset         string   `json:"asset"`
	Hash          string   `json:"hash"`
	RawContract   contract `json:"rawContract"`
}

// contract represents a contract that is interacted with during a transfer
type contract struct {
	Address string `json:"address"`
	Value   string `json:"value"`
	Decimal string `json:"decimal"`
}

// tokenContractMetadata represents a token contract's metadata
type tokenContractMetadata struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Logo   string `json:"logo"`
}

type uriWithMetadata struct {
	uri string
	md  map[string]interface{}
}

type tokenWithBlockNumber struct {
	token       *persist.Token
	blockNumber string
}

// TODO combine this function with get transfers to so that we can ensure a certain amount of results are returned and paginate

// getTransfersFrom returns the transfers from the given address
func getTransfersFrom(pAddress, pFromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {
	result := &transfers{}

	block, err := util.NormalizeHexString(pFromBlock)
	if err != nil {
		return nil, err
	}
	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + block
	opts["fromAddress"] = pAddress
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false

	err = pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Total Transfers From: %d", len(result.Transfers))
	if len(result.Transfers) == 1000 {
		it := &transfers{}
		opts["pageKey"] = result.PageKey
		for len(it.Transfers) == 1000 {
			err = pRuntime.InfraClients.RPCClient.Call(it, "alchemy_getAssetTransfers", opts)
			if err != nil {
				return nil, err
			}
			result.Transfers = append(result.Transfers, it.Transfers...)
			opts["pageKey"] = it.PageKey
		}
	}

	return result.Transfers, nil
}

// getTransfersTo returns the transfers to the given address
func getTransfersTo(pAddress, pFromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {
	result := &transfers{}

	block, err := util.NormalizeHexString(pFromBlock)
	if err != nil {
		return nil, err
	}
	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + block
	opts["toAddress"] = pAddress
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false

	err = pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Total Transfers To: %d", len(result.Transfers))
	if len(result.Transfers) == 1000 {
		it := &transfers{}
		opts["pageKey"] = result.PageKey
		for len(it.Transfers) == 1000 {
			err = pRuntime.InfraClients.RPCClient.Call(it, "alchemy_getAssetTransfers", opts)
			if err != nil {
				return nil, err
			}
			result.Transfers = append(result.Transfers, it.Transfers...)
			opts["pageKey"] = it.PageKey
		}
	}

	return result.Transfers, nil
}

// getContractTransfers returns the transfers for a given contract
func getContractTransfers(pAddress, pFromBlock string, pPageNumber, pMaxCount int, pRuntime *runtime.Runtime) ([]*transfer, error) {
	result := &transfers{}

	if pMaxCount < 1 {
		pMaxCount = 50
	}

	block, err := util.NormalizeHexString(pFromBlock)
	if err != nil {
		return nil, err
	}
	opts := map[string]interface{}{}
	opts["fromBlock"] = "0x" + block
	opts["contractAddresses"] = []string{pAddress}
	opts["category"] = []string{"token"}
	opts["excludeZeroValue"] = false

	if pPageNumber > 0 {
		opts["maxCount"] = hexutil.EncodeUint64(uint64(pMaxCount))
	}

	err = pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getAssetTransfers", opts)
	if err != nil {
		return nil, err
	}

	if pPageNumber > 0 {
		uniqueIDs := 0
		ids := map[string]bool{}
		for _, tran := range result.Transfers {
			if _, ok := ids[tran.ERC721TokenID]; !ok {
				uniqueIDs++
				ids[tran.ERC721TokenID] = true
			}
		}

		opts["pageKey"] = result.PageKey
		for uniqueIDs < pPageNumber*pMaxCount {
			it := &transfers{}
			err = pRuntime.InfraClients.RPCClient.Call(it, "alchemy_getAssetTransfers", opts)
			if err != nil {
				return nil, err
			}
			if len(it.Transfers) == 0 {
				break
			}
			for _, tran := range it.Transfers {
				if _, ok := ids[tran.ERC721TokenID]; !ok {
					uniqueIDs++
					ids[tran.ERC721TokenID] = true
				}
			}
			result.Transfers = append(result.Transfers, it.Transfers...)
			opts["pageKey"] = it.PageKey
		}
	} else {
		if len(result.Transfers) == 1000 {
			it := &transfers{}
			opts["pageKey"] = result.PageKey
			for len(it.Transfers) == 1000 {
				err = pRuntime.InfraClients.RPCClient.Call(it, "alchemy_getAssetTransfers", opts)
				if err != nil {
					return nil, err
				}
				result.Transfers = append(result.Transfers, it.Transfers...)
				opts["pageKey"] = it.PageKey
			}
		}
	}
	logrus.Debugf("Total Transfers Contract: %d", len(result.Transfers))

	return result.Transfers, nil
}

// getTokenContractMetadata returns the metadata for a given contract (without URI)
func getTokenContractMetadata(address string, pRuntime *runtime.Runtime) (tokenContractMetadata, error) {
	result := &tokenContractMetadata{}

	err := pRuntime.InfraClients.RPCClient.Call(result, "alchemy_getTokenMetadata", address)
	if err != nil {
		return tokenContractMetadata{}, err
	}

	return *result, nil
}

// getTokenURI returns metadata URI for a given token address
func getTokenURI(address, tokenID string, pRuntime *runtime.Runtime) (string, error) {

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

// getTokenMetadata parses and returns the NFT metadata for a given token URI
func getTokenMetadata(tokenURI string, pRuntime *runtime.Runtime) (map[string]interface{}, error) {

	client := &http.Client{
		Timeout: time.Second + (time.Millisecond * 300),
	}

	if strings.Contains(tokenURI, "data:application/json;base64,") {
		// decode the base64 encoded json
		noSuffix := strings.TrimSuffix(tokenURI, "==#4")
		b64data := noSuffix[strings.IndexByte(noSuffix, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return nil, err
		}

		metadata := map[string]interface{}{}
		err = json.Unmarshal(decoded, &metadata)
		if err != nil {
			return nil, err
		}

		return metadata, nil
	} else if strings.HasPrefix(tokenURI, "ipfs://") {

		path := strings.TrimPrefix(tokenURI, "ipfs://")

		it, err := pRuntime.IPFS.Cat(path)
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
		return nil, nil
	}

}

// getTokensFromBCForWallet returns the ERC721 token for the given wallet address
func getTokensFromBCForWallet(pCtx context.Context, pAddress string, pPageNumber, pMaxCount int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForWallet"})
	allTransfers, err := getAllTransfersForWallet(pCtx, pAddress, pFromBlock, pRuntime)
	if err != nil {
		return nil, err
	}
	total, start, end := setupPagination(pPageNumber, pMaxCount, len(allTransfers))

	// channel that will receive complete tokens from goroutines that will fill out token data
	tokenChan := make(chan *tokenWithBlockNumber)
	// channel that will receive errors from goroutines that will fill out token data
	errChan := make(chan error)

	// map of token contract address + token ID => uriWithMetadata to prevent duplicate calls to the
	// blockchain for retrieving token URI
	tokenDetails := &sync.Map{}

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

		if strings.EqualFold(allTransfers[i].From, pAddress) {
			if len(allTransfers) > end+i {
				end++
				total++
			}
		}

		go func(tran *transfer) {
			token, blockNum, err := processWalletTransfer(tokenDetails, tran, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			tokenChan <- &tokenWithBlockNumber{token: token, blockNumber: blockNum}
		}(allTransfers[i])
	}

	// map of token contract address + token ID => token to keep track of ownership seeing as
	// tokens will appear more than once in the transfers
	allTokens, finalBlockNum := receieveTransfers(tokenChan, errChan, total, logger)

	// all the tokens owned by `address`
	ownedTokens := []*persist.Token{}

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
		// if pQueueUpdate {
		// 	queueUpdateForWallet(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		// }
	}()

	return ownedTokens, nil
}

// getTokensFromBCForContract returns the ERC721 token for the given contract address
func getTokensFromBCForContract(pCtx context.Context, pAddress string, pPageNumber, pMaxCount int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForContract"})
	allTransfers, err := getContractTransfers(pAddress, pFromBlock, pPageNumber, pMaxCount, pRuntime)
	if err != nil {
		return nil, err
	}
	total, start, end := setupPagination(pPageNumber, pMaxCount, len(allTransfers))

	// channel receiving fully filled tokens from goroutines
	tokenChan := make(chan *tokenWithBlockNumber)
	// channel receiving errors from goroutines
	errChan := make(chan error)

	// map of tokenID => uriWithMetadata to prevent duplicate queries
	uris := &sync.Map{}

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
		go func(tran *transfer) {
			token, blockNum, err := processContractTransfers(uris, tran, logger, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			tokenChan <- &tokenWithBlockNumber{token: token, blockNumber: blockNum}
		}(allTransfers[i])
	}

	// map of tokenID => token
	tokens, finalBlockNum := receieveTransfers(tokenChan, errChan, total, logger)

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

func getAllTransfersForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {

	from, err := getTransfersFrom(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	to, err := getTransfersTo(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	allTransfers := append(to, from...)

	sort.Slice(allTransfers, func(i, j int) bool {
		b1, err := util.NormalizeHexInt(allTransfers[i].BlockNumber)
		if err != nil {
			return false
		}
		b2, err := util.NormalizeHexInt(allTransfers[j].BlockNumber)
		if err != nil {
			return false
		}
		return b1.Cmp(b2) == -1
	})

	return allTransfers, nil
}

func setupPagination(pPageNumber, pPageSize, pLenTotal int) (int, int, int) {
	total := pPageSize
	start := pPageNumber - 1*pPageSize
	if pPageNumber == 0 {
		start = 0
		total = pLenTotal
	} else {
		if start < 0 {
			start = 0
		}
		if pLenTotal < start+total {
			total = pLenTotal
			start = total - pPageSize
			if start < 0 {
				start = 0
			}
		}
	}
	end := start + total
	return total, start, end
}

func processWalletTransfer(tokenDetails *sync.Map, tran *transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {
	// required data for a token
	if tran.ERC721TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}
	if tran.RawContract.Address == "" {
		return nil, "", errors.New("no contract address found for token")
	}

	genericUwm, ok := tokenDetails.Load(tran.RawContract.Address + tran.ERC721TokenID)
	if !ok {
		uri, err := getTokenURI(tran.RawContract.Address, tran.ERC721TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": tran.RawContract.Address, "tokenID": tran.ERC721TokenID}).Error(err)
		}

		metadata, err := getTokenMetadata(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
		}
		genericUwm = uriWithMetadata{uri, metadata}

		tokenDetails.Store(tran.RawContract.Address+tran.ERC721TokenID, genericUwm)
	}

	uwm := genericUwm.(uriWithMetadata)

	// get token ID in non-prefixed hex format from big int
	tokenID, err := util.NormalizeHexString(tran.ERC721TokenID)
	if err != nil {
		return nil, "", err
	}
	blockNum, err := util.NormalizeHexString(tran.BlockNumber)
	if err != nil {
		return nil, "", err
	}

	token := &persist.Token{
		ContractAddress: strings.ToLower(tran.RawContract.Address),
		TokenID:         tokenID,
		TokenURI:        uwm.uri,
		OwnerAddress:    strings.ToLower(tran.To),
		PreviousOwners:  []string{strings.ToLower(tran.From)},
		TokenMetadata:   uwm.md,
	}
	return token, blockNum, nil
}

func receieveTransfers(tokenChan chan *tokenWithBlockNumber, errChan chan error, total int, logger *logrus.Entry) ([]*persist.Token, string) {

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

	sort.Slice(allTokens, func(i, j int) bool {
		b1, err := util.NormalizeHexInt(allTokens[i].blockNumber)
		if err != nil {
			return false
		}
		b2, err := util.NormalizeHexInt(allTokens[j].blockNumber)
		if err != nil {
			return false
		}
		return b1.Cmp(b2) == -1
	})

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

	return allResult, allTokens[len(allTokens)-1].blockNumber
}

func processContractTransfers(uris *sync.Map, tran *transfer, logger *logrus.Entry, pRuntime *runtime.Runtime) (*persist.Token, string, error) {

	if tran.ERC721TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}

	if _, ok := uris.Load(tran.ERC721TokenID); !ok {
		uri, err := getTokenURI(tran.RawContract.Address, tran.ERC721TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": tran.RawContract.Address, "tokenID": tran.ERC721TokenID}).Error(err)
		}
		metadata, err := getTokenMetadata(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
		}
		uris.Store(tran.ERC721TokenID, uriWithMetadata{uri, metadata})
	}

	tokenID, err := util.NormalizeHexString(tran.ERC721TokenID)
	if err != nil {
		return nil, "", err
	}

	blockNum, err := util.NormalizeHexString(tran.BlockNumber)
	if err != nil {
		return nil, "", err
	}

	genericURI, _ := uris.Load(tran.ERC721TokenID)
	uri := genericURI.(uriWithMetadata)

	token := &persist.Token{
		ContractAddress: strings.ToLower(tran.RawContract.Address),
		TokenID:         tokenID,
		TokenURI:        uri.uri,
		OwnerAddress:    strings.ToLower(tran.To),
		PreviousOwners:  []string{strings.ToLower(tran.From)},
		TokenMetadata:   uri.md,
	}
	return token, blockNum, nil
}

func updateContractsForTransfers(pCtx context.Context, pTranfsers []*transfer, pRuntime *runtime.Runtime) error {
	transferToBlock := map[string]*big.Int{}

	for _, tran := range pTranfsers {
		newBlock, err := util.NormalizeHexInt(tran.BlockNumber)
		if err != nil {
			return err
		}
		if block, ok := transferToBlock[tran.RawContract.Address]; ok {
			cmp := newBlock.Cmp(block)
			if cmp == 1 {
				transferToBlock[tran.RawContract.Address] = block
			} else {
				transferToBlock[tran.RawContract.Address] = newBlock
			}
		} else {
			transferToBlock[tran.RawContract.Address] = newBlock
		}
	}
	for k, v := range transferToBlock {
		contractMetadata, err := getTokenContractMetadata(k, pRuntime)
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
			_, err := getTokensFromBCForWallet(pCtx, pWalletAddress, 0, 0, pLastBlock, false, pRuntime)
			return err
		},
	})
}
func queueUpdateForContract(pCtx context.Context, pQueue *queue.Queue, pContractAddress string, pLastBlock string, pRuntime *runtime.Runtime) {
	pQueue.AddJob(queue.Job{
		Name: "UpdateContract",
		Action: func() error {
			_, err := getTokensFromBCForContract(pCtx, pContractAddress, 0, 0, pLastBlock, false, pRuntime)
			return err
		},
	})
}

func metadataToContract(metadata tokenContractMetadata, address string, blockNum string) (*persist.Contract, error) {
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
