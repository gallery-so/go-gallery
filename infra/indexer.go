package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/persist"
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

type address string

type tokenID string

type tokenIdentifiers string

type metadata map[string]interface{}

type tokenBalances map[address]*big.Int

type uri string

type uniqueMetadataHandler func(*Indexer, address, tokenID) (metadata, error)

type uniqueMetadatas map[address]uniqueMetadataHandler

type ownerAtBlock struct {
	owner address
	block uint64
}

// Indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type Indexer struct {
	state int64

	ethClient  *ethclient.Client
	ipfsClient *shell.Shell

	mu *sync.RWMutex
	wg *sync.WaitGroup

	chain persist.Chain

	metadatas      map[tokenIdentifiers]metadata
	uris           map[tokenIdentifiers]uri
	contractStored map[address]bool
	owners         map[tokenIdentifiers]ownerAtBlock
	balances       map[tokenIdentifiers]tokenBalances
	previousOwners map[tokenIdentifiers][]ownerAtBlock

	eventHashes []EventHash

	lastSyncedBlock uint64
	mostRecentBlock uint64
	statsFile       string

	subscriptions chan types.Log
	transfers     chan []*transfer
	tokens        chan *persist.Token
	contracts     chan *persist.Contract
	done          chan error
	cancel        chan os.Signal

	badURIs uint64

	uniqueMetadatas uniqueMetadatas
}

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens
func NewIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, pChain persist.Chain, pEvents []EventHash, statsFileName string, uniqueMetadatas uniqueMetadatas) *Indexer {
	finalBlockUint, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	statsFile, err := os.Open(statsFileName)
	startingBlock := uint64(defaultERC721Block)
	if err == nil {
		defer statsFile.Close()
		decoder := json.NewDecoder(statsFile)

		var stats map[string]interface{}
		err = decoder.Decode(&stats)
		if err != nil {
			panic(err)
		}
		startingBlock = uint64(stats["last_block"].(float64))
	}

	cancel := make(chan os.Signal)

	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM)

	return &Indexer{

		mu: &sync.RWMutex{},
		wg: &sync.WaitGroup{},

		ethClient:  ethClient,
		ipfsClient: ipfsClient,

		chain: pChain,

		metadatas:      make(map[tokenIdentifiers]metadata),
		uris:           make(map[tokenIdentifiers]uri),
		contractStored: make(map[address]bool),
		owners:         make(map[tokenIdentifiers]ownerAtBlock),
		balances:       make(map[tokenIdentifiers]tokenBalances),
		previousOwners: make(map[tokenIdentifiers][]ownerAtBlock),

		lastSyncedBlock: startingBlock,
		mostRecentBlock: finalBlockUint,

		eventHashes: pEvents,
		statsFile:   statsFileName,

		subscriptions: make(chan types.Log),
		transfers:     make(chan []*transfer),
		tokens:        make(chan *persist.Token),
		contracts:     make(chan *persist.Contract),
		done:          make(chan error),
		cancel:        cancel,

		uniqueMetadatas: uniqueMetadatas,
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	i.wg.Add(1)
	i.state = 1
	go i.processLogs()
	go i.processTransfers()
	go i.processTokens()
	go i.processContracts()
	go i.handleDone()
	i.wg.Wait()
}

func (i *Indexer) processLogs() {

	defer func() {
		go i.subscribeNewLogs()
	}()

	go i.listenForNewBlocks()

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events, nil, nil, nil}

	curBlock := new(big.Int).SetUint64(i.lastSyncedBlock)
	interval := getBlockInterval(1, 2000, i.lastSyncedBlock, i.mostRecentBlock)
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(interval)))

	for nextBlock.Cmp(new(big.Int).SetUint64(atomic.LoadUint64(&i.mostRecentBlock))) == -1 {
		logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		logsTo, err := i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: curBlock,
			ToBlock:   nextBlock,
			Topics:    topics,
		})
		cancel()
		if err != nil {
			logrus.WithError(err).Error("error getting logs, trying again")
			continue
		}
		logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

		for _, log := range logsTo {
			i.transfers <- logToTransfer(log)
		}

		atomic.StoreUint64(&i.lastSyncedBlock, nextBlock.Uint64())

		curBlock.Add(curBlock, big.NewInt(int64(interval)))
		nextBlock.Add(nextBlock, big.NewInt(int64(interval)))
		interval = getBlockInterval(1, 2000, curBlock.Uint64(), i.mostRecentBlock)
	}

}

func (i *Indexer) processTransfers() {

	defer close(i.tokens)
	count := 0
	for transfers := range i.transfers {
		if transfers != nil && len(transfers) > 0 {
			for _, transfer := range transfers {
				go processTransfer(i, transfer)
			}
			if count%10000 == 0 {
				logrus.Infof("Processed %d sets of transfers", count)
				go storedDataToTokens(i)
			}
			count++
		}
	}
	logrus.Info("Transfer channel closed")
	storedDataToTokens(i)
	logrus.Info("Done processing transfers, closing tokens channel")
}

