package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/getsentry/sentry-go"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/alchemy"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/conc/iter"
)

func init() {
	env.RegisterValidation("GCLOUD_TOKEN_CONTENT_BUCKET", "required")
	env.RegisterValidation("ALCHEMY_API_URL", "required")
}

const (
	// transferEventHash represents the keccak256 hash of Transfer(address,address,uint256)
	transferEventHash eventHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// transferSingleEventHash represents the keccak256 hash of TransferSingle(address,address,address,uint256,uint256)
	transferSingleEventHash eventHash = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	// transferBatchEventHash represents the keccak256 hash of TransferBatch(address,address,address,uint256[],uint256[])
	transferBatchEventHash eventHash = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
	// uriEventHash represents the keccak256 hash of URI(string,uint256)
	uriEventHash eventHash = "0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b"

	defaultWorkerPoolSize     = 3
	defaultWorkerPoolWaitSize = 10
	blocksPerLogsCall         = 50
)

var (
	rpcEnabled            bool = false // Enables external RPC calls
	erc1155ABI, _              = contracts.IERC1155MetaData.GetAbi()
	animationKeywords          = []string{"animation", "video"}
	imageKeywords              = []string{"image"}
	defaultTransferEvents      = []eventHash{
		transferBatchEventHash,
		transferEventHash,
		transferSingleEventHash,
	}
)

type errForTokenAtBlockAndIndex struct {
	err error
	boi blockchainOrderInfo
	ti  persist.EthereumTokenIdentifiers
}

func (e errForTokenAtBlockAndIndex) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return e.ti
}

func (e errForTokenAtBlockAndIndex) OrderInfo() blockchainOrderInfo {
	return e.boi
}

// eventHash represents an event keccak256 hash
type eventHash string

type transfersAtBlock struct {
	block     persist.BlockNumber
	transfers []rpc.Transfer
}

type contractAtBlock struct {
	ti       persist.EthereumTokenIdentifiers
	boi      blockchainOrderInfo
	contract persist.Contract
}

func (o contractAtBlock) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return o.ti
}

func (o contractAtBlock) OrderInfo() blockchainOrderInfo {
	return o.boi
}

type getLogsFunc func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error)

// indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type indexer struct {
	ethClient         *ethclient.Client
	httpClient        *http.Client
	ipfsClient        *shell.Shell
	arweaveClient     *goar.Client
	storageClient     *storage.Client
	tokenRepo         persist.TokenRepository
	contractRepo      persist.ContractRepository
	addressFilterRepo refresh.AddressFilterRepository
	dbMu              *sync.Mutex // Manages writes to the db
	stateMu           *sync.Mutex // Manages updates to the indexer's state
	memoryMu          *sync.Mutex // Manages large memory operations

	tokenBucket string

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64  // Current height of the blockchain
	lastSyncedChunk uint64  // Start block of the last chunk handled by the indexer
	maxBlock        *uint64 // If provided, the indexer will only index up to maxBlock

	contractOwnerStats *sync.Map // map[contractOwnerMethod]int - Used to track the number of times a contract owner method is used

	isListening bool // Indicates if the indexer is waiting for new blocks

	getLogsFunc getLogsFunc

	contractDBHooks []DBHook[persist.Contract]
	tokenDBHooks    []DBHook[persist.Token]
}

