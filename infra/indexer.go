package infra

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

// EventHash represents an event keccak256 hash
type EventHash string

const (
	// TransferEventHash represents the keccak256 hash of Transfer(address,address,uint256)
	TransferEventHash EventHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// TransferSingleEventHash represents the keccak256 hash of TransferSingle(address,address,address,uint256,uint256)
	TransferSingleEventHash EventHash = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	// TransferBatchEventHash represents the keccak256 hash of TransferBatch(address,address,address,uint256[],uint256[])
	TransferBatchEventHash EventHash = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
	// URIEventHash represents the keccak256 hash of URI(string,uint256)
	URIEventHash EventHash = "0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b"
)

// TODO do we need this if we can ensure that each log is proccessed in order
// TODO if we remove this we can allow token processing to occur without waiting for transfers to be sorted
type ownerAtBlock struct {
	owner string
	block uint64
}

// TokenReceiveFunc is a function that is called when a token is received
type TokenReceiveFunc func(pCtx context.Context, token *persist.Token, runtime *runtime.Runtime) error

// ContractReceiveFunc is a function that is called when a token contract is received
type ContractReceiveFunc func(pCtx context.Context, contract *persist.Contract, runtime *runtime.Runtime) error

// Indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type Indexer struct {
	state int64

	runtime *runtime.Runtime

	mu *sync.RWMutex
	wg *sync.WaitGroup

	metadatas      map[string]map[string]interface{}
	uris           map[string]string
	types          map[string]string
	contractStored map[string]bool
	owners         map[string]ownerAtBlock
	balances       map[string]map[string]*big.Int
	previousOwners map[string][]ownerAtBlock

	eventHashes []EventHash

	lastSyncedBlock uint64
	statsFile       string

	logs          chan []types.Log
	subscriptions chan types.Log
	transfers     chan []*transfer
	tokens        chan *persist.Token
	contracts     chan string
	done          chan bool
	cancel        chan os.Signal

	tokenReceive    TokenReceiveFunc
	contractReceive ContractReceiveFunc
}

// TODO figure out how to ensure that the event types are represented when going transfer -> token

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens with the given
// tokenReceiveFunc and store stats on the indexer in the given statsFile
func NewIndexer(pEvents []EventHash, tokenReceiveFunc TokenReceiveFunc, contractReceiveFunc ContractReceiveFunc, statsFileName string, pRuntime *runtime.Runtime) *Indexer {
	return &Indexer{

		runtime:        pRuntime,
		mu:             &sync.RWMutex{},
		wg:             &sync.WaitGroup{},
		metadatas:      make(map[string]map[string]interface{}),
		uris:           make(map[string]string),
		types:          make(map[string]string),
		balances:       make(map[string]map[string]*big.Int),
		contractStored: make(map[string]bool),
		owners:         make(map[string]ownerAtBlock),
		previousOwners: make(map[string][]ownerAtBlock),

		eventHashes: pEvents,
		statsFile:   statsFileName,

		logs:          make(chan []types.Log),
		subscriptions: make(chan types.Log),
		transfers:     make(chan []*transfer),
		tokens:        make(chan *persist.Token),
		contracts:     make(chan string),
		done:          make(chan bool),
		cancel:        pRuntime.Cancel,

		tokenReceive:    tokenReceiveFunc,
		contractReceive: contractReceiveFunc,
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	i.state = 1
	i.wg.Add(6)
	go i.processLogs()
	go i.processTransfers()
	go i.processTokens()
	go i.processContracts()
	go i.subscribeNewLogs()
	go i.handleDone()
	i.wg.Wait()
}

func (i *Indexer) processLogs() {
	defer i.wg.Done()
	defer close(i.transfers)
	finalBlockUint, err := i.runtime.InfraClients.ETHClient.BlockNumber(context.Background())
	if err != nil {
		logrus.Errorf("failed to get block number: %v", err)
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}

	logrus.Infof("final block number: %v", finalBlockUint)

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	finalBlock := new(big.Int).SetUint64(finalBlockUint)

	go func() {
		defer close(i.logs)
		curBlock := new(big.Int).SetUint64(i.lastSyncedBlock)
		nextBlock := new(big.Int).Add(curBlock, big.NewInt(1800))
		for nextBlock.Cmp(finalBlock) == -1 {
			logsTo, err := i.runtime.InfraClients.ETHClient.FilterLogs(context.Background(), ethereum.FilterQuery{
				FromBlock: curBlock,
				ToBlock:   nextBlock,
				Topics:    topics,
			})
			if err != nil {
				logrus.WithError(err).Error("Error getting logs, trying again")
				continue
			}
			logrus.Infof("Found %s logs at block %d", len(logsTo), curBlock.Uint64())
			i.logs <- logsTo
			i.mu.Lock()
			i.lastSyncedBlock = curBlock.Uint64()
			i.mu.Unlock()
			curBlock.Add(curBlock, big.NewInt(1800))
			nextBlock.Add(nextBlock, big.NewInt(1800))

			logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())
		}
	}()

	for logs := range i.logs {
		for _, log := range logs {
			i.transfers <- logToTransfer(log)
		}
	}
	logrus.Info("Logs channel closed, closing transfers channel")
}

