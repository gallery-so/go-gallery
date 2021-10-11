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

// TODO why does CTRL+C not work?

type address string

type tokenID string

type tokenIdentifiers string

type metadata map[string]interface{}
type uri string
type tokenMetadata struct {
	ti tokenIdentifiers
	md metadata
}

type tokenBalances struct {
	ti   tokenIdentifiers
	from address
	to   address
	amt  *big.Int
}

type tokenURI struct {
	ti  tokenIdentifiers
	uri uri
}

type uniqueMetadataHandler func(*Indexer, uri, address, tokenID) (metadata, error)

type uniqueMetadatas map[address]uniqueMetadataHandler

type ownerAtBlock struct {
	ti    tokenIdentifiers
	owner address
	block uint64
}

// Indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type Indexer struct {
	state int64

	ethClient    *ethclient.Client
	ipfsClient   *shell.Shell
	tokenRepo    persist.TokenRepository
	contractRepo persist.ContractRepository

	chain persist.Chain

	eventHashes []EventHash

	lastSyncedBlock uint64
	mostRecentBlock uint64
	statsFile       *os.File

	metadatas      chan tokenMetadata
	uris           chan tokenURI
	owners         chan ownerAtBlock
	balances       chan tokenBalances
	previousOwners chan ownerAtBlock

	subscriptions chan types.Log
	transfers     chan []*transfer
	tokens        chan []*persist.Token
	contracts     chan *persist.Contract
	done          chan error
	cancel        chan os.Signal

	badURIs uint64

	uniqueMetadatas uniqueMetadatas
}

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens
func NewIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, pChain persist.Chain, pEvents []EventHash, statsFileName string) *Indexer {
	finalBlockUint, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	statsFile, err := os.Open(statsFileName)
	startingBlock := uint64(defaultERC721Block)
	if err == nil {
		decoder := json.NewDecoder(statsFile)

		var stats map[string]interface{}
		err = decoder.Decode(&stats)
		if err != nil {
			panic(err)
		}
		startingBlock = uint64(stats["last_block"].(float64))
	} else {
		fi, err := os.Create(statsFileName)
		if err != nil {
			panic(err)
		}
		statsFile = fi
	}

	cancel := make(chan os.Signal)

	signal.Notify(cancel, syscall.SIGINT, syscall.SIGTERM)

	return &Indexer{

		ethClient:    ethClient,
		ipfsClient:   ipfsClient,
		tokenRepo:    tokenRepo,
		contractRepo: contractRepo,

		chain: pChain,

		lastSyncedBlock: startingBlock,
		mostRecentBlock: finalBlockUint,

		eventHashes: pEvents,
		statsFile:   statsFile,

		metadatas:      make(chan tokenMetadata),
		uris:           make(chan tokenURI),
		owners:         make(chan ownerAtBlock),
		balances:       make(chan tokenBalances),
		previousOwners: make(chan ownerAtBlock),

		subscriptions: make(chan types.Log),
		transfers:     make(chan []*transfer),
		tokens:        make(chan []*persist.Token),
		contracts:     make(chan *persist.Contract),
		done:          make(chan error),
		cancel:        cancel,

		uniqueMetadatas: getUniqueMetadataHandlers(),
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	defer i.statsFile.Close()
	i.state = 1
	go i.processLogs()
	go i.processTransfers()
	go i.processTokens()
	go i.processContracts()
	i.handleDone()
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

		i.transfers <- logsToTransfer(logsTo)

		atomic.StoreUint64(&i.lastSyncedBlock, nextBlock.Uint64())

		curBlock.Add(curBlock, big.NewInt(int64(interval)))
		nextBlock.Add(nextBlock, big.NewInt(int64(interval)))
		interval = getBlockInterval(1, 2000, curBlock.Uint64(), atomic.LoadUint64(&i.mostRecentBlock))
	}

}

func (i *Indexer) processTransfers() {

	defer close(i.tokens)
	for transfers := range i.transfers {
		if transfers != nil && len(transfers) > 0 {
			go processTransfers(i, transfers)
		}
	}
	logrus.Info("Transfer channel closed")
	logrus.Info("Done processing transfers, closing tokens channel")
}