func (i *Indexer) processTokens() {

	for token := range i.tokens {
		go func(t *persist.Token) {
			logrus.Infof("Processing token %s-%s", t.ContractAddress, t.TokenID)
			err := i.tokenReceive(context.Background(), t)
			if err != nil {
				logrus.WithError(err).Error("error processing token")
			}
		}(token)
	}
}

func (i *Indexer) processContracts() {
	for contract := range i.contracts {
		go func(c *persist.Contract) {
			logrus.Infof("Processing contract %+v", c)
			// TODO turn contract into persist.Contract
			err := i.contractReceive(context.Background(), c)
			if err != nil {
				logrus.WithError(err).Error("error processing token")
			}
		}(contract)
	}
}

func (i *Indexer) subscribeNewLogs() {

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events, nil, nil, nil}

	sub, err := i.ethClient.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(i.lastSyncedBlock),
		Topics:    topics,
	}, i.subscriptions)
	if err != nil {
		logrus.Errorf("failed to subscribe to logs: %v", err)
		atomic.StoreInt64(&i.state, -1)
		i.done <- err
	}
	for {
		select {
		case log := <-i.subscriptions:
			logrus.Infof("Got log at: %d", log.BlockNumber)
			i.transfers <- logToTransfer(log)
		case err := <-sub.Err():
			logrus.Errorf("subscription error: %v", err)
			atomic.StoreInt64(&i.state, -1)
			i.done <- err
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
			i.writeStats()
			os.Exit(1)
			return
		case err := <-i.done:
			i.writeStats()
			logrus.Errorf("Indexer done: %v", err)
			os.Exit(0)
			return
		case <-time.After(time.Second * 30):
			i.writeStats()
		}
	}
}

func processTransfer(i *Indexer, transfer *transfer) {

	key := makeKeyForToken(transfer.RawContract.Address, transfer.TokenID)
	logrus.Infof("Processing transfer %s", key)

	contractAddress := address(transfer.RawContract.Address)
	from := address(transfer.From)
	to := address(transfer.To)
	tokenID := tokenID(transfer.TokenID)

	i.mu.Lock()
	defer i.mu.Unlock()

	if !i.contractStored[contractAddress] {
		i.contractStored[contractAddress] = true
		c := &persist.Contract{
			Address:     string(contractAddress),
			LatestBlock: atomic.LoadUint64(&i.lastSyncedBlock),
		}
		cMetadata, err := getTokenContractMetadata(contractAddress, i.ethClient)
		if err != nil {
			logrus.WithError(err).Error("error getting contract metadata")
		} else {
			c.Name = cMetadata.Name
			c.Symbol = cMetadata.Symbol
		}
		i.contracts <- c
	}
	switch persist.TokenType(transfer.Type) {
	case persist.TokenTypeERC721:
		if it, ok := i.owners[key]; ok {
			if it.block < transfer.BlockNumber.Uint64() {
				it.block = transfer.BlockNumber.Uint64()
				it.owner = to
			}
		} else {
			i.owners[key] = ownerAtBlock{from, transfer.BlockNumber.Uint64()}
		}
		if it, ok := i.previousOwners[key]; !ok {
			i.previousOwners[key] = []ownerAtBlock{{
				owner: from,
				block: transfer.BlockNumber.Uint64(),
			}}
		} else {
			it = append(it, ownerAtBlock{
				owner: from,
				block: transfer.BlockNumber.Uint64(),
			})
		}
		if _, ok := i.uris[key]; !ok {
			uri, err := getERC721TokenURI(contractAddress, tokenID, i.ethClient)
			if err != nil {
				logrus.WithError(err).Error("error getting URI for ERC721 token")

			} else {
				i.uris[key] = uri
			}
		}
	case persist.TokenTypeERC1155:
		balances, ok := i.balances[key]
		if !ok {
			balances = make(map[address]*big.Int)
			i.balances[key] = balances
		}
		if it, ok := balances[from]; ok {
			it.Sub(it, new(big.Int).SetUint64(transfer.Amount))
		} else {
			i.balances[key][from] = new(big.Int).SetUint64(transfer.Amount)
		}
		if it, ok := balances[to]; ok {
			it.Add(it, new(big.Int).SetUint64(transfer.Amount))
		} else {
			i.balances[key][to] = new(big.Int).SetUint64(transfer.Amount)
		}
		if _, ok := i.uris[key]; !ok {
			uri, err := getERC1155TokenURI(contractAddress, tokenID, i.ethClient)
			if err != nil {
				logrus.WithError(err).Error("error getting URI for ERC1155 token")

			} else {
				i.uris[key] = uri
			}
		}
	default:
		logrus.Error("unknown token type")
		atomic.StoreInt64(&i.state, -1)
		i.done <- errors.New("unknown token type")
	}

	if _, ok := i.metadatas[key]; !ok {
		if handler, ok := i.uniqueMetadatas[contractAddress]; ok {
			metadata, err := handler(i, contractAddress, tokenID)
			if err != nil {
				logrus.WithError(err).Error("error getting metadata for token")
				atomic.AddUint64(&i.badURIs, 1)
			}
			i.metadatas[key] = metadata
		} else {
			if it, ok := i.uris[key]; ok {
				id, err := util.HexToBigInt(string(tokenID))
				if err != nil {
					logrus.WithError(err).Error("error converting token ID to big int")
					atomic.StoreInt64(&i.state, -1)
					i.done <- err
				}
				uriReplaced := uri(strings.ReplaceAll(string(it), "{id}", id.String()))
				metadata, err := getMetadataFromURI(uriReplaced, i.ipfsClient)
				if err != nil {
					logrus.WithError(err).Error("error getting metadata for token")
					atomic.AddUint64(&i.badURIs, 1)
				} else {
					i.metadatas[key] = metadata
				}
			}
		}
	}
}

