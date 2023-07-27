package indexer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gcptasks "cloud.google.com/go/cloudtasks/apiv2"
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
	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/db/gen/indexerdb"
	"github.com/mikeydub/go-gallery/env"
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

	defaultWorkerPoolSize     = 3
	defaultWorkerPoolWaitSize = 10
	blocksPerLogsCall         = 50
)

var (
	rpcEnabled            bool = false // Enables external RPC calls
	erc1155ABI, _              = contracts.IERC1155MetaData.GetAbi()
	defaultTransferEvents      = []eventHash{
		transferBatchEventHash,
		transferEventHash,
		transferSingleEventHash,
	}
)

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

type tokenAtBlock struct {
	ti    persist.EthereumTokenIdentifiers
	boi   blockchainOrderInfo
	token persist.Token
}

func (o tokenAtBlock) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return o.ti
}

func (o tokenAtBlock) OrderInfo() blockchainOrderInfo {
	return o.boi
}

type getLogsFunc func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error)

// indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type indexer struct {
	ethClient     *ethclient.Client
	httpClient    *http.Client
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	storageClient *storage.Client
	queries       *indexerdb.Queries
	tokenRepo     persist.TokenRepository
	contractRepo  persist.ContractRepository
	dbMu          *sync.Mutex // Manages writes to the db
	stateMu       *sync.Mutex // Manages updates to the indexer's state
	memoryMu      *sync.Mutex // Manages large memory operations

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
func newIndexer(ethClient *ethclient.Client, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, iQueries *indexerdb.Queries, bQueries *coredb.Queries, taskClient *gcptasks.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, pChain persist.Chain, pEvents []eventHash, getLogsFunc getLogsFunc, startingBlock, maxBlock *uint64) *indexer {
	if rpcEnabled && ethClient == nil {
		panic("RPC is enabled but an ethClient wasn't provided!")
	}

	ownerStats := &sync.Map{}

	i := &indexer{
		ethClient:     ethClient,
		httpClient:    httpClient,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		storageClient: storageClient,
		tokenRepo:     tokenRepo,
		contractRepo:  contractRepo,
		queries:       iQueries,
		dbMu:          &sync.Mutex{},
		stateMu:       &sync.Mutex{},
		memoryMu:      &sync.Mutex{},

		tokenBucket: env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"),

		chain: pChain,

		maxBlock: maxBlock,

		contractOwnerStats: ownerStats,

		eventHashes: pEvents,

		getLogsFunc: getLogsFunc,

		contractDBHooks: newContractHooks(iQueries, contractRepo, ethClient, httpClient, ownerStats),
		tokenDBHooks:    newTokenHooks(taskClient, bQueries),

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
	enabledPlugins := []chan<- TransferPluginMsg{plugins.contracts.in, plugins.tokens.in}

	statsID, err := i.queries.InsertStatistic(ctx, indexerdb.InsertStatisticParams{ID: persist.GenerateID(), BlockStart: start, BlockEnd: start + blocksPerLogsCall})
	if err != nil {
		panic(err)
	}

	go func() {
		ctx := sentryutil.NewSentryHubContext(ctx)
		span, ctx := tracing.StartSpan(ctx, "indexer.logs", "processLogs")
		defer tracing.FinishSpan(span)

		logs := i.fetchLogs(ctx, start, topics, statsID)
		i.processLogs(ctx, transfers, logs)
	}()
	go i.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transfers, enabledPlugins, statsID)
	i.processTokens(ctx, plugins.tokens.out, plugins.contracts.out, statsID)

	err = i.queries.UpdateStatisticSuccess(ctx, indexerdb.UpdateStatisticSuccessParams{ID: statsID, Success: true, ProcessingTimeSeconds: sql.NullInt64{Int64: int64(time.Since(startTime) / time.Second), Valid: true}})
	if err != nil {
		panic(err)
	}

	logger.For(ctx).Warnf("Finished processing %d blocks from block %d in %s", blocksPerLogsCall, start.Uint64(), time.Since(startTime))
}