func (i *Indexer) processTokens() {

	go func() {
		for tokens := range i.tokens {
			go func(t []*persist.Token) {
				for _, token := range t {
					logrus.Infof("Processing token %s-%s", token.ContractAddress, token.TokenID)
					err := i.tokenReceive(context.Background(), token)
					if err != nil {
						logrus.WithError(err).Error("error processing token")
					}
				}
			}(tokens)
		}
	}()

	mu := &sync.Mutex{}
	owners := map[tokenIdentifiers]ownerAtBlock{}
	previousOwners := map[tokenIdentifiers][]ownerAtBlock{}
	balances := map[tokenIdentifiers]map[address]*big.Int{}
	metadatas := map[tokenIdentifiers]tokenMetadata{}
	uris := map[tokenIdentifiers]tokenURI{}

	go func() {
		for {
			<-time.After(time.Second * 25)
			mu.Lock()
			i.tokens <- i.storedDataToTokens(owners, previousOwners, balances, metadatas, uris)
			mu.Unlock()
		}
	}()

	for {
		mu.Lock()
		select {
		case owner := <-i.owners:
			if it, ok := owners[owner.ti]; ok {
				if it.block < owner.block {
					owners[owner.ti] = owner
				}
			} else {
				owners[owner.ti] = owner
			}
		case balance := <-i.balances:
			if balances[balance.ti] == nil {
				balances[balance.ti] = map[address]*big.Int{}
			}
			if balances[balance.ti][balance.to] == nil {
				balances[balance.ti][balance.to] = big.NewInt(0)
			}
			balances[balance.ti][balance.to].Add(balances[balance.ti][balance.to], balance.amt)
			if balances[balance.ti][balance.from] == nil {
				balances[balance.ti][balance.from] = big.NewInt(0)
			}
			balances[balance.ti][balance.from].Sub(balances[balance.ti][balance.from], balance.amt)
		case metadata := <-i.metadatas:
			metadatas[metadata.ti] = metadata
		case uri := <-i.uris:
			uris[uri.ti] = uri
		case previousOwner := <-i.previousOwners:
			if previousOwners[previousOwner.ti] == nil {
				previousOwners[previousOwner.ti] = []ownerAtBlock{}
			}
			previousOwners[previousOwner.ti] = append(previousOwners[previousOwner.ti], previousOwner)
		}
		mu.Unlock()
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
		i.failWithMessage(err, "error subscribing to logs")
		return
	}
	for {
		select {
		case log := <-i.subscriptions:
			logrus.Infof("Got log at: %d", log.BlockNumber)
			i.transfers <- logsToTransfer([]types.Log{log})
		case err := <-sub.Err():
			i.failWithMessage(err, "subscription error")
			return
		case <-i.done:
			return
		}
	}
}

func (i *Indexer) handleDone() {
	for {
		select {
		case <-i.cancel:
			i.writeStats()
			os.Exit(1)
			return
		case err := <-i.done:
			i.writeStats()
			logrus.Errorf("Indexer done: %v", err)
			panic(err)
		case <-time.After(time.Second * 30):
			i.writeStats()
		}
	}
}

func processTransfers(i *Indexer, transfers []*transfer) {

	for _, transfer := range transfers {
		contractAddress := address(transfer.RawContract.Address)
		from := toRegularAddress(address(transfer.From))
		to := toRegularAddress(address(transfer.To))
		tokenID := tokenID(transfer.TokenID)

		key := makeKeyForToken(contractAddress, tokenID)
		logrus.Infof("Processing transfer %s", key)

		var u uri
		switch persist.TokenType(transfer.Type) {
		case persist.TokenTypeERC721:
			i.owners <- ownerAtBlock{key, to, transfer.BlockNumber.Uint64()}
			i.previousOwners <- ownerAtBlock{key, from, transfer.BlockNumber.Uint64()}

			uri, err := getERC721TokenURI(contractAddress, tokenID, i.ethClient)
			if err != nil {
				logrus.WithError(err).Error("error getting URI for ERC721 token")
			} else {
				u = uri
			}

		case persist.TokenTypeERC1155:
			i.balances <- tokenBalances{key, from, to, new(big.Int).SetUint64(transfer.Amount)}

			uri, err := getERC1155TokenURI(contractAddress, tokenID, i.ethClient)
			if err != nil {
				logrus.WithError(err).Error("error getting URI for ERC1155 token")
			} else {
				u = uri
			}

		default:
			i.failWithMessage(errors.New("token type"), "unknown token type")
		}

		go func() {
			i.contracts <- handleContract(i.ethClient, contractAddress, atomic.LoadUint64(&i.lastSyncedBlock))
		}()

		i.uris <- tokenURI{key, u}

		id, err := util.HexToBigInt(string(tokenID))
		if err != nil {
			i.failWithMessage(err, "failed to convert token ID to big int")
			return
		}
		uriReplaced := uri(strings.ReplaceAll(string(u), "{id}", id.String()))
		go func() {
			if handler, ok := i.uniqueMetadatas[contractAddress]; ok {
				if metadata, err := handler(i, uriReplaced, contractAddress, tokenID); err != nil {
					logrus.WithError(err).Error("error getting metadata for token")
					atomic.AddUint64(&i.badURIs, 1)
				} else {
					i.metadatas <- tokenMetadata{key, metadata}
					// meta, err := makePreviewsForMetadata(context.TODO(), metadata, string(contractAddress), string(tokenID), string(uriReplaced), i.ipfsClient)
					// if err != nil {
					// 	logrus.WithError(err).Error(fmt.Printf("error getting previews for token %s", uriReplaced))
					// } else {
					// 	logrus.WithField("tokenURI", uriReplaced).Infof("%+v", *meta)
					// }
				}
			} else {
				if metadata, err := getMetadataFromURI(uriReplaced, i.ipfsClient); err != nil {
					logrus.WithError(err).Error("error getting metadata for token")
					atomic.AddUint64(&i.badURIs, 1)
				} else {
					i.metadatas <- tokenMetadata{key, metadata}
					// meta, err := makePreviewsForMetadata(context.TODO(), metadata, string(contractAddress), string(tokenID), string(uriReplaced), i.ipfsClient)
					// if err != nil {
					// 	logrus.WithError(err).Error(fmt.Printf("error getting previews for token %s", uriReplaced))
					// } else {
					// 	logrus.WithField("tokenURI", uriReplaced).Infof("%+v", *meta)
					// }
				}
			}
		}()
	}
}

