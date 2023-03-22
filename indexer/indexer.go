package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
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
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

func init() {
	env.RegisterValidation("GCLOUD_TOKEN_CONTENT_BUCKET", "required")
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

type tokenMetadata struct {
	ti persist.EthereumTokenIdentifiers
	md persist.TokenMetadata
}

type tokenBalances struct {
	ti      persist.EthereumTokenIdentifiers
	boi     blockchainOrderInfo
	from    persist.EthereumAddress
	to      persist.EthereumAddress
	fromAmt *big.Int
	toAmt   *big.Int
}

func (t tokenBalances) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return t.ti
}

func (t tokenBalances) OrderInfo() blockchainOrderInfo {
	return t.boi
}

type tokenURI struct {
	boi blockchainOrderInfo
	ti  persist.EthereumTokenIdentifiers
	uri persist.TokenURI
}

func (t tokenURI) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return t.ti
}

func (t tokenURI) OrderInfo() blockchainOrderInfo {
	return t.boi
}

type tokenBalancesAtBlock struct {
	ti       persist.EthereumTokenIdentifiers
	boi      blockchainOrderInfo
	balances map[persist.EthereumAddress]balanceAtBlock
}

func (t tokenBalancesAtBlock) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return t.ti
}

func (t tokenBalancesAtBlock) OrderInfo() blockchainOrderInfo {
	return t.boi
}

type transfersAtBlock struct {
	block     persist.BlockNumber
	transfers []rpc.Transfer
}

type ownerAtBlock struct {
	ti    persist.EthereumTokenIdentifiers
	boi   blockchainOrderInfo
	owner persist.EthereumAddress
}

func (o ownerAtBlock) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return o.ti
}

func (o ownerAtBlock) OrderInfo() blockchainOrderInfo {
	return o.boi
}

type previousOwnersAtBlock struct {
	owners []ownerAtBlock
	ti     persist.EthereumTokenIdentifiers
	boi    blockchainOrderInfo
}

func (p previousOwnersAtBlock) TokenIdentifiers() persist.EthereumTokenIdentifiers {
	return p.ti
}

func (p previousOwnersAtBlock) OrderInfo() blockchainOrderInfo {
	return p.boi
}

type balanceAtBlock struct {
	ti    persist.EthereumTokenIdentifiers
	block persist.BlockNumber
	amnt  *big.Int
}

type tokenMedia struct {
	ti    persist.EthereumTokenIdentifiers
	media persist.Media
}

type getLogsFunc func(ctx context.Context, curBlock, nextBlock *big.Int, topics [][]common.Hash) ([]types.Log, error)

// indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type indexer struct {
	ethClient         *ethclient.Client
	ipfsClient        *shell.Shell
	arweaveClient     *goar.Client
	storageClient     *storage.Client
	tokenRepo         persist.TokenRepository
	contractRepo      persist.ContractRepository
	addressFilterRepo refresh.AddressFilterRepository
	dbMu              *sync.Mutex // Manages writes to the db
	stateMu           *sync.Mutex // Manages updates to the indexer's state

	tokenBucket string

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64  // Current height of the blockchain
	lastSyncedChunk uint64  // Start block of the last chunk handled by the indexer
	maxBlock        *uint64 // If provided, the indexer will only index up to maxBlock

	isListening bool // Indicates if the indexer is waiting for new blocks

	getLogsFunc getLogsFunc
}