// newIndexer sets up an indexer for retrieving the specified events that will process tokens
func newIndexer(ethClient *ethclient.Client, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, addressFilterRepo refresh.AddressFilterRepository, pChain persist.Chain, pEvents []eventHash, getLogsFunc getLogsFunc, startingBlock, maxBlock *uint64) *indexer {
	if rpcEnabled && ethClient == nil {
		panic("RPC is enabled but an ethClient wasn't provided!")
	}

	ownerStats := &sync.Map{}

	i := &indexer{
		ethClient:         ethClient,
		httpClient:        httpClient,
		ipfsClient:        ipfsClient,
		arweaveClient:     arweaveClient,
		storageClient:     storageClient,
		tokenRepo:         tokenRepo,
		contractRepo:      contractRepo,
		addressFilterRepo: addressFilterRepo,
		dbMu:              &sync.Mutex{},
		stateMu:           &sync.Mutex{},
		memoryMu:          &sync.Mutex{},

		tokenBucket: env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"),

		chain: pChain,

		maxBlock: maxBlock,

		contractOwnerStats: ownerStats,

		eventHashes: pEvents,

		getLogsFunc: getLogsFunc,

		contractDBHooks: newContractHooks(contractRepo, ethClient, httpClient, ownerStats),
		tokenDBHooks:    newTokenHooks(),

		mostRecentBlock: 0,
		lastSyncedChunk: 0,
		isListening:     false,
	}

	if startingBlock != nil {
		i.lastSyncedChunk = *startingBlock
		i.lastSyncedChunk -= i.lastSyncedChunk % blocksPerLogsCall
	} else {
		recentDBBlock, err := tokenRepo.MostRecentBlock(context.Background())
		if err != nil {
			panic(err)
		}
		i.lastSyncedChunk = recentDBBlock.Uint64()

		safeSub, overflowed := math.SafeSub(i.lastSyncedChunk, (i.lastSyncedChunk%blocksPerLogsCall)+(blocksPerLogsCall*defaultWorkerPoolSize))

		if overflowed {
			i.lastSyncedChunk = 0
		} else {
			i.lastSyncedChunk = safeSub
		}

	}

	if maxBlock != nil {
		i.mostRecentBlock = *maxBlock
	} else if rpcEnabled {
		mostRecentBlock, err := ethClient.BlockNumber(context.Background())
		if err != nil {
			panic(err)
		}
		i.mostRecentBlock = mostRecentBlock
	}

	if i.lastSyncedChunk > i.mostRecentBlock {
		panic(fmt.Sprintf("last handled chunk=%d is greater than the height=%d!", i.lastSyncedChunk, i.mostRecentBlock))
	}

	if i.getLogsFunc == nil {
		i.getLogsFunc = i.defaultGetLogs
	}

	logger.For(nil).Infof("starting indexer at block=%d until block=%d with rpc enabled: %t", i.lastSyncedChunk, i.mostRecentBlock, rpcEnabled)
	return i
}

// INITIALIZATION FUNCS ---------------------------------------------------------

// Start begins indexing events from the blockchain
func (i *indexer) Start(ctx context.Context) {
	if rpcEnabled && i.maxBlock == nil {
		go i.listenForNewBlocks(sentryutil.NewSentryHubContext(ctx))
	}

	topics := eventsToTopics(i.eventHashes)

	logger.For(ctx).Info("Catching up to latest block")
	i.isListening = false
	i.catchUp(ctx, topics)

	if !rpcEnabled {
		logger.For(ctx).Info("Running in cached logs only mode, not listening for new logs")
		return
	}

	logger.For(ctx).Info("Subscribing to new logs")
	i.isListening = true
	i.waitForBlocks(ctx, topics)
}

// catchUp processes logs up to the most recent block.
func (i *indexer) catchUp(ctx context.Context, topics [][]common.Hash) {
	wp := workerpool.New(defaultWorkerPoolSize)
	defer wp.StopWait()

	go func() {
		time.Sleep(10 * time.Second)
		for wp.WaitingQueueSize() > 0 {
			logger.For(ctx).Infof("Catching up: waiting for %d workers to finish", wp.WaitingQueueSize())
			time.Sleep(10 * time.Second)
		}
	}()

	from := i.lastSyncedChunk
	for ; from < atomic.LoadUint64(&i.mostRecentBlock); from += blocksPerLogsCall {
		input := from
		toQueue := func() {
			workerCtx := sentryutil.NewSentryHubContext(ctx)
			defer recoverAndWait(workerCtx)
			defer sentryutil.RecoverAndRaise(workerCtx)
			logger.For(workerCtx).Infof("Indexing block range starting at %d", input)
			i.startPipeline(workerCtx, persist.BlockNumber(input), topics)
			i.updateLastSynced(input)
			logger.For(workerCtx).Infof("Finished indexing block range starting at %d", input)
		}
		if wp.WaitingQueueSize() > defaultWorkerPoolWaitSize {
			wp.SubmitWait(toQueue)
		} else {
			wp.Submit(toQueue)
		}
	}
}

func (i *indexer) updateLastSynced(block uint64) {
	i.stateMu.Lock()
	if i.lastSyncedChunk < block {
		i.lastSyncedChunk = block
	}
	i.stateMu.Unlock()
}