func (i *indexer) startNewBlocksPipeline(ctx context.Context, topics [][]common.Hash) {
	span, ctx := tracing.StartSpan(ctx, "indexer.pipeline", "polling", sentry.TransactionName("indexer-main:polling"))
	defer tracing.FinishSpan(span)

	transfers := make(chan []transfersAtBlock)
	plugins := NewTransferPlugins(ctx)
	enabledPlugins := []chan<- TransferPluginMsg{plugins.contracts.in, plugins.tokens.in}

	mostRecentBlock, err := rpc.RetryGetBlockNumber(ctx, i.ethClient)
	if err != nil {
		panic(err)
	}

	if i.lastSyncedChunk+blocksPerLogsCall > mostRecentBlock {
		logger.For(ctx).Infof("No new blocks to process. Last synced block: %d, most recent block: %d", i.lastSyncedChunk, mostRecentBlock)
		return
	}

	statsID, err := i.queries.InsertStatistic(ctx, indexerdb.InsertStatisticParams{ID: persist.GenerateID(), BlockStart: persist.BlockNumber(i.lastSyncedChunk), BlockEnd: persist.BlockNumber(mostRecentBlock - (i.mostRecentBlock % blocksPerLogsCall))})
	if err != nil {
		panic(err)
	}

	go i.pollNewLogs(sentryutil.NewSentryHubContext(ctx), transfers, topics, mostRecentBlock, statsID)
	go i.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transfers, enabledPlugins, statsID)
	i.processTokens(ctx, plugins.tokens.out, plugins.contracts.out, statsID)

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

func (i *indexer) fetchLogs(ctx context.Context, startingBlock persist.BlockNumber, topics [][]common.Hash, statsID persist.DBID) []types.Log {
	curBlock := startingBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(blocksPerLogsCall)))

	logger.For(ctx).Infof("Getting logs from %d to %d", curBlock, nextBlock)

	logsTo, err := i.getLogsFunc(ctx, curBlock, nextBlock, topics)
	if err != nil {
		panic(fmt.Sprintf("error getting logs: %s", err))
	}

	err = i.queries.UpdateStatisticTotalLogs(ctx, indexerdb.UpdateStatisticTotalLogsParams{
		ID:        statsID,
		TotalLogs: sql.NullInt64{Int64: int64(len(logsTo)), Valid: true},
	})
	if err != nil {
		panic(err)
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

func (i *indexer) pollNewLogs(ctx context.Context, transfersChan chan<- []transfersAtBlock, topics [][]common.Hash, mostRecentBlock uint64, statsID persist.DBID) {
	span, ctx := tracing.StartSpan(ctx, "indexer.logs", "pollLogs")
	defer tracing.FinishSpan(span)
	defer close(transfersChan)
	defer recoverAndWait(ctx)
	defer sentryutil.RecoverAndRaise(ctx)

	logger.For(ctx).Infof("Subscribing to new logs from block %d starting with block %d", mostRecentBlock, i.lastSyncedChunk)

	totalLogs := &atomic.Int64{}

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

			totalLogs.Add(int64(len(logsTo)))

			go saveLogsInBlockRange(ctx, strconv.Itoa(int(curBlock)), strconv.Itoa(int(nextBlock)), logsTo, i.storageClient, i.memoryMu)

			logger.For(ctx).Infof("Found %d logs at block %d", len(logsTo), curBlock)

			transfers := logsToTransfers(ctx, logsTo)

			logger.For(ctx).Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

			logger.For(ctx).Debugf("Sending %d total transfers to transfers channel", len(transfers))
			transfersChan <- transfersToTransfersAtBlock(transfers)

		})
	}

	wp.StopWait()

	total := totalLogs.Load()

	err := i.queries.UpdateStatisticTotalLogs(ctx, indexerdb.UpdateStatisticTotalLogsParams{
		TotalLogs: sql.NullInt64{Int64: total, Valid: true},
		ID:        statsID,
	})
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to update total logs")
		panic(err)
	}

	logger.For(ctx).Infof("Processed %d logs from %d to %d.", total, i.lastSyncedChunk, mostRecentBlock)

	i.updateLastSynced(mostRecentBlock - (mostRecentBlock % blocksPerLogsCall))

}

// TRANSFERS FUNCS -------------------------------------------------------------