func (i *Indexer) processTransfers() {
	defer i.wg.Done()
	defer close(i.tokens)
	count := 0
	for transfers := range i.transfers {
		if count%1000 == 0 {
			logrus.Infof("Processed %d sets of transfers", count)
			go storedDataToTokens(i)
		}
		logrus.Infof("Got %d transfers", len(transfers))
		for _, transfer := range transfers {
			go processTransfer(i, transfer)
		}
		count++
	}
	logrus.Info("Transfer channel closed")
	storedDataToTokens(i)
	logrus.Info("Done processing transfers, closing tokens channel")
}

func (i *Indexer) processTokens() {
	defer i.wg.Done()
	for token := range i.tokens {
		go func(t *persist.Token) {
			logrus.Infof("Processing token %s-%s", t.ContractAddress, t.TokenID)
			err := i.tokenReceive(context.Background(), t, i.runtime)
			if err != nil {
				logrus.WithError(err).Error("Error processing token")
				// TODO handle this
			}
		}(token)
	}
}

func (i *Indexer) processContracts() {
	i.wg.Done()
	for contract := range i.contracts {
		go func(c string) {
			logrus.Infof("Processing contract %s", c)
			// TODO turn contract into persist.Contract
			err := i.contractReceive(context.Background(), &persist.Contract{}, i.runtime)
			if err != nil {
				logrus.WithError(err).Error("Error processing token")
				// TODO handle this
			}
		}(contract)
	}
}

func (i *Indexer) subscribeNewLogs() {
	defer i.wg.Done()
	finalBlockUint, err := i.runtime.InfraClients.ETHClient.BlockNumber(context.Background())
	if err != nil {
		logrus.Errorf("failed to get block number: %v", err)
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}

	logrus.Infof("final block number: %v", finalBlockUint)

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	finalBlock := new(big.Int).SetUint64(finalBlockUint)
	sub, err := i.runtime.InfraClients.ETHClient.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: finalBlock,
		Topics:    topics,
	}, i.subscriptions)
	if err != nil {
		logrus.Errorf("failed to subscribe to logs: %v", err)
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}
	for {
		select {
		case log := <-i.subscriptions:
			logrus.Infof("Got log at: %d", log.BlockNumber)
			// for now just log because we have no way of knowing if transfers is closed
			// i.transfers <- logToTransfer(log)
		case err := <-sub.Err():
			logrus.Errorf("subscription error: %v", err)
			atomic.StoreInt64(&i.state, -1)
			i.done <- true
		case <-i.done:
			return
		}
	}
}

func (i *Indexer) handleDone() {
	defer i.wg.Done()
	for {
		select {
		case <-i.cancel:
			logrus.Warn("CANCELLING")
			return
		case <-i.done:
			logrus.Info("STATE ", i.state)
			i.writeStats()
			// TODO start subscribing to events starting from last block
			return
		}
	}
}

func processTransfer(i *Indexer, transfer *transfer) {
	bn, err := util.HexToBigInt(transfer.BlockNumber)
	if err != nil {
		logrus.WithError(err).Error("Error converting block number")
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}
	key := transfer.RawContract.Address + "--" + transfer.TokenID
	logrus.Infof("Processing transfer %s", key)

	i.mu.Lock()
	defer i.mu.Unlock()
	tokenType, ok := i.types[key]
	if !ok {
		i.types[key] = transfer.Type
		tokenType = transfer.Type
	}

	if !i.contractStored[key] {
		i.contractStored[key] = true
		i.contracts <- transfer.RawContract.Address
	}
	switch tokenType {
	case persist.TokenTypeERC721:
		if it, ok := i.owners[key]; ok {
			if it.owner != transfer.To {
				if it.block < bn.Uint64() {
					it.block = bn.Uint64()
					it.owner = transfer.To
				}
			}
		} else {
			i.owners[key] = ownerAtBlock{transfer.From, bn.Uint64()}
		}
		if it, ok := i.previousOwners[key]; !ok {
			i.previousOwners[key] = []ownerAtBlock{{
				owner: transfer.From,
				block: bn.Uint64(),
			}}
		} else {
			it = append(it, ownerAtBlock{
				owner: transfer.From,
				block: bn.Uint64(),
			})
		}
	case persist.TokenTypeERC1155:
		if it, ok := i.balances[key][transfer.From]; ok {
			it.Sub(it, new(big.Int).SetUint64(transfer.Amount))
		} else {
			i.balances[key][transfer.From] = new(big.Int).SetUint64(transfer.Amount)
		}
		if it, ok := i.balances[key][transfer.To]; ok {
			it.Add(it, new(big.Int).SetUint64(transfer.Amount))
		} else {
			i.balances[key][transfer.To] = new(big.Int).SetUint64(transfer.Amount)
		}
	default:
		logrus.Error("Unknown token type")
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}
	if _, ok := i.uris[key]; !ok {
		uri, err := getURIForERC1155Token(transfer.RawContract.Address, transfer.TokenID, i.runtime)
		if err != nil {
			logrus.WithError(err).Error("Error getting URI for ERC1155 token")
			// TODO handle this
		}
		i.uris[key] = uri
	}
	if _, ok := i.metadatas[key]; !ok {
		uri := i.uris[key]
		if uri == "" {
			logrus.Error("No URI found for ERC1155 token")
			atomic.StoreInt64(&i.state, -1)
			i.done <- true
		}
		id, err := util.HexToBigInt(transfer.TokenID)
		if err != nil {
			logrus.WithError(err).Error("Error converting token ID to big int")
			atomic.StoreInt64(&i.state, -1)
			i.done <- true
		}
		uriReplaced := strings.ReplaceAll(uri, "{id}", id.String())
		metadata, err := getMetadataFromURI(uriReplaced, i.runtime)
		if err != nil {
			logrus.WithError(err).Error("Error getting metadata for token")
			// TODO handle this
		}
		i.metadatas[key] = metadata
	}
}