// waitForBlocks polls for new blocks.
func (i *indexer) waitForBlocks(ctx context.Context, topics [][]common.Hash) {
	for {
		timeAfterWait := <-time.After(time.Minute * 3)
		i.startNewBlocksPipeline(ctx, topics)
		logger.For(ctx).Infof("Waiting for new blocks... Finished recent blocks in %s", time.Since(timeAfterWait))
	}
}

func (i *indexer) startPipeline(ctx context.Context, start persist.BlockNumber, topics [][]common.Hash) {
	span, ctx := tracing.StartSpan(ctx, "indexer.pipeline", "catchup", sentry.TransactionName("indexer-main:catchup"))
	tracing.AddEventDataToSpan(span, map[string]interface{}{"block": start})
	defer tracing.FinishSpan(span)

	startTime := time.Now()
	transfers := make(chan []transfersAtBlock)
	plugins := NewTransferPlugins(ctx)
	enabledPlugins := []chan<- TransferPluginMsg{plugins.contracts.in}

	logsToCheckAgainst := make(chan []types.Log)
	go func() {
		ctx := sentryutil.NewSentryHubContext(ctx)
		span, ctx := tracing.StartSpan(ctx, "indexer.logs", "processLogs")
		defer tracing.FinishSpan(span)

		logs := i.fetchLogs(ctx, start, topics)
		i.processLogs(ctx, transfers, logs)
		logsToCheckAgainst <- logs
	}()
	go i.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transfers, enabledPlugins)
	i.processTokens(ctx, plugins.contracts.out)

	logger.For(ctx).Warnf("Finished processing %d blocks from block %d in %s", blocksPerLogsCall, start.Uint64(), time.Since(startTime))
}

func (i *indexer) startNewBlocksPipeline(ctx context.Context, topics [][]common.Hash) {
	span, ctx := tracing.StartSpan(ctx, "indexer.pipeline", "polling", sentry.TransactionName("indexer-main:polling"))
	defer tracing.FinishSpan(span)

	transfers := make(chan []transfersAtBlock)
	plugins := NewTransferPlugins(ctx)
	enabledPlugins := []chan<- TransferPluginMsg{plugins.contracts.in}

	go i.pollNewLogs(sentryutil.NewSentryHubContext(ctx), transfers, topics)
	go i.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transfers, enabledPlugins)
	i.processTokens(ctx, plugins.contracts.out)

}

func (i *indexer) listenForNewBlocks(ctx context.Context) {
	defer sentryutil.RecoverAndRaise(ctx)

	for {
		<-time.After(time.Second*12*time.Duration(blocksPerLogsCall) + time.Minute)
		finalBlockUint, err := rpc.RetryGetBlockNumber(ctx, i.ethClient)
		if err != nil {
			panic(fmt.Sprintf("error getting block number: %s", err))
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logger.For(ctx).Debugf("final block number: %v", finalBlockUint)
	}
}

// LOGS FUNCS ---------------------------------------------------------------

func (i *indexer) fetchLogs(ctx context.Context, startingBlock persist.BlockNumber, topics [][]common.Hash) []types.Log {
	curBlock := startingBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(blocksPerLogsCall)))

	logger.For(ctx).Infof("Getting logs from %d to %d", curBlock, nextBlock)

	logsTo, err := i.getLogsFunc(ctx, curBlock, nextBlock, topics)
	if err != nil {
		panic(fmt.Sprintf("error getting logs: %s", err))
	}

	logger.For(ctx).Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

	return logsTo
}