func (i *Indexer) tokenReceive(ctx context.Context, t *persist.Token) error {
	if t.TokenURI == "" {
		return errors.New("token URI is empty")
	}
	if t.OwnerAddress == "0x00000000000000000000000000000000000000" {
		return errors.New("token owner is empty")
	}
	i.done <- fmt.Errorf("token %s received at block %d", t.TokenURI, atomic.LoadUint64(&i.lastSyncedBlock))
	// return persist.TokenUpsert(ctx, t, i.runtime)
	return nil
}

func (i *Indexer) contractReceive(ctx context.Context, contract *persist.Contract) error {
	// return persist.ContractUpsertByAddress(ctx, contract.Address, contract, i.runtime)
	return nil
}

func storedDataToTokens(i *Indexer) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for k, v := range i.owners {
		contractAddress, tokenID, err := parseKeyForToken(k)
		if err != nil {
			atomic.StoreInt64(&i.state, -1)
			i.done <- err
		}
		sort.Slice(i.previousOwners[k], func(l, m int) bool {
			return i.previousOwners[k][l].block < i.previousOwners[k][m].block
		})
		previousOwnerAddresses := make([]string, len(i.previousOwners[k]))
		for i, w := range i.previousOwners[k] {
			previousOwnerAddresses[i] = string(toRegularAddress(w.owner))
		}
		metadata := i.metadatas[k]
		var name, description string
		if metadata != nil {
			if v, ok := findFirstFieldFromMetadata(metadata, "name").(string); ok {
				name = v
			}
			if v, ok := findFirstFieldFromMetadata(metadata, "description").(string); ok {
				description = v
			}
		}
		token := &persist.Token{
			TokenID:         string(tokenID),
			ContractAddress: string(contractAddress),
			OwnerAddress:    string(toRegularAddress(v.owner)),
			Amount:          1,
			Name:            name,
			Description:     description,
			PreviousOwners:  previousOwnerAddresses,
			TokenType:       persist.TokenTypeERC721,
			TokenMetadata:   metadata,
			TokenURI:        string(i.uris[k]),
			Chain:           i.chain,
			LatestBlock:     atomic.LoadUint64(&i.lastSyncedBlock),
		}
		i.tokens <- token
	}
	for k, v := range i.balances {
		contractAddress, tokenID, err := parseKeyForToken(k)
		if err != nil {
			atomic.StoreInt64(&i.state, -1)
			i.done <- err
		}

		metadata := i.metadatas[k]
		var name, description string
		if metadata != nil {
			if v, ok := findFirstFieldFromMetadata(metadata, "name").(string); ok {
				name = v
			}
			if v, ok := findFirstFieldFromMetadata(metadata, "description").(string); ok {
				description = v
			}
		}
		uri := i.uris[k]
		for addr, balance := range v {
			token := &persist.Token{
				TokenID:         string(tokenID),
				ContractAddress: string(contractAddress),
				OwnerAddress:    string(toRegularAddress(addr)),
				Amount:          balance.Uint64(),
				TokenType:       persist.TokenTypeERC1155,
				TokenMetadata:   metadata,
				TokenURI:        string(uri),
				Name:            name,
				Description:     description,
				Chain:           i.chain,
				LatestBlock:     atomic.LoadUint64(&i.lastSyncedBlock),
			}
			i.tokens <- token
		}
	}
}

