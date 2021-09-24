package infra

import (
	"context"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

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

	lastBlockNumber uint64
	statsFile       string

	logs      chan []types.Log
	transfers chan []*transfer
	tokens    chan *persist.Token
	contracts chan string
	done      chan bool
	cancel    chan os.Signal

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

		logs:      make(chan []types.Log),
		transfers: make(chan []*transfer),
		tokens:    make(chan *persist.Token),
		contracts: make(chan string),
		done:      make(chan bool),
		cancel:    pRuntime.Cancel,

		tokenReceive:    tokenReceiveFunc,
		contractReceive: contractReceiveFunc,
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	i.state = 1
	i.wg.Add(5)
	go i.processLogs()
	go i.processTransfers()
	go i.processTokens()
	go i.processContracts()
	go i.handleCancel()
	i.wg.Wait()
}

func (i *Indexer) processLogs() {
	defer i.wg.Done()
	finalBlockUint, err := i.runtime.InfraClients.ETHClient.BlockNumber(context.Background())
	if err != nil {
		logrus.Errorf("failed to get block number: %v", err)
		atomic.StoreInt64(&i.state, -1)
		i.done <- true
	}

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	finalBlock := new(big.Int).SetUint64(finalBlockUint)

	go func() {
		defer close(i.logs)
		curBlock := new(big.Int).SetUint64(i.lastBlockNumber)
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
			i.lastBlockNumber = curBlock.Uint64()
			i.mu.Unlock()
			curBlock.Add(curBlock, big.NewInt(1800))
			nextBlock.Add(nextBlock, big.NewInt(1800))

			logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())
		}
	}()

	for {
		select {
		case logs, ok := <-i.logs:
			for _, log := range logs {
				i.transfers <- logToTransfer(log)
			}
			if !ok {
				logrus.Info("Logs channel closed, closing transfers channel")
				close(i.transfers)
				return
			}
		case <-i.done:
			return
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
		panic("unknown event hash")
	}
}

func (i *Indexer) processTransfers() {
	defer i.wg.Done()
	for {
		select {
		case transfers, ok := <-i.transfers:
			logrus.Infof("Got %d transfers", len(transfers))
			for _, transfer := range transfers {
				go processTransfer(i, transfer)
			}
			if !ok {
				logrus.Info("Transfer channel closed")
				transfersToTokens(i)
				logrus.Info("Done processing transfers, closing tokens channel")
				close(i.tokens)
				return
			}
		case <-i.done:
			return
		}
	}
}

func (i *Indexer) processTokens() {
	defer i.wg.Done()
	for {
		select {
		case token, ok := <-i.tokens:
			logrus.Infof("Processing token %s-%s", token.ContractAddress, token.TokenID)
			go func(notDone bool) {
				if !notDone {
					defer func() {
						atomic.StoreInt64(&i.state, 0)
						i.done <- true
					}()
				}
				err := i.tokenReceive(context.Background(), token, i.runtime)
				if err != nil {
					logrus.WithError(err).Error("Error processing token")
					// TODO handle this
				}
			}(ok)
		case <-i.done:
			return
		}
	}
}

func (i *Indexer) processContracts() {
	i.wg.Done()
	for {
		select {
		case contract := <-i.contracts:
			go func() {
				logrus.Infof("Processing contract %s", contract)
				// TODO turn contract into persist.Contract
				err := i.contractReceive(context.Background(), &persist.Contract{}, i.runtime)
				if err != nil {
					logrus.WithError(err).Error("Error processing token")
					// TODO handle this
				}
			}()
		case <-i.done:
			return
		}
	}
}

func (i *Indexer) handleCancel() {
	defer i.wg.Done()
	for {
		select {
		case <-i.cancel:
			logrus.Warn("CANCELLING")
			i.done <- true
			return
		case <-i.done:
			logrus.Info("STATE ", i.state)
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
				i.previousOwners[key] = append(i.previousOwners[key], it)
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
	i.mu.Unlock()
}

func transfersToTokens(i *Indexer) {
	i.mu.RLock()
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
	i.mu.RUnlock()
}