func (i *indexer) defaultGetLogs(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error) {
	var logsTo []types.Log
	reader, err := i.storageClient.Bucket(env.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%d-%d", curBlock, nextBlock)).NewReader(ctx)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("error getting logs from GCP")
	} else {
		func() {
			logger.For(ctx).Infof("Reading logs from GCP")
			i.memoryMu.Lock()
			defer i.memoryMu.Unlock()
			defer reader.Close()
			err = json.NewDecoder(reader).Decode(&logsTo)
			if err != nil {
				panic(err)
			}
		}()
	}

	if len(logsTo) > 0 {
		lastLog := logsTo[len(logsTo)-1]
		if nextBlock.Uint64()-lastLog.BlockNumber > (blocksPerLogsCall / 5) {
			logger.For(ctx).Warnf("Last log is %d blocks old, skipping", nextBlock.Uint64()-lastLog.BlockNumber)
			logsTo = []types.Log{}
		}
	}

	rpcCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if len(logsTo) == 0 && rpcEnabled {
		logger.For(ctx).Infof("Reading logs from Blockchain")
		logsTo, err = rpc.RetryGetLogs(rpcCtx, i.ethClient, ethereum.FilterQuery{
			FromBlock: curBlock,
			ToBlock:   nextBlock,
			Topics:    topics,
		})
		if err != nil {
			logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
				"fromBlock": curBlock.String(),
				"toBlock":   nextBlock.String(),
				"rpcCall":   "eth_getFilterLogs",
			})
			if rpcErr, ok := err.(gethrpc.Error); ok {
				logEntry = logEntry.WithFields(logrus.Fields{"rpcErrorCode": strconv.Itoa(rpcErr.ErrorCode())})
			}
			logEntry.Error("failed to fetch logs")
			return []types.Log{}, nil
		}
		go saveLogsInBlockRange(ctx, curBlock.String(), nextBlock.String(), logsTo, i.storageClient, i.memoryMu)
	}
	logger.For(ctx).Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())
	return logsTo, nil
}

func (i *indexer) processLogs(ctx context.Context, transfersChan chan<- []transfersAtBlock, logsTo []types.Log) {
	defer close(transfersChan)
	defer recoverAndWait(ctx)
	defer sentryutil.RecoverAndRaise(ctx)

	transfers := logsToTransfers(ctx, logsTo)

	logger.For(ctx).Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

	transfersChan <- transfersToTransfersAtBlock(transfers)
}

func logsToTransfers(ctx context.Context, pLogs []types.Log) []rpc.Transfer {

	result := make([]rpc.Transfer, 0, len(pLogs)*2)
	for _, pLog := range pLogs {

		switch {
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferEventHash)):

			if len(pLog.Topics) < 4 {
				continue
			}

			result = append(result, rpc.Transfer{
				From:            persist.EthereumAddress(pLog.Topics[1].Hex()),
				To:              persist.EthereumAddress(pLog.Topics[2].Hex()),
				TokenID:         persist.TokenID(pLog.Topics[3].Hex()),
				Amount:          1,
				BlockNumber:     persist.BlockNumber(pLog.BlockNumber),
				ContractAddress: persist.EthereumAddress(pLog.Address.Hex()),
				TokenType:       persist.TokenTypeERC721,
				TxHash:          pLog.TxHash,
				BlockHash:       pLog.BlockHash,
				TxIndex:         pLog.TxIndex,
			})

		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferSingleEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}

			eventData := map[string]interface{}{}
			err := erc1155ABI.UnpackIntoMap(eventData, "TransferSingle", pLog.Data)
			if err != nil {
				logger.For(ctx).WithError(err).Error("Failed to unpack TransferSingle event")
				panic(err)
			}

			id, ok := eventData["id"].(*big.Int)
			if !ok {
				panic("Failed to unpack TransferSingle event, id not found")
			}

			value, ok := eventData["value"].(*big.Int)
			if !ok {
				panic("Failed to unpack TransferSingle event, value not found")
			}

			result = append(result, rpc.Transfer{
				From:            persist.EthereumAddress(pLog.Topics[2].Hex()),
				To:              persist.EthereumAddress(pLog.Topics[3].Hex()),
				TokenID:         persist.TokenID(id.Text(16)),
				Amount:          value.Uint64(),
				BlockNumber:     persist.BlockNumber(pLog.BlockNumber),
				ContractAddress: persist.EthereumAddress(pLog.Address.Hex()),
				TokenType:       persist.TokenTypeERC1155,
				TxHash:          pLog.TxHash,
				BlockHash:       pLog.BlockHash,
				TxIndex:         pLog.TxIndex,
			})

		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferBatchEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}

			eventData := map[string]interface{}{}
			err := erc1155ABI.UnpackIntoMap(eventData, "TransferBatch", pLog.Data)
			if err != nil {
				logger.For(ctx).WithError(err).Error("Failed to unpack TransferBatch event")
				panic(err)
			}

			ids, ok := eventData["ids"].([]*big.Int)
			if !ok {
				panic("Failed to unpack TransferBatch event, ids not found")
			}

			values, ok := eventData["values"].([]*big.Int)
			if !ok {
				panic("Failed to unpack TransferBatch event, values not found")
			}

			for j := 0; j < len(ids); j++ {

				result = append(result, rpc.Transfer{
					From:            persist.EthereumAddress(pLog.Topics[2].Hex()),
					To:              persist.EthereumAddress(pLog.Topics[3].Hex()),
					TokenID:         persist.TokenID(ids[j].Text(16)),
					Amount:          values[j].Uint64(),
					ContractAddress: persist.EthereumAddress(pLog.Address.Hex()),
					TokenType:       persist.TokenTypeERC1155,
					BlockNumber:     persist.BlockNumber(pLog.BlockNumber),
					TxHash:          pLog.TxHash,
					BlockHash:       pLog.BlockHash,
					TxIndex:         pLog.TxIndex,
				})
			}

		default:
			logger.For(ctx).WithFields(logrus.Fields{
				"address":   pLog.Address,
				"block":     pLog.BlockNumber,
				"eventType": pLog.Topics[0]},
			).Warn("unknown event")
		}
	}
	return result
}