func storedDataToTokens(i *Indexer) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for k, v := range i.owners {
		spl := strings.Split(k, "--")
		if len(spl) != 2 {
			logrus.Error("Invalid key")
			atomic.StoreInt64(&i.state, -1)
			i.done <- true
		}
		sort.Slice(i.previousOwners[k], func(l, m int) bool {
			return i.previousOwners[k][l].block < i.previousOwners[k][m].block
		})
		previousOwnerAddresses := make([]string, len(i.previousOwners[k]))
		for i, v := range i.previousOwners[k] {
			previousOwnerAddresses[i] = v.owner
		}
		token := &persist.Token{
			TokenID:         spl[1],
			ContractAddress: spl[0],
			OwnerAddress:    v.owner,
			PreviousOwners:  previousOwnerAddresses,
			Type:            persist.TokenTypeERC721,
			TokenMetadata:   i.metadatas[k],
			TokenURI:        i.uris[k],
		}
		i.tokens <- token
	}
	for k, v := range i.balances {
		spl := strings.Split(k, "--")
		if len(spl) != 2 {
			logrus.Error("Invalid key")
			atomic.StoreInt64(&i.state, -1)
			i.done <- true
		}
		for addr, balance := range v {
			token := &persist.Token{
				TokenID:         spl[1],
				ContractAddress: spl[0],
				OwnerAddress:    addr,
				Amount:          balance.Uint64(),
				Type:            persist.TokenTypeERC1155,
				TokenMetadata:   i.metadatas[k],
				TokenURI:        i.uris[k],
			}
			i.tokens <- token
		}
	}
}

func logToTransfer(pLog types.Log) []*transfer {
	switch pLog.Topics[0].Hex() {
	case string(TransferEventHash):
		from := strings.TrimPrefix("0x", pLog.Topics[1].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		id := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		return []*transfer{
			{
				From:        from,
				To:          to,
				TokenID:     id,
				Amount:      1,
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber).Text(16),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC721,
			},
		}
	case string(TransferSingleEventHash):
		if len(pLog.Topics) < 4 {
			panic("invalid topic length for single transfer event")
		}
		from := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		id := new(big.Int).SetBytes(pLog.Data[:len(pLog.Data)/2])
		amount := new(big.Int).SetBytes(pLog.Data[len(pLog.Data)/2:])
		return []*transfer{
			{
				From:        from,
				To:          to,
				TokenID:     id.Text(16),
				Amount:      amount.Uint64(),
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber).Text(16),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC1155,
			},
		}

	case string(TransferBatchEventHash):
		from := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		amountOffset := len(pLog.Data) / 2
		total := amountOffset / 64
		result := make([]*transfer, total)

		for i := 0; i < total; i++ {
			id := new(big.Int).SetBytes(pLog.Data[i*64 : (i+1)*64])
			amount := new(big.Int).SetBytes(pLog.Data[(amountOffset)+(i*64) : (amountOffset)+((i+1)*64)])
			result[i] = &transfer{
				From:    from,
				To:      to,
				TokenID: id.Text(16),
				Amount:  amount.Uint64(),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC1155,
			}
		}
		return result
	default:
		return nil
	}
}

func (i *Indexer) writeStats() {
	i.mu.RLock()
	defer i.mu.RUnlock()

	fi, err := os.Create(fmt.Sprintf("%s-%s.txt", i.statsFile, time.Now().Format("2006-01-02-15-04-05")))
	if err != nil {
		logrus.WithError(err).Error("Error creating stats file")
		return
	}
	defer fi.Close()
	fi.WriteString(fmt.Sprintf("State: %d\n", atomic.LoadInt64(&i.state)))
	fi.WriteString(fmt.Sprintf("Total ERC721 Tokens: %d\n", len(i.owners)))
	fi.WriteString(fmt.Sprintf("Total ERC1155 Tokens: %d\n", len(i.balances)))
	fi.WriteString(fmt.Sprintf("Total Tokens with URI: %d\n", len(i.uris)))
	fi.WriteString(fmt.Sprintf("Total Tokens with Metadata: %d\n", len(i.metadatas)))
	fi.WriteString(fmt.Sprintf("Last Block Number: %d", i.lastSyncedBlock))
}