// newIndexer sets up an indexer for retrieving the specified events that will process tokens
func newIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, addressFilterRepo refresh.AddressFilterRepository, pChain persist.Chain, pEvents []eventHash, getLogsFunc getLogsFunc, startingBlock, maxBlock *uint64) *indexer {
	if rpcEnabled && ethClient == nil {
		panic("RPC is enabled but an ethClient wasn't provided!")
	}
	i := &indexer{
		ethClient:         ethClient,
		ipfsClient:        ipfsClient,
		arweaveClient:     arweaveClient,
		storageClient:     storageClient,
		tokenRepo:         tokenRepo,
		contractRepo:      contractRepo,
		addressFilterRepo: addressFilterRepo,
		dbMu:              &sync.Mutex{},
		stateMu:           &sync.Mutex{},

		tokenBucket: env.GetString(context.Background(), "GCLOUD_TOKEN_CONTENT_BUCKET"),

		chain: pChain,

		maxBlock: maxBlock,

		eventHashes: pEvents,

		getLogsFunc: getLogsFunc,
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
	plugins := NewTransferPlugins(ctx, i.ethClient, i.tokenRepo, i.addressFilterRepo)
	enabledPlugins := []chan<- PluginMsg{plugins.balances.in, plugins.owners.in, plugins.uris.in, plugins.refresh.in, plugins.previousOwners.in}

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
	i.processTokens(ctx, plugins.uris.out, plugins.owners.out, plugins.previousOwners.out, plugins.balances.out, plugins.refresh.out)

	check := <-logsToCheckAgainst
	i.checkTokensExistForLogs(ctx, check)
	logger.For(ctx).Warnf("Finished processing %d blocks from block %d in %s", blocksPerLogsCall, start.Uint64(), time.Since(startTime))
}

func (i *indexer) startNewBlocksPipeline(ctx context.Context, topics [][]common.Hash) {
	span, ctx := tracing.StartSpan(ctx, "indexer.pipeline", "polling", sentry.TransactionName("indexer-main:polling"))
	defer tracing.FinishSpan(span)

	transfers := make(chan []transfersAtBlock)
	plugins := NewTransferPlugins(ctx, i.ethClient, i.tokenRepo, i.addressFilterRepo)
	enabledPlugins := []chan<- PluginMsg{plugins.balances.in, plugins.owners.in, plugins.previousOwners.in, plugins.uris.in, plugins.refresh.in}
	logsToCheckAgainst := make(chan []types.Log)
	go i.pollNewLogs(sentryutil.NewSentryHubContext(ctx), transfers, logsToCheckAgainst, topics)
	go i.processAllTransfers(sentryutil.NewSentryHubContext(ctx), transfers, enabledPlugins)
	i.processTokens(ctx, plugins.uris.out, plugins.owners.out, plugins.previousOwners.out, plugins.balances.out, plugins.refresh.out)

	check := <-logsToCheckAgainst
	i.checkTokensExistForLogs(ctx, check)
}

func (i *indexer) checkTokensExistForLogs(ctx context.Context, logs []types.Log) {
	span, ctx := tracing.StartSpan(ctx, "indexer.checkTokensExistForLogs", "checkTokensExistForLogs")
	defer tracing.FinishSpan(span)

	doesNotExist := make(chan types.Log)

	wp := workerpool.New(defaultWorkerPoolSize)

	for _, log := range logs {
		log := log
		wp.Submit(func() {
			ctx := sentryutil.NewSentryHubContext(ctx)
			defer recoverAndWait(ctx)
			defer sentryutil.RecoverAndRaise(ctx)
			tis, err := getTokenIdentifiersFromLog(ctx, log)
			if err != nil {
				logger.For(ctx).Errorf("error getting token identifiers from log: %s", err)
				return
			}
			for _, ti := range tis {
				if ti.tokenType == persist.TokenTypeERC1155 {
					balance, err := rpc.GetBalanceOfERC1155Token(ctx, ti.ownerAddress, ti.contractAddress, ti.tokenID, i.ethClient)
					if err != nil {
						logger.For(ctx).Errorf("error getting balance of ERC1155 token: %s", err)
						return
					}
					if balance.Cmp(big.NewInt(0)) == 0 {
						logger.For(ctx).Debugf("balance of ERC1155 token is 0, skipping check")
						return
					}
				}
				exists, err := i.tokenRepo.TokenExistsByTokenIdentifiers(ctx, ti.tokenID, ti.contractAddress)
				if err != nil {
					logger.For(ctx).Errorf("error checking if token exists: %s", err)
					return
				}
				if !exists {
					doesNotExist <- log
				}
			}
		})
	}

	go func() {
		wp.StopWait()
		close(doesNotExist)
	}()

	for log := range doesNotExist {
		marshalled, err := json.Marshal(log)
		if err != nil {
			panic(err)
		}

		logger.For(ctx).Errorf("token does not exist for log: %s", marshalled)
	}

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
	reader, err := i.storageClient.Bucket(env.GetString(ctx, "GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%d-%d", curBlock, nextBlock)).NewReader(ctx)
	if err != nil {
		logger.For(ctx).WithError(err).Warn("error getting logs from GCP")
	} else {
		defer reader.Close()
		err = json.NewDecoder(reader).Decode(&logsTo)
		if err != nil {
			panic(err)
		}
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
		go saveLogsInBlockRange(ctx, curBlock.String(), nextBlock.String(), logsTo, i.storageClient)
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
		initial := time.Now()
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

			logger.For(ctx).Debugf("Processed transfer event in %s", time.Since(initial))
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
			logger.For(ctx).Debugf("Processed single transfer event in %s", time.Since(initial))
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
			logger.For(ctx).Debugf("Processed batch event in %s", time.Since(initial))
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

func (i *indexer) pollNewLogs(ctx context.Context, transfersChan chan<- []transfersAtBlock, logsToCheckAgainst chan<- []types.Log, topics [][]common.Hash) {
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

	// this chan will take in every log that we get when polling for logs in this pipeline
	allLogsInPoll := make(chan []types.Log)

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

			go saveLogsInBlockRange(ctx, strconv.Itoa(int(curBlock)), strconv.Itoa(int(nextBlock)), logsTo, i.storageClient)

			logger.For(ctx).Infof("Found %d logs at block %d", len(logsTo), curBlock)

			transfers := logsToTransfers(ctx, logsTo)

			logger.For(ctx).Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

			logger.For(ctx).Debugf("Sending %d total transfers to transfers channel", len(transfers))
			transfersChan <- transfersToTransfersAtBlock(transfers)
			allLogsInPoll <- logsTo

		})
	}
	go func() {
		wp.StopWait()
		close(allLogsInPoll)
	}()

	resultLogs := []types.Log{}
	// combine all logs into one slice
	for logs := range allLogsInPoll {
		resultLogs = append(resultLogs, logs...)
	}

	logger.For(ctx).Infof("Processed logs from %d to %d.", i.lastSyncedChunk, mostRecentBlock)

	i.updateLastSynced(mostRecentBlock - (mostRecentBlock % blocksPerLogsCall))

	logsToCheckAgainst <- resultLogs
}