type tokenIdentifiers struct {
	tokenID         persist.TokenID
	contractAddress persist.EthereumAddress
	ownerAddress    persist.EthereumAddress
	tokenType       persist.TokenType
}

func getTokenIdentifiersFromLog(ctx context.Context, log types.Log) ([]tokenIdentifiers, error) {

	result := make([]tokenIdentifiers, 0, 10)
	switch {
	case strings.EqualFold(log.Topics[0].Hex(), string(transferEventHash)):

		if len(log.Topics) < 4 {
			return nil, fmt.Errorf("invalid log topics length: %d: %+v", len(log.Topics), log)
		}

		ti := tokenIdentifiers{
			tokenID:         persist.TokenID(log.Topics[3].Hex()),
			contractAddress: persist.EthereumAddress(log.Address.Hex()),
			ownerAddress:    persist.EthereumAddress(log.Topics[2].Hex()),
			tokenType:       persist.TokenTypeERC721,
		}

		result = append(result, ti)

	case strings.EqualFold(log.Topics[0].Hex(), string(transferSingleEventHash)):
		if len(log.Topics) < 4 {
			return nil, fmt.Errorf("invalid log topics length: %d: %+v", len(log.Topics), log)
		}

		eventData := map[string]interface{}{}
		err := erc1155ABI.UnpackIntoMap(eventData, "TransferSingle", log.Data)
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to unpack TransferSingle event")
			panic(err)
		}

		id, ok := eventData["id"].(*big.Int)
		if !ok {
			panic("Failed to unpack TransferSingle event, id not found")
		}
		ti := tokenIdentifiers{
			tokenID:         persist.TokenID(id.Text(16)),
			contractAddress: persist.EthereumAddress(log.Address.Hex()),
			ownerAddress:    persist.EthereumAddress(log.Topics[3].Hex()),
			tokenType:       persist.TokenTypeERC1155,
		}

		result = append(result, ti)

	case strings.EqualFold(log.Topics[0].Hex(), string(transferBatchEventHash)):
		if len(log.Topics) < 4 {
			return nil, fmt.Errorf("invalid log topics length: %d: %+v", len(log.Topics), log)
		}

		eventData := map[string]interface{}{}
		err := erc1155ABI.UnpackIntoMap(eventData, "TransferBatch", log.Data)
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to unpack TransferBatch event")
			panic(err)
		}

		ids, ok := eventData["ids"].([]*big.Int)
		if !ok {
			panic("Failed to unpack TransferBatch event, ids not found")
		}

		for j := 0; j < len(ids); j++ {

			ti := tokenIdentifiers{
				tokenID:         persist.TokenID(ids[j].Text(16)),
				contractAddress: persist.EthereumAddress(log.Address.Hex()),
				ownerAddress:    persist.EthereumAddress(log.Topics[3].Hex()),
				tokenType:       persist.TokenTypeERC1155,
			}

			result = append(result, ti)
		}
	default:
		logger.For(ctx).WithFields(logrus.Fields{
			"address":   log.Address,
			"block":     log.BlockNumber,
			"eventType": log.Topics[0]},
		).Warn("unknown event")
	}

	return result, nil

}