func (i *Indexer) tokenReceive(ctx context.Context, t *persist.Token) error {
	if t.TokenURI == "" {
		return errors.New("token URI is empty")
	}
	if t.OwnerAddress == "0x00000000000000000000000000000000000000" {
		return errors.New("token owner is empty")
	}
	if err := i.tokenRepo.Upsert(ctx, t); err != nil {
		return err
	}
	return nil
}

func (i *Indexer) contractReceive(ctx context.Context, contract *persist.Contract) error {
	return i.contractRepo.UpsertByAddress(ctx, contract.Address, contract)
}

func (i *Indexer) storedDataToTokens(owners map[tokenIdentifiers]ownerAtBlock, previousOwners map[tokenIdentifiers][]ownerAtBlock, balances map[tokenIdentifiers]map[address]*big.Int, metadatas map[tokenIdentifiers]tokenMetadata, uris map[tokenIdentifiers]tokenURI) []*persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v)
	}
	result := make([]*persist.Token, len(owners)+totalBalances)
	j := 0
	for _, v := range previousOwners {
		sort.Slice(v, func(i, j int) bool {
			return v[i].block < v[j].block
		})
	}
	for k, v := range owners {
		contractAddress, tokenID, err := parseKeyForToken(k)
		if err != nil {
			i.failWithMessage(err, "failed to parse key for token")
			return nil
		}
		previousOwnerAddresses := make([]string, len(previousOwners[k]))
		for i, w := range previousOwners[k] {
			previousOwnerAddresses[i] = strings.ToLower(string(w.owner))
		}
		metadata := metadatas[k]
		var name, description string

		if w, ok := findFirstFieldFromMetadata(metadata.md, "name").(string); ok {
			name = w
		}
		if w, ok := findFirstFieldFromMetadata(metadata.md, "description").(string); ok {
			description = w
		}

		result[j] = &persist.Token{
			TokenID:         string(tokenID),
			ContractAddress: strings.ToLower(string(contractAddress)),
			OwnerAddress:    strings.ToLower(string(v.owner)),
			Amount:          1,
			Name:            name,
			Description:     description,
			PreviousOwners:  previousOwnerAddresses,
			TokenType:       persist.TokenTypeERC721,
			TokenMetadata:   metadata.md,
			TokenURI:        string(uris[k].uri),
			Chain:           i.chain,
			LatestBlock:     atomic.LoadUint64(&i.lastSyncedBlock),
		}
		j++
	}
	for k, v := range balances {
		contractAddress, tokenID, err := parseKeyForToken(k)
		if err != nil {
			i.failWithMessage(err, "failed to parse key for token")
			return nil
		}

		metadata := metadatas[k]
		var name, description string

		if v, ok := findFirstFieldFromMetadata(metadata.md, "name").(string); ok {
			name = v
		}
		if v, ok := findFirstFieldFromMetadata(metadata.md, "description").(string); ok {
			description = v
		}

		uri := uris[k]
		for addr, balance := range v {
			result[j] = &persist.Token{
				TokenID:         string(tokenID),
				ContractAddress: strings.ToLower(string(contractAddress)),
				OwnerAddress:    strings.ToLower(string(addr)),
				Amount:          balance.Uint64(),
				TokenType:       persist.TokenTypeERC1155,
				TokenMetadata:   metadata.md,
				TokenURI:        string(uri.uri),
				Name:            name,
				Description:     description,
				Chain:           i.chain,
				LatestBlock:     atomic.LoadUint64(&i.lastSyncedBlock),
			}
			j++
		}
	}

	return result
}