// TRANSFERS FUNCS -------------------------------------------------------------

func (i *indexer) processAllTransfers(ctx context.Context, incomingTransfers <-chan []transfersAtBlock, plugins []chan<- PluginMsg) {
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

func (i *indexer) processTransfers(ctx context.Context, transfers []transfersAtBlock, plugins []chan<- PluginMsg) {

	for _, transferAtBlock := range transfers {
		for _, transfer := range transferAtBlock.transfers {

			initial := time.Now()
			contractAddress := persist.EthereumAddress(transfer.ContractAddress.String())
			from := transfer.From
			to := transfer.To
			tokenID := transfer.TokenID

			key := persist.NewEthereumTokenIdentifiers(contractAddress, tokenID)

			RunPlugins(ctx, transfer, key, plugins)

			logger.For(ctx).WithFields(logrus.Fields{
				"tokenID":         tokenID,
				"contractAddress": contractAddress,
				"fromAddress":     from,
				"toAddress":       to,
				"duration":        time.Since(initial),
			}).Debugf("Processed transfer %s to %s and from %s ", key, to, from)
		}

	}

}

func getBalances(ctx context.Context, contractAddress persist.EthereumAddress, from persist.EthereumAddress, tokenID persist.TokenID, key persist.EthereumTokenIdentifiers, blockNumber persist.BlockNumber, txIndex uint, to persist.EthereumAddress, ethClient *ethclient.Client) (tokenBalances, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	var fromBalance, toBalance *big.Int
	var err error

	if from.String() != persist.ZeroAddress.String() {
		fromBalance, err = rpc.RetryGetBalanceOfERC1155Token(ctx, from, contractAddress, tokenID, ethClient)
		if err != nil {
			return tokenBalances{}, err
		}
	}
	if to.String() != persist.ZeroAddress.String() {
		toBalance, err = rpc.RetryGetBalanceOfERC1155Token(ctx, to, contractAddress, tokenID, ethClient)
		if err != nil {
			return tokenBalances{}, err
		}
	}

	// MaxUint because there is no txIndex, this is simply the most up to date balance on the blockchain so it should always be ahead of any other information at this block
	// CurBlock becuase the RPC functions return the current balance, not the balance of the block being processed
	bal := tokenBalances{key, blockchainOrderInfo{blockNumber: blockNumber, txIndex: txIndex}, from, to, fromBalance, toBalance}
	return bal, nil
}

func getOwner(ctx context.Context, contractAddress persist.EthereumAddress, tokenID persist.TokenID, key persist.EthereumTokenIdentifiers, blockNumber persist.BlockNumber, txIndex uint, ethClient *ethclient.Client) (ownerAtBlock, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	owner, err := rpc.RetryGetOwnerOfERC721Token(ctx, contractAddress, tokenID, ethClient)
	if err != nil {
		return ownerAtBlock{}, err
	}

	// MaxUint because there is no txIndex, this is simply the most up to date balance on the blockchain so it should always be ahead of any other information at this block
	// CurBlock becuase the RPC functions return the current balance, not the balance of the block being processed
	bal := ownerAtBlock{key, blockchainOrderInfo{blockNumber: blockNumber, txIndex: txIndex}, owner}
	return bal, nil
}

func getURI(ctx context.Context, contractAddress persist.EthereumAddress, tokenID persist.TokenID, tokenType persist.TokenType, ethClient *ethclient.Client) persist.TokenURI {
	u, err := rpc.RetryGetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
	if err != nil {
		logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
			"tokenType":       tokenType,
			"tokenID":         tokenID,
			"contractAddress": contractAddress,
			"rpcCall":         "eth_call",
		})
		logEthCallRPCError(logEntry, err, "error getting URI for token")

		if strings.Contains(err.Error(), "execution reverted") {
			u = persist.InvalidTokenURI
		}
	}

	u = u.ReplaceID(tokenID)
	if (len(u.String())) > util.KB {
		logger.For(ctx).Infof("URI size for %s-%s: %s", contractAddress, tokenID, util.InByteSizeFormat(uint64(len(u.String()))))
		if (len(u.String())) > util.KB*100 {
			logger.For(ctx).Errorf("Skipping URI for %s-%s with size: %s", contractAddress, tokenID, util.InByteSizeFormat(uint64(len(u.String()))))
			return ""
		}
	}
	return u
}