func (i *indexer) pollNewLogs(ctx context.Context, transfersChan chan<- []transfersAtBlock, topics [][]common.Hash) {
	span, ctx := tracing.StartSpan(ctx, "indexer.logs", "pollLogs")
	defer tracing.FinishSpan(span)
	defer close(transfersChan)
	defer recoverAndWait(ctx)
	defer sentryutil.RecoverAndRaise(ctx)

	mostRecentBlock, err := rpc.RetryGetBlockNumber(ctx, i.ethClient)
	if err != nil {
		panic(err)
	}

	logger.For(ctx).Infof("Subscribing to new logs from block %d starting with block %d", mostRecentBlock, i.lastSyncedChunk)

	wp := workerpool.New(10)
	// starting at the last chunk that we synced, poll for logs in chunks of blocksPerLogsCall
	for j := i.lastSyncedChunk; j+blocksPerLogsCall <= mostRecentBlock; j += blocksPerLogsCall {
		curBlock := j
		wp.Submit(func() {
			ctx := sentryutil.NewSentryHubContext(ctx)
			defer sentryutil.RecoverAndRaise(ctx)

			nextBlock := curBlock + blocksPerLogsCall

			rpcCtx, cancel := context.WithTimeout(ctx, time.Second*30)
			defer cancel()

			logsTo, err := rpc.RetryGetLogs(rpcCtx, i.ethClient, ethereum.FilterQuery{
				FromBlock: persist.BlockNumber(curBlock).BigInt(),
				ToBlock:   persist.BlockNumber(nextBlock).BigInt(),
				Topics:    topics,
			})
			if err != nil {
				errData := map[string]interface{}{
					"from": curBlock,
					"to":   nextBlock,
					"err":  err.Error(),
				}
				logger.For(ctx).WithError(err).Error(errData)
				return
			}

			go saveLogsInBlockRange(ctx, strconv.Itoa(int(curBlock)), strconv.Itoa(int(nextBlock)), logsTo, i.storageClient, i.memoryMu)

			logger.For(ctx).Infof("Found %d logs at block %d", len(logsTo), curBlock)

			transfers := logsToTransfers(ctx, logsTo)

			logger.For(ctx).Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

			logger.For(ctx).Debugf("Sending %d total transfers to transfers channel", len(transfers))
			transfersChan <- transfersToTransfersAtBlock(transfers)

		})
	}

	logger.For(ctx).Infof("Processed logs from %d to %d.", i.lastSyncedChunk, mostRecentBlock)

	i.updateLastSynced(mostRecentBlock - (mostRecentBlock % blocksPerLogsCall))

}

// TRANSFERS FUNCS -------------------------------------------------------------

func (i *indexer) processAllTransfers(ctx context.Context, incomingTransfers <-chan []transfersAtBlock, plugins []chan<- TransferPluginMsg) {
	span, ctx := tracing.StartSpan(ctx, "indexer.transfers", "processTransfers")
	defer tracing.FinishSpan(span)
	defer sentryutil.RecoverAndRaise(ctx)
	for _, plugin := range plugins {
		defer close(plugin)
	}

	wp := workerpool.New(5)

	logger.For(ctx).Info("Starting to process transfers...")
	for transfers := range incomingTransfers {
		if transfers == nil || len(transfers) == 0 {
			continue
		}

		submit := transfers
		wp.Submit(func() {
			ctx := sentryutil.NewSentryHubContext(ctx)
			timeStart := time.Now()
			logger.For(ctx).Infof("Processing %d transfers", len(submit))
			i.processTransfers(ctx, submit, plugins)
			logger.For(ctx).Infof("Processed %d transfers in %s", len(submit), time.Since(timeStart))
		})
	}
	logger.For(ctx).Info("Waiting for transfers to finish...")
	wp.StopWait()
	logger.For(ctx).Info("Closing field channels...")
}

func (i *indexer) processTransfers(ctx context.Context, transfers []transfersAtBlock, plugins []chan<- TransferPluginMsg) {

	for _, transferAtBlock := range transfers {
		for _, transfer := range transferAtBlock.transfers {

			contractAddress := persist.EthereumAddress(transfer.ContractAddress.String())

			tokenID := transfer.TokenID

			key := persist.NewEthereumTokenIdentifiers(contractAddress, tokenID)

			RunTransferPlugins(ctx, transfer, key, plugins)

		}

	}

}