func logsToTransfer(pLogs []types.Log) []*transfer {
	result := []*transfer{}
	for _, pLog := range pLogs {
		switch pLog.Topics[0].Hex() {
		case string(TransferEventHash):
			if len(pLog.Topics) != 4 {
				continue
			}

			result = append(result, &transfer{
				From:        pLog.Topics[1].Hex(),
				To:          pLog.Topics[2].Hex(),
				TokenID:     pLog.Topics[3].Hex(),
				Amount:      1,
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC721,
			})
		case string(TransferSingleEventHash):
			if len(pLog.Topics) != 4 {
				continue
			}

			result = append(result, &transfer{
				From:        pLog.Topics[2].Hex(),
				To:          pLog.Topics[3].Hex(),
				TokenID:     common.BytesToHash(pLog.Data[:len(pLog.Data)/2]).Hex(),
				Amount:      common.BytesToHash(pLog.Data[len(pLog.Data)/2:]).Big().Uint64(),
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
				Type: persist.TokenTypeERC1155,
			})

		case string(TransferBatchEventHash):
			if len(pLog.Topics) != 4 {
				continue
			}
			from := pLog.Topics[2].Hex()
			to := pLog.Topics[3].Hex()
			amountOffset := len(pLog.Data) / 2
			total := amountOffset / 64
			this := make([]*transfer, total)

			for j := 0; j < total; j++ {
				this[j] = &transfer{
					From:    from,
					To:      to,
					TokenID: common.BytesToHash(pLog.Data[j*64 : (j+1)*64]).Hex(),
					Amount:  common.BytesToHash(pLog.Data[(amountOffset)+(j*64) : (amountOffset)+((j+1)*64)]).Big().Uint64(),
					RawContract: contract{
						Address: pLog.Address.Hex(),
					},
					Type: persist.TokenTypeERC1155,
				}
			}
			result = append(result, this...)
		}
	}
	return result
}

func (i *Indexer) writeStats() {
	logrus.Info("Writing Stats...")

	stats := map[string]interface{}{}
	stats["state"] = atomic.LoadInt64(&i.state)
	stats["last_block"] = atomic.LoadUint64(&i.lastSyncedBlock)
	stats["bad_uris"] = atomic.LoadUint64(&i.badURIs)
	bs, err := json.Marshal(stats)
	if err != nil {
		i.failWithMessage(err, "failed to marshal stats")
	}
	_, err = io.Copy(i.statsFile, bytes.NewReader(bs))
	if err != nil {
		i.failWithMessage(err, "failed to write stats")
	}
}

func (i *Indexer) listenForNewBlocks() {
	for {
		finalBlockUint, err := i.ethClient.BlockNumber(context.Background())
		if err != nil {
			i.failWithMessage(err, "failed to get block number")
			return
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logrus.Infof("final block number: %v", finalBlockUint)
		time.Sleep(time.Minute)
	}
}

func (i *Indexer) failWithMessage(err error, msg string) {
	logrus.WithError(err).Error(msg)
	atomic.StoreInt64(&i.state, -1)
	i.done <- err
}

func getUniqueMetadataHandlers() uniqueMetadatas {
	return uniqueMetadatas{
		address(strings.ToLower("0xd4e4078ca3495DE5B1d4dB434BEbc5a986197782")): autoglyphs,
		address(strings.ToLower("0x60F3680350F65Beb2752788cB48aBFCE84a4759E")): colorglyphs,
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

func makeKeyForToken(contractAddress address, tokenID tokenID) tokenIdentifiers {
	return tokenIdentifiers(fmt.Sprintf("%s_%s", contractAddress, tokenID))
}

func parseKeyForToken(key tokenIdentifiers) (address, tokenID, error) {
	parts := strings.Split(string(key), "_")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid key")
	}
	return address(parts[0]), tokenID(parts[1]), nil
}

func handleContract(ethClient *ethclient.Client, contractAddress address, lastSyncedBlock uint64) *persist.Contract {
	c := &persist.Contract{
		Address:     string(contractAddress),
		LatestBlock: lastSyncedBlock,
	}
	cMetadata, err := getTokenContractMetadata(contractAddress, ethClient)
	if err != nil {
		logrus.WithError(err).Error("error getting contract metadata")
	} else {
		c.Name = cMetadata.Name
		c.Symbol = cMetadata.Symbol
	}
	return c
}