func (i *indexer) processAllTransfers(ctx context.Context, incomingTransfers <-chan []transfersAtBlock, plugins []chan<- TransferPluginMsg, statsID persist.DBID) {
	span, ctx := tracing.StartSpan(ctx, "indexer.transfers", "processTransfers")
	defer tracing.FinishSpan(span)
	defer sentryutil.RecoverAndRaise(ctx)
	for _, plugin := range plugins {
		defer close(plugin)
	}

	wp := workerpool.New(5)

	logger.For(ctx).Info("Starting to process transfers...")
	var totatTransfers int64
	for transfers := range incomingTransfers {
		if len(transfers) == 0 {
			continue
		}

		totatTransfers += int64(len(transfers))

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

	err := i.queries.UpdateStatisticTotalTransfers(ctx, indexerdb.UpdateStatisticTotalTransfersParams{
		TotalTransfers: sql.NullInt64{Int64: totatTransfers, Valid: true},
		ID:             statsID,
	})
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to update total transfers")
		panic(err)
	}

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

func (i *indexer) processTokens(ctx context.Context, tokensOut <-chan tokenAtBlock, contractsOut <-chan contractAtBlock, statsID persist.DBID) {

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}

	contractsMap := make(map[persist.EthereumTokenIdentifiers]contractAtBlock)
	tokensMap := make(map[persist.EthereumTokenIdentifiers]tokenAtBlock)

	RunTransferPluginReceiver(ctx, wg, mu, contractsPluginReceiver, contractsOut, contractsMap)
	RunTransferPluginReceiver(ctx, wg, mu, tokensPluginReceiver, tokensOut, tokensMap)

	wg.Wait()

	contracts := contractsAtBlockToContracts(contractsMap)
	tokens := tokensAtBlockToTokens(tokensMap)

	i.queries.UpdateStatisticTotalTokensAndContracts(ctx, indexerdb.UpdateStatisticTotalTokensAndContractsParams{
		TotalContracts: sql.NullInt64{Int64: int64(len(contracts)), Valid: true},
		ID:             statsID,
	})

	i.runDBHooks(ctx, contracts, tokens, statsID)
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

func tokensAtBlockToTokens(tokensAtBlock map[persist.EthereumTokenIdentifiers]tokenAtBlock) []persist.Token {
	tokens := make([]persist.Token, 0, len(tokensAtBlock))
	seen := make(map[persist.TokenUniqueIdentifiers]bool)
	for _, token := range tokensAtBlock {
		tids := persist.TokenUniqueIdentifiers{
			Chain:           token.token.Chain,
			ContractAddress: persist.Address(token.token.ContractAddress),
			TokenID:         token.token.TokenID,
			OwnerAddress:    persist.Address(token.token.OwnerAddress),
		}
		if seen[tids] {
			continue
		}
		tokens = append(tokens, token.token)
		seen[tids] = true
	}
	return tokens
}

func (i *indexer) runDBHooks(ctx context.Context, contracts []persist.Contract, tokens []persist.Token, statsID persist.DBID) {
	defer recoverAndWait(ctx)

	wp := workerpool.New(10)

	for _, hook := range i.contractDBHooks {
		hook := hook
		wp.Submit(func() {
			err := hook(ctx, contracts, statsID)
			if err != nil {
				logger.For(ctx).WithError(err).Errorf("Failed to run contract db hook %s", err)
			}
		})
	}

	for _, hook := range i.tokenDBHooks {
		hook := hook
		wp.Submit(func() {
			err := hook(ctx, tokens, statsID)
			if err != nil {
				logger.For(ctx).WithError(err).Errorf("Failed to run token db hook %s", err)
			}
		})
	}

	wp.StopWait()
}

func contractsPluginReceiver(cur contractAtBlock, inc contractAtBlock) contractAtBlock {
	return inc
}

func tokensPluginReceiver(cur tokenAtBlock, inc tokenAtBlock) tokenAtBlock {
	if cur.token.TokenType == persist.TokenTypeERC1155 {
		inc.token.Quantity = cur.token.Quantity.Add(inc.token.Quantity)
	}
	return inc
}

type AlchemyContract struct {
	Address          persist.EthereumAddress `json:"address"`
	ContractMetadata AlchemyContractMetadata `json:"contractMetadata"`
}

type AlchemyContractMetadata struct {
	Address          persist.EthereumAddress  `json:"address"`
	Metadata         alchemy.ContractMetadata `json:"contractMetadata"`
	ContractDeployer persist.EthereumAddress  `json:"contractDeployer"`
}

func fillContractFields(ctx context.Context, contracts []persist.Contract, queries *indexerdb.Queries, contractRepo persist.ContractRepository, httpClient *http.Client, ethClient *ethclient.Client, contractOwnerStats *sync.Map, upChan chan<- []persist.Contract, statsID persist.DBID) {
	defer close(upChan)

	innerPipelineStats := map[persist.ContractOwnerMethod]any{}

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
		// get contract metadata
		toUp, _ := GetContractMetadatas(ctx, batch, httpClient, ethClient)

		for _, c := range toUp {
			it, ok := contractOwnerStats.LoadOrStore(c.OwnerMethod, 1)
			if ok {
				total := it.(int)
				total++
				contractOwnerStats.Store(c.OwnerMethod, total)
			}

			it, ok = innerPipelineStats[c.OwnerMethod]
			if ok {
				total := it.(int)
				total++
				innerPipelineStats[c.OwnerMethod] = total
			} else {
				innerPipelineStats[c.OwnerMethod] = 1
			}
		}

		logger.For(ctx).Infof("Fetched metadata for %d contracts", len(toUp))

		asContracts, _ := util.Map(toUp, func(c ContractOwnerResult) (persist.Contract, error) {
			return c.Contract, nil
		})
		upChan <- asContracts
	}

	marshalled, err := json.Marshal(innerPipelineStats)
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to marshal inner pipeline stats")
		panic(err)
	}

	err = queries.UpdateStatisticContractStats(ctx, indexerdb.UpdateStatisticContractStatsParams{
		ContractStats: pgtype.JSONB{Bytes: marshalled, Status: pgtype.Present},
		ID:            statsID,
	})
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to update contract stats")
		panic(err)
	}

	logger.For(ctx).Infof("Fetched metadata for total %d contracts", len(contracts))

}