// TOKENS FUNCS ---------------------------------------------------------------

func (i *indexer) processTokens(ctx context.Context, contractsOut <-chan contractAtBlock) {

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}

	contractsMap := make(map[persist.EthereumTokenIdentifiers]contractAtBlock)

	RunTransferPluginReceiver(ctx, wg, mu, contractsPluginReceiver, contractsOut, contractsMap)

	wg.Wait()

	contracts := contractsAtBlockToContracts(contractsMap)

	i.runDBHooks(ctx, contracts, []persist.Token{})
}

func contractsAtBlockToContracts(contractsAtBlock map[persist.EthereumTokenIdentifiers]contractAtBlock) []persist.Contract {
	contracts := make([]persist.Contract, 0, len(contractsAtBlock))
	seen := make(map[persist.EthereumAddress]bool)
	for _, contract := range contractsAtBlock {
		if seen[contract.contract.Address] {
			continue
		}
		contracts = append(contracts, contract.contract)
		seen[contract.contract.Address] = true
	}
	return contracts
}

func (i *indexer) runDBHooks(ctx context.Context, contracts []persist.Contract, tokens []persist.Token) {
	defer recoverAndWait(ctx)

	wp := workerpool.New(10)

	for _, hook := range i.contractDBHooks {
		hook := hook
		wp.Submit(func() {
			hook(ctx, contracts)
		})
	}

	for _, hook := range i.tokenDBHooks {
		hook := hook
		wp.Submit(func() {
			hook(ctx, tokens)
		})
	}

	wp.StopWait()
}

func contractsPluginReceiver(cur contractAtBlock, inc contractAtBlock) contractAtBlock {
	return inc
}

type alchemyContractMetadata struct {
	Address          persist.EthereumAddress  `json:"address"`
	Metadata         alchemy.ContractMetadata `json:"contractMetadata"`
	ContractDeployer persist.EthereumAddress  `json:"contractDeployer"`
}

func fillContractFields(ctx context.Context, contracts []persist.Contract, contractRepo persist.ContractRepository, httpClient *http.Client, ethClient *ethclient.Client, contractOwnerStats *sync.Map, upChan chan<- []persist.Contract) {
	defer close(upChan)

	contractsNotInDB := make(chan persist.Contract)

	batched := make(chan []persist.Contract)

	go func() {
		defer close(contractsNotInDB)

		iter.ForEach(contracts, func(c *persist.Contract) {
			_, err := contractRepo.GetByAddress(ctx, c.Address)
			if err == nil {
				return
			}
			contractsNotInDB <- *c
		})
	}()

	go func() {
		defer close(batched)
		var curBatch []persist.Contract
		for contract := range contractsNotInDB {
			curBatch = append(curBatch, contract)
			if len(curBatch) == 100 {
				logger.For(ctx).Infof("Batching %d contracts for metadata", len(curBatch))
				batched <- curBatch
				curBatch = []persist.Contract{}
			}
		}
		if len(curBatch) > 0 {
			logger.For(ctx).Infof("Batching %d contracts for metadata", len(curBatch))
			batched <- curBatch
		}
	}()

	// process contracts in batches of 100
	for batch := range batched {
		toUp := make([]persist.Contract, 0, 100)

		cToAddr := make(map[string]persist.Contract)
		for _, c := range batch {
			cToAddr[c.Address.String()] = c
		}

		addresses := util.MapKeys(cToAddr)

		// get contract metadata

		u := fmt.Sprintf("%s/getContractMetadataBatch", env.GetString("ALCHEMY_API_URL"))

		in := map[string][]string{"contractAddresses": addresses}
		inAsJSON, err := json.Marshal(in)
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to marshal contract metadata request")
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewBuffer(inAsJSON))
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to create contract metadata request")
			continue
		}

		req.Header.Add("accept", "application/json")
		req.Header.Add("content-type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to execute contract metadata request")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyAsBytes, _ := ioutil.ReadAll(resp.Body)
			logger.For(ctx).Errorf("Failed to execute contract metadata request: %s status: %s (url: %s) (input: %s) ", string(bodyAsBytes), resp.Status, u, string(inAsJSON))
			continue
		}

		var out []alchemyContractMetadata
		err = json.NewDecoder(resp.Body).Decode(&out)
		if err != nil {
			logger.For(ctx).WithError(err).Error("Failed to decode contract metadata response")
			continue
		}

		for _, c := range out {
			contract := cToAddr[c.Address.String()]
			contract.Name = persist.NullString(c.Metadata.Name)
			contract.Symbol = persist.NullString(c.Metadata.Symbol)

			var method = contractOwnerMethodAlchemy
			cOwner, err := rpc.GetContractOwner(ctx, c.Address, ethClient)
			if err != nil {
				logger.For(ctx).WithError(err).WithFields(logrus.Fields{
					"contractAddress": c.Address,
				}).Error("error getting contract owner")
				contract.OwnerAddress = c.ContractDeployer
			} else {
				contract.OwnerAddress = cOwner
				method = contractOwnerMethodOwnable
			}

			if contract.OwnerAddress == "" {
				method = contractOwnerMethodFailed
			}

			contract.CreatorAddress = c.ContractDeployer

			it, ok := contractOwnerStats.LoadOrStore(method, 1)
			if ok {
				total := it.(int)
				total++
				contractOwnerStats.Store(method, total)
			}

			toUp = append(toUp, contract)
		}

		logger.For(ctx).Infof("Fetched metadata for %d contracts", len(toUp))

		upChan <- toUp
	}

	logger.For(ctx).Infof("Fetched metadata for total %d contracts", len(contracts))

}