func logToTransfer(pLog types.Log) []*transfer {

	switch pLog.Topics[0].Hex() {
	case string(TransferEventHash):
		if len(pLog.Topics) != 4 {
			// logrus.Error("Invalid transfer event")
			return nil
		}

		return []*transfer{
			{
				From:        pLog.Topics[1].Hex(),
				To:          pLog.Topics[2].Hex(),
				TokenID:     pLog.Topics[3].Hex(),
				Amount:      1,
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC721,
			},
		}
	case string(TransferSingleEventHash):
		if len(pLog.Topics) != 4 {
			logrus.Error("Invalid transfer single event")
			return nil
		}

		return []*transfer{
			{
				From:        pLog.Topics[2].Hex(),
				To:          pLog.Topics[3].Hex(),
				TokenID:     common.BytesToHash(pLog.Data[:len(pLog.Data)/2]).Hex(),
				Amount:      common.BytesToHash(pLog.Data[len(pLog.Data)/2:]).Big().Uint64(),
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC1155,
			},
		}

	case string(TransferBatchEventHash):
		if len(pLog.Topics) != 4 {
			logrus.Error("Invalid transfer batch event")
			return nil
		}
		from := pLog.Topics[2].Hex()
		to := pLog.Topics[3].Hex()
		amountOffset := len(pLog.Data) / 2
		total := amountOffset / 64
		result := make([]*transfer, total)

		for i := 0; i < total; i++ {
			result[i] = &transfer{
				From:    from,
				To:      to,
				TokenID: common.BytesToHash(pLog.Data[i*64 : (i+1)*64]).Hex(),
				Amount:  common.BytesToHash(pLog.Data[(amountOffset)+(i*64) : (amountOffset)+((i+1)*64)]).Big().Uint64(),
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
	logrus.Info("Writing Stats...")
	i.mu.RLock()
	defer i.mu.RUnlock()

	fi, err := os.Create(i.statsFile)
	if err != nil {
		logrus.WithError(err).Error("error creating stats file")
		return
	}
	defer fi.Close()
	stats := map[string]interface{}{}
	stats["state"] = atomic.LoadInt64(&i.state)
	stats["total_erc721"] = len(i.owners)
	stats["total_erc1155"] = len(i.balances)
	stats["total_metadatas"] = len(i.metadatas)
	stats["total_uris"] = len(i.uris)
	stats["last_block"] = atomic.LoadUint64(&i.lastSyncedBlock)
	// stats["all_tokens"] = testAllTokens[(len(testAllTokens)/10)*9:]
	stats["bad_uris"] = atomic.LoadUint64(&i.badURIs)
	bs, err := json.Marshal(stats)
	if err != nil {
		logrus.WithError(err).Error("error marshalling stats")
		return
	}
	_, err = io.Copy(fi, bytes.NewReader(bs))
	if err != nil {
		logrus.WithError(err).Error("error writing stats")
		return
	}
}

func (i *Indexer) listenForNewBlocks() {
	for {
		finalBlockUint, err := i.ethClient.BlockNumber(context.Background())
		if err != nil {
			logrus.Errorf("failed to get block number: %v", err)
			atomic.StoreInt64(&i.state, -1)
			i.done <- err
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logrus.Infof("final block number: %v", finalBlockUint)
		time.Sleep(time.Minute)
	}
}

func findFirstFieldFromMetadata(metadata map[string]interface{}, fields ...string) interface{} {
	for _, field := range fields {
		if v, ok := metadata[field]; ok {
			return v
		}
		if v, ok := metadata["properties"].(map[string]interface{}); ok {
			if v, ok := v[field]; ok {
				return v
			}
		}
		if v, ok := metadata["traits"].(map[string]interface{}); ok {
			if v, ok := v[field]; ok {
				return v
			}
		}
		if v, ok := metadata["attributes"].(map[string]interface{}); ok {
			if v, ok := v[field]; ok {
				return v
			}
		}
	}
	return nil
}

// function that returns a progressively smaller value between min and max for every million block numbers
func getBlockInterval(min, max, blockNumber, lastBlockNumber uint64) uint64 {
	blockDivisor := lastBlockNumber / 20
	if blockNumber < blockDivisor {
		return max
	}
	return (max - min) / (blockNumber / blockDivisor)
}

func toRegularAddress(addr address) address {
	return address(strings.ToLower(fmt.Sprintf("0x%s", addr[len(addr)-38:])))
}

func makeKeyForToken(contractAddress, tokenID string) tokenIdentifiers {
	return tokenIdentifiers(fmt.Sprintf("%s_%s", contractAddress, tokenID))
}

func parseKeyForToken(key tokenIdentifiers) (address, tokenID, error) {
	parts := strings.Split(string(key), "_")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid key")
	}
	return address(parts[0]), tokenID(parts[1]), nil
}
