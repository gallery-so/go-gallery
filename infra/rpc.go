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

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
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
	Category    string   `json:"category"`
	BlockNumber string   `json:"blockNum"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Value       float64  `json:"value"`
	TokenID     string   `json:"erc721TokenId"`
	Type        string   `json:"type"`
	Amount      uint64   `json:"amount"`
	Asset       string   `json:"asset"`
	Hash        string   `json:"hash"`
	RawContract contract `json:"rawContract"`
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

// getTokenTransfersFrom returns the transfers from the given address
func getTokenTransfersFrom(pAddress, pFromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {
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

// getTokenTransfersTo returns the transfers to the given address
func getTokenTransfersTo(pAddress, pFromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {
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

// getContractTokenTransfers returns the transfers for a given contract
func getContractTokenTransfers(pAddress, pFromBlock string, pPageNumber, pMaxCount int, pRuntime *runtime.Runtime) ([]*transfer, error) {
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
			if _, ok := ids[tran.TokenID]; !ok {
				uniqueIDs++
				ids[tran.TokenID] = true
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
				if _, ok := ids[tran.TokenID]; !ok {
					uniqueIDs++
					ids[tran.TokenID] = true
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

// getERC721TokenURI returns metadata URI for a given token address
func getERC721TokenURI(address, tokenID string, pRuntime *runtime.Runtime) (string, error) {

	contract := common.HexToAddress(address)
	instance, err := contracts.NewIERC721Metadata(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return "", err
	}

	i, err := util.HexToBigInt(tokenID)
	if err != nil {
		return "", err
	}
	logrus.Debugf("Token ID: %d\tToken Address: %s", i.Uint64(), contract.Hex())

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

// getMetadataFromURI parses and returns the NFT metadata for a given token URI
func getMetadataFromURI(tokenURI string, pRuntime *runtime.Runtime) (map[string]interface{}, error) {

	client := &http.Client{
		Timeout: time.Second * 3,
	}

	if strings.Contains(tokenURI, "data:application/json;base64,") {
		// decode the base64 encoded json
		b64data := tokenURI[strings.IndexByte(tokenURI, ',')+1:]
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

// getERC721sForWallet returns the ERC721 token for the given wallet address
func getERC721sForWallet(pCtx context.Context, pAddress string, pPageNumber, pMaxCount int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForWallet"})
	allTransfers, err := aggregateTransfersForWallet(pCtx, pAddress, pFromBlock, pRuntime)
	if err != nil {
		return nil, err
	}
	total, start, end := setupPagination(pPageNumber, pMaxCount, len(allTransfers))

	// channel that will receive complete tokens from goroutines that will fill out token data
	tokenChan := make(chan *tokenWithBlockNumber)
	// channel that will receive errors from goroutines that will fill out token data
	errChan := make(chan error)

	// map of token contract address + token ID => uriWithMetadata to prevent duplicate calls to the
	// blockchain for retrieving token URI and Metadata
	tokenDetails := &sync.Map{}

	// spin up a goroutine for each transfer
	for i := start; i < end; i++ {

		if allTransfers[i].TokenID == "" {
			if len(allTransfers) > end+i {
				end++
				continue
			} else {
				total--
				continue
			}
		}

		// if this user is removing a token we want to increase total so that
		// we get the correct number of items per page
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
	allTokens, _ := receieveERC721Transfers(tokenChan, errChan, total, logger)

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
		// user, _ := persist.UserGetByAddress(pCtx, pAddress, pRuntime)
		// if user != nil {
		// 	for _, v := range allTokens {
		// 		v.OwnerUserID = user.ID
		// 	}
		// }
		// // update DB
		// if err = persist.TokenBulkUpsert(pCtx, allTokens, pRuntime); err != nil {
		// 	logger.WithError(err).Error("failed to upsert tokens")
		// }
		// if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
		// 	Address:         pAddress,
		// 	LastSyncedBlock: finalBlockNum,
		// }, pRuntime); err != nil {
		// 	logger.WithError(err).Error("failed to update account")
		// }
		// if err = updateContractsForTransfers(pCtx, allTransfers, pRuntime); err != nil {
		// 	logger.WithError(err).Error("error updating contracts for transfers")
		// }
		// for _, v := range allTokens {
		// 	pRuntime.ImageProcessingQueue.AddJob(queue.Job{
		// 		Name: fmt.Sprintf("image processing %s-%s", v.ContractAddress, v.TokenID),
		// 		Action: func() error {
		// 			return makePreviewsForToken(pCtx, v.ContractAddress, v.TokenID, pRuntime)
		// 		},
		// 	})
		// }
		// if pQueueUpdate {
		// 	queueUpdateForWallet(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		// }
	}()

	return ownedTokens, nil
}

// getERC721sForContract returns the ERC721 token for the given contract address
func getERC721sForContract(pCtx context.Context, pAddress string, pPageNumber, pMaxCount int, pFromBlock string, pQueueUpdate bool, pRuntime *runtime.Runtime) ([]*persist.Token, error) {
	logger := logrus.WithFields(logrus.Fields{"method": "getTokensFromBCForContract"})
	allTransfers, err := getContractTokenTransfers(pAddress, pFromBlock, pPageNumber, pMaxCount, pRuntime)
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
		if allTransfers[i].TokenID == "" {
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
	tokens, _ := receieveERC721Transfers(tokenChan, errChan, total, logger)

	go func() {
		// update DB
		// if err = persist.TokenBulkUpsert(pCtx, tokens, pRuntime); err != nil {
		// 	logger.WithError(err).Error("failed to upsert tokens")
		// }
		// if err = persist.AccountUpsertByAddress(pCtx, pAddress, &persist.Account{
		// 	Address:         pAddress,
		// 	LastSyncedBlock: finalBlockNum,
		// }, pRuntime); err != nil {
		// 	logger.WithError(err).Error("failed to update account")
		// }
		// if err = updateContractsForTransfers(pCtx, allTransfers, pRuntime); err != nil {
		// 	logger.WithError(err).Error("error updating contracts for transfers")
		// }
		// for _, v := range tokens {
		// 	pRuntime.ImageProcessingQueue.AddJob(queue.Job{
		// 		Name: fmt.Sprintf("image processing %s-%s", v.ContractAddress, v.TokenID),
		// 		Action: func() error {
		// 			return makePreviewsForToken(pCtx, v.ContractAddress, v.TokenID, pRuntime)
		// 		},
		// 	})
		// }
		// if pQueueUpdate {
		// 	queueUpdateForContract(pCtx, pRuntime.BlockchainUpdateQueue, pAddress, finalBlockNum, pRuntime)
		// }

	}()

	return tokens, nil
}

func aggregateTransfersForWallet(pCtx context.Context, address string, fromBlock string, pRuntime *runtime.Runtime) ([]*transfer, error) {

	from, err := getTokenTransfersFrom(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	to, err := getTokenTransfersTo(address, fromBlock, pRuntime)
	if err != nil {
		return nil, err
	}

	allTransfers := append(to, from...)

	sort.Slice(allTransfers, func(i, j int) bool {
		b1, err := util.HexToBigInt(allTransfers[i].BlockNumber)
		if err != nil {
			return false
		}
		b2, err := util.HexToBigInt(allTransfers[j].BlockNumber)
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
	if tran.TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}
	if tran.RawContract.Address == "" {
		return nil, "", errors.New("no contract address found for token")
	}

	genericUwm, ok := tokenDetails.Load(tran.RawContract.Address + tran.TokenID)
	if !ok {
		uri, err := getERC721TokenURI(tran.RawContract.Address, tran.TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": tran.RawContract.Address, "tokenID": tran.TokenID}).Error(err)
		}

		metadata, err := getMetadataFromURI(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
		}
		genericUwm = uriWithMetadata{uri, metadata}

		tokenDetails.Store(tran.RawContract.Address+tran.TokenID, genericUwm)
	}

	uwm := genericUwm.(uriWithMetadata)

	// get token ID in non-prefixed hex format from big int
	tokenID, err := util.NormalizeHexString(tran.TokenID)
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
		Type:            persist.TokenTypeERC721,
	}
	return token, blockNum, nil
}

func receieveERC721Transfers(tokenChan chan *tokenWithBlockNumber, errChan chan error, total int, logger *logrus.Entry) ([]*persist.Token, string) {

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
		b1, err := util.HexToBigInt(allTokens[i].blockNumber)
		if err != nil {
			return false
		}
		b2, err := util.HexToBigInt(allTokens[j].blockNumber)
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

	if tran.TokenID == "" {
		return nil, "", errors.New("no token ID found for token")
	}

	if _, ok := uris.Load(tran.TokenID); !ok {
		uri, err := getERC721TokenURI(tran.RawContract.Address, tran.TokenID, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenURI", "contract": tran.RawContract.Address, "tokenID": tran.TokenID}).Error(err)
		}
		metadata, err := getMetadataFromURI(uri, pRuntime)
		if err != nil {
			logger.WithFields(logrus.Fields{"section": "GetTokenMetadata", "uri": uri}).Error(err)
		}
		uris.Store(tran.TokenID, uriWithMetadata{uri, metadata})
	}

	tokenID, err := util.NormalizeHexString(tran.TokenID)
	if err != nil {
		return nil, "", err
	}

	blockNum, err := util.NormalizeHexString(tran.BlockNumber)
	if err != nil {
		return nil, "", err
	}

	genericURI, _ := uris.Load(tran.TokenID)
	uri := genericURI.(uriWithMetadata)

	token := &persist.Token{
		ContractAddress: strings.ToLower(tran.RawContract.Address),
		TokenID:         tokenID,
		TokenURI:        uri.uri,
		OwnerAddress:    strings.ToLower(tran.To),
		PreviousOwners:  []string{strings.ToLower(tran.From)},
		Type:            persist.TokenTypeERC721,
		TokenMetadata:   uri.md,
	}
	return token, blockNum, nil
}

func updateContractsForTransfers(pCtx context.Context, pTranfsers []*transfer, pRuntime *runtime.Runtime) error {
	transferToBlock := map[string]*big.Int{}

	for _, tran := range pTranfsers {
		newBlock, err := util.HexToBigInt(tran.BlockNumber)
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

// if logging all events is too large and takes too much time, start from the front and go backwards until one is found
// given that the most recent URI event should be the current URI
func getERC1155TokenURI(pContractAddress, pTokenID string, pRuntime *runtime.Runtime) (string, error) {
	topics := [][]common.Hash{{common.HexToHash("0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b")}, {common.HexToHash("0x" + padHex(pTokenID, 64))}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	def, err := util.HexToBigInt(defaultERC721Block)
	if err != nil {
		return "", err
	}

	logs, err := pRuntime.InfraClients.ETHClient.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: def,
		Addresses: []common.Address{common.HexToAddress(pContractAddress)},
		Topics:    topics,
	})
	if err != nil {
		return "", err
	}
	if len(logs) == 0 {
		return "", errors.New("No logs found")
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].BlockNumber > logs[j].BlockNumber
	})
	if len(logs[0].Data) < 128 {
		return "", errors.New("invalid data")
	}

	offset := new(big.Int).SetBytes(logs[0].Data[:32])
	length := new(big.Int).SetBytes(logs[0].Data[32:64])
	uri := string(logs[0].Data[offset.Uint64()+32 : offset.Uint64()+32+length.Uint64()])
	return uri, nil
}

func getBalanceOfERC1155Token(pOwnerAddress, pContractAddress, pTokenID string, pRuntime *runtime.Runtime) (*big.Int, error) {
	contract := common.HexToAddress(pContractAddress)
	owner := common.HexToAddress(pOwnerAddress)
	instance, err := contracts.NewIERC1155(contract, pRuntime.InfraClients.ETHClient)
	if err != nil {
		return nil, err
	}

	i, err := util.HexToBigInt(pTokenID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	tokenURI, err := instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, owner, i)
	if err != nil {
		return nil, err
	}

	return tokenURI, nil
}

func padHex(pHex string, pLength int) string {
	for len(pHex) < pLength {
		pHex = "0" + pHex
	}
	return pHex
}