// TOKENS FUNCS ---------------------------------------------------------------

func (i *indexer) processTokens(ctx context.Context,
	uris <-chan tokenURI,
	owners <-chan ownerAtBlock,
	previousOwners <-chan ownerAtBlock,
	balances <-chan tokenBalances,
	refreshes <-chan errForTokenAtBlockAndIndex,
) {
	ownersMap := map[persist.EthereumTokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[persist.EthereumTokenIdentifiers]*previousOwnersAtBlock{}
	balancesMap := map[persist.EthereumTokenIdentifiers]*tokenBalancesAtBlock{}
	urisMap := map[persist.EthereumTokenIdentifiers]tokenURI{}

	wg := &sync.WaitGroup{}

	// we won't be storing any results of this plugin
	RunPluginReceiver(ctx, wg, &sync.Mutex{}, refreshesPluginReceiver(ctx), refreshes, map[persist.EthereumTokenIdentifiers]errForTokenAtBlockAndIndex{})

	// run the receivers in parallel and return one result from each channel for a total of totalRunningPlugins (5)
	RunPluginReceiver(ctx, wg, &sync.Mutex{}, urisPluginReceiver, uris, urisMap)
	RunPluginReceiver(ctx, wg, &sync.Mutex{}, balancesPluginReceiver, balances, balancesMap)
	RunPluginReceiver(ctx, wg, &sync.Mutex{}, ownersPluginReceiver, owners, ownersMap)
	RunPluginReceiver(ctx, wg, &sync.Mutex{}, previousOwnersPluginReceiver, previousOwners, previousOwnersMap)

	wg.Wait()

	logger.For(ctx).Info("Done recieving field data, converting fields into tokens...")

	i.createTokens(ctx, ownersMap, previousOwnersMap, balancesMap, urisMap)
}

func (i *indexer) createTokens(ctx context.Context,
	ownersMap map[persist.EthereumTokenIdentifiers]ownerAtBlock,
	previousOwnersMap map[persist.EthereumTokenIdentifiers]*previousOwnersAtBlock,
	balancesMap map[persist.EthereumTokenIdentifiers]*tokenBalancesAtBlock,
	urisMap map[persist.EthereumTokenIdentifiers]tokenURI,
) {
	defer recoverAndWait(ctx)

	tokens := i.fieldMapsToTokens(ctx, ownersMap, previousOwnersMap, balancesMap, urisMap)
	if tokens == nil || len(tokens) == 0 {
		logger.For(ctx).Info("No tokens to process")
		return
	}

	logger.For(ctx).Info("Created tokens to insert into database...")

	timeout := (time.Minute * time.Duration((len(tokens) / 100))) + time.Minute
	logger.For(ctx).Infof("Upserting %d tokens and contracts with a timeout of %s", len(tokens), timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := upsertTokensAndContracts(ctx, tokens, i.tokenRepo, i.contractRepo, i.ethClient, i.dbMu)
	if err != nil {
		panic(fmt.Sprintf("error upserting tokens and contracts: %s", err))
	}

	logger.For(ctx).Info("Done upserting tokens and contracts")
}

func ownersPluginReceiver(cur ownerAtBlock, inc ownerAtBlock) ownerAtBlock {
	return inc
}

func previousOwnersPluginReceiver(cur *previousOwnersAtBlock, inc ownerAtBlock) *previousOwnersAtBlock {
	var curPrev []ownerAtBlock
	if cur == nil {
		cur = &previousOwnersAtBlock{
			owners: []ownerAtBlock{},
			ti:     inc.ti,
			boi:    inc.boi,
		}
	} else if cur.owners == nil {
		cur.owners = []ownerAtBlock{}
	}
	curPrev = cur.owners

	curPrev = append(curPrev, inc)

	curPrev = util.Dedupe(curPrev, true)

	cur.owners = curPrev
	cur.boi = inc.boi

	return cur
}

func balancesPluginReceiver(cur *tokenBalancesAtBlock, balance tokenBalances) *tokenBalancesAtBlock {

	if cur == nil {
		cur = &tokenBalancesAtBlock{
			ti:       balance.ti,
			boi:      balance.boi,
			balances: make(map[persist.EthereumAddress]balanceAtBlock),
		}
	} else if cur.balances == nil {
		cur.balances = make(map[persist.EthereumAddress]balanceAtBlock)
	}
	balanceMap := cur.balances
	toBal := balanceMap[balance.to]
	if toBal.block < balance.boi.blockNumber {
		toBal.block = balance.boi.blockNumber
		toBal.amnt = balance.toAmt
		balanceMap[balance.to] = toBal
	}

	fromBal := balanceMap[balance.from]
	if fromBal.block < balance.boi.blockNumber {
		fromBal.block = balance.boi.blockNumber
		fromBal.amnt = balance.fromAmt
		balanceMap[balance.from] = fromBal
	}

	cur.balances = balanceMap
	cur.boi = balance.boi

	return cur

}

func urisPluginReceiver(cur tokenURI, inc tokenURI) tokenURI {
	return inc
}

func refreshesPluginReceiver(ctx context.Context) PluginReceiver[errForTokenAtBlockAndIndex, errForTokenAtBlockAndIndex] {
	return func(cur errForTokenAtBlockAndIndex, inc errForTokenAtBlockAndIndex) errForTokenAtBlockAndIndex {
		if inc.err != nil {
			logger.For(ctx).WithError(inc.err).Error("failed to save filter")
		}
		return inc
	}
}

func (i *indexer) fieldMapsToTokens(ctx context.Context,
	owners map[persist.EthereumTokenIdentifiers]ownerAtBlock,
	previousOwners map[persist.EthereumTokenIdentifiers]*previousOwnersAtBlock,
	balances map[persist.EthereumTokenIdentifiers]*tokenBalancesAtBlock,
	uris map[persist.EthereumTokenIdentifiers]tokenURI,
) []persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v.balances)
	}
	result := make([]persist.Token, 0, len(owners)+totalBalances)

	for k, v := range owners {
		contractAddress, tokenID, err := k.GetParts()
		if err != nil {
			logger.For(ctx).WithError(err).Errorf("error getting parts from %s: - %s | val: %+v", k, err, v)
			continue
		}

		previousOwnerAddresses := make([]persist.EthereumAddressAtBlock, len(previousOwners[k].owners))
		for i, w := range previousOwners[k].owners {
			previousOwnerAddresses[i] = persist.EthereumAddressAtBlock{Address: w.owner, Block: w.boi.blockNumber}
		}

		uri := uris[k]
		delete(uris, k)

		t := persist.Token{
			TokenID:          tokenID,
			ContractAddress:  contractAddress,
			OwnerAddress:     v.owner,
			Quantity:         persist.HexString("1"),
			OwnershipHistory: previousOwnerAddresses,
			TokenType:        persist.TokenTypeERC721,
			TokenURI:         uri.uri,
			Chain:            i.chain,
			BlockNumber:      v.boi.blockNumber,
		}

		result = append(result, t)
		delete(owners, k)
	}
	for k, v := range balances {
		contractAddress, tokenID, err := k.GetParts()
		if err != nil {
			logger.For(ctx).WithError(err).Errorf("error getting parts from %s: - %s | val: %+v", k, err, v)
			continue
		}

		uri := uris[k]
		delete(uris, k)

		for addr, balance := range v.balances {

			t := persist.Token{
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				OwnerAddress:    addr,
				Quantity:        persist.HexString(balance.amnt.Text(16)),
				TokenType:       persist.TokenTypeERC1155,

				TokenURI: uri.uri,

				Chain:       i.chain,
				BlockNumber: balance.block,
			}
			result = append(result, t)
			delete(balances, k)
		}
	}

	return result
}