// HELPER FUNCS ---------------------------------------------------------------

func transfersToTransfersAtBlock(transfers []rpc.Transfer) []transfersAtBlock {
	transfersMap := map[persist.BlockNumber]transfersAtBlock{}

	for _, transfer := range transfers {
		if tab, ok := transfersMap[transfer.BlockNumber]; !ok {
			transfers := make([]rpc.Transfer, 0, 10)
			transfers = append(transfers, transfer)
			transfersMap[transfer.BlockNumber] = transfersAtBlock{
				block:     transfer.BlockNumber,
				transfers: transfers,
			}
		} else {
			tab.transfers = append(tab.transfers, transfer)
			transfersMap[transfer.BlockNumber] = tab
		}
	}

	allTransfersAtBlock := make([]transfersAtBlock, len(transfersMap))
	i := 0
	for _, transfersAtBlock := range transfersMap {
		allTransfersAtBlock[i] = transfersAtBlock
		i++
	}
	sort.Slice(allTransfersAtBlock, func(i, j int) bool {
		return allTransfersAtBlock[i].block < allTransfersAtBlock[j].block
	})
	return allTransfersAtBlock
}

func saveLogsInBlockRange(ctx context.Context, curBlock, nextBlock string, logsTo []types.Log, storageClient *storage.Client, memoryMu *sync.Mutex) {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	logger.For(ctx).Infof("Saving logs in block range %s to %s", curBlock, nextBlock)
	obj := storageClient.Bucket(env.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock, nextBlock))
	obj.Delete(ctx)
	storageWriter := obj.NewWriter(ctx)

	if err := json.NewEncoder(storageWriter).Encode(logsTo); err != nil {
		panic(err)
	}
	if err := storageWriter.Close(); err != nil {
		panic(err)
	}
}

func recoverAndWait(ctx context.Context) {
	if err := recover(); err != nil {
		logger.For(ctx).Errorf("Error in indexer: %v", err)
		time.Sleep(time.Second * 10)
	}
}

func logEthCallRPCError(entry *logrus.Entry, err error, message string) {
	if rpcErr, ok := err.(gethrpc.Error); ok {
		entry = entry.WithFields(logrus.Fields{"rpcErrorCode": strconv.Itoa(rpcErr.ErrorCode())})
		// If the contract is missing a method then we only want to Warn rather than Error on it.
		if rpcErr.ErrorCode() == -32000 && rpcErr.Error() == "execution reverted" {
			entry.Warn(message)
		}
	} else {
		entry.Error(message)
	}
}

func eventsToTopics(hashes []eventHash) [][]common.Hash {
	events := make([]common.Hash, len(hashes))
	for i, event := range hashes {
		events[i] = common.HexToHash(string(event))
	}
	return [][]common.Hash{events}
}