type ContractOwnerResult struct {
	Contract    persist.Contract            `json:"contract"`
	OwnerMethod persist.ContractOwnerMethod `json:"ownerMethod"`
}

func GetContractMetadatas(ctx context.Context, batch []persist.Contract, httpClient *http.Client, ethClient *ethclient.Client) ([]ContractOwnerResult, error) {
	toUp := make([]ContractOwnerResult, 0, 100)

	cToAddr := make(map[string]persist.Contract)
	for _, c := range batch {
		cToAddr[c.Address.String()] = c
	}

	addresses := util.MapKeys(cToAddr)

	u := fmt.Sprintf("%s/getContractMetadataBatch", env.GetString("ALCHEMY_API_URL"))

	in := map[string][]string{"contractAddresses": addresses}
	inAsJSON, err := json.Marshal(in)
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to marshal contract metadata request")
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewBuffer(inAsJSON))
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to create contract metadata request")
		return nil, err
	}

	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to execute contract metadata request")
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		bodyAsBytes, _ := io.ReadAll(resp.Body)
		logger.For(ctx).Errorf("Failed to execute contract metadata request: %s status: %s (url: %s) (input: %s) ", string(bodyAsBytes), resp.Status, u, string(inAsJSON))
		return nil, err
	}

	var out []AlchemyContract
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		logger.For(ctx).WithError(err).Error("Failed to decode contract metadata response")
		return nil, err
	}

	for _, c := range out {

		result := ContractOwnerResult{}
		contract := cToAddr[c.Address.String()]
		contract.Name = persist.NullString(c.ContractMetadata.Metadata.Name)
		contract.Symbol = persist.NullString(c.ContractMetadata.Metadata.Symbol)

		var method = persist.ContractOwnerMethodAlchemy
		cOwner, err := rpc.GetContractOwner(ctx, c.Address, ethClient)
		if err != nil {
			logger.For(ctx).WithError(err).WithFields(logrus.Fields{
				"contractAddress": c.Address,
			}).Error("error getting contract owner")
			contract.OwnerAddress = c.ContractMetadata.ContractDeployer
		} else {
			contract.OwnerAddress = cOwner
			method = persist.ContractOwnerMethodOwnable
		}

		if contract.OwnerAddress == "" {
			method = persist.ContractOwnerMethodFailed
		}

		contract.CreatorAddress = c.ContractMetadata.ContractDeployer

		result.OwnerMethod = method
		result.Contract = contract

		toUp = append(toUp, result)
	}
	return toUp, nil
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

func eventsToTopics(hashes []eventHash) [][]common.Hash {
	events := make([]common.Hash, len(hashes))
	for i, event := range hashes {
		events[i] = common.HexToHash(string(event))
	}
	return [][]common.Hash{events}
}