func upsertTokensAndContracts(ctx context.Context, t []persist.Token, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, dbMu *sync.Mutex) error {

	err := func() error {
		dbMu.Lock()
		defer dbMu.Unlock()
		now := time.Now()
		logger.For(ctx).Debugf("Upserting %d tokens", len(t))
		// upsert tokens in batches of 500
		for i := 0; i < len(t); i += 500 {
			end := i + 500
			if end > len(t) {
				end = len(t)
			}
			err := tokenRepo.BulkUpsert(ctx, t[i:end])
			if err != nil {
				if strings.Contains(err.Error(), "deadlock detected (SQLSTATE 40P01)") {
					logger.For(ctx).Errorf("Deadlock detected, retrying upsert")
					time.Sleep(5 * time.Second)
					if err := tokenRepo.BulkUpsert(ctx, t[i:end]); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
		logger.For(ctx).Debugf("Upserted %d tokens in %v time", len(t), time.Since(now))
		return nil
	}()
	if err != nil {
		return err
	}

	contractsChan := make(chan persist.Contract)
	go func() {
		defer close(contractsChan)
		contracts := make(map[persist.EthereumAddress]bool)

		wp := workerpool.New(3)

		for _, token := range t {
			to := token
			if contracts[to.ContractAddress] {
				continue
			}
			wp.Submit(func() {
				ctx := sentryutil.NewSentryHubContext(ctx)
				contract := persist.Contract{
					Address:     to.ContractAddress,
					LatestBlock: to.BlockNumber,
				}
				if rpcEnabled {
					contract = fillContractFields(ctx, ethClient, to.ContractAddress, to.BlockNumber)
				}
				logger.For(ctx).Debugf("Processing contract %s", contract.Address)
				contractsChan <- contract
			})

			contracts[to.ContractAddress] = true
		}
		wp.StopWait()
	}()

	finalNow := time.Now()

	allContracts := make([]persist.Contract, 0, len(t)/2)
	for contract := range contractsChan {
		allContracts = append(allContracts, contract)
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	logger.For(ctx).Debugf("Upserting %d contracts", len(allContracts))
	err = contractRepo.BulkUpsert(ctx, allContracts)
	if err != nil {
		if strings.Contains(err.Error(), "deadlock detected (SQLSTATE 40P01)") {
			logger.For(ctx).Errorf("Deadlock detected, retrying upserting contracts")
			time.Sleep(time.Second * 3)
			if err = contractRepo.BulkUpsert(ctx, allContracts); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("err upserting contracts: %s", err.Error())
		}
	}
	logger.For(ctx).Debugf("Upserted %d contracts in %v time", len(allContracts), time.Since(finalNow))
	return nil
}

func fillContractFields(ctx context.Context, ethClient *ethclient.Client, contractAddress persist.EthereumAddress, lastSyncedBlock persist.BlockNumber) persist.Contract {
	c := persist.Contract{
		Address:     contractAddress,
		LatestBlock: lastSyncedBlock,
	}
	cMetadata, err := rpc.RetryGetTokenContractMetadata(ctx, contractAddress, ethClient)
	if err != nil {
		logEntry := logger.For(ctx).WithError(err).WithFields(logrus.Fields{
			"contractAddress": contractAddress,
			"rpcCall":         "eth_call",
		})
		logEthCallRPCError(logEntry, err, "error getting contract metadata")
	} else {
		c.Name = persist.NullString(cMetadata.Name)
		c.Symbol = persist.NullString(cMetadata.Symbol)
	}
	return c
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

func saveLogsInBlockRange(ctx context.Context, curBlock, nextBlock string, logsTo []types.Log, storageClient *storage.Client) {
	logger.For(ctx).Infof("Saving logs in block range %s to %s", curBlock, nextBlock)
	obj := storageClient.Bucket(env.GetString(ctx, "GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock, nextBlock))
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
	// if err := recover(); err != nil {
	// 	logger.For(ctx).Errorf("Error in indexer: %v", err)
	// 	time.Sleep(time.Second * 10)
	// }
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
