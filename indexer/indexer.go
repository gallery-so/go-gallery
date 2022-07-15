package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// var defaultStartingBlock persist.BlockNumber = 5000000
var defaultStartingBlock persist.BlockNumber = 13768584

var erc1155ABI, _ = contracts.IERC1155MetaData.GetAbi()

var uniqueMetadataHandlers = uniqueMetadatas{
	persist.EthereumAddress("0xd4e4078ca3495de5b1d4db434bebc5a986197782"): autoglyphs,
	persist.EthereumAddress("0x60f3680350f65beb2752788cb48abfce84a4759e"): colorglyphs,
	persist.EthereumAddress("0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"): ens,
}

const defaultWorkerPoolSize = 8

const defaultWorkerPoolWaitSize = 25

const blocksPerLogsCall = 50

// eventHash represents an event keccak256 hash
type eventHash string

const (
	// transferEventHash represents the keccak256 hash of Transfer(address,address,uint256)
	transferEventHash eventHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// transferSingleEventHash represents the keccak256 hash of TransferSingle(address,address,address,uint256,uint256)
	transferSingleEventHash eventHash = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	// transferBatchEventHash represents the keccak256 hash of TransferBatch(address,address,address,uint256[],uint256[])
	transferBatchEventHash eventHash = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
	// uriEventHash represents the keccak256 hash of URI(string,uint256)
	uriEventHash eventHash = "0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b"
	// // foundationMintedEventHash represents the keccak256 hash of Minted(address,uint256,string,string)
	// foundationMintedEventHash eventHash = "0xe2406cfd356cfbe4e42d452bde96d27f48c423e5f02b5d78695893308399519d"
	// //foundationTransferEventHash represents the keccak256 hash of NFTOwnerMigrated(uint256,address,address)
	// foundationTransferEventHash eventHash = "0xde55f075ebd46256cd6bd57d8fb53e0406f687db372e90ae8c18e72be46f5c16"
)

type tokenMetadata struct {
	ti persist.EthereumTokenIdentifiers
	md persist.TokenMetadata
}

type tokenBalances struct {
	ti      persist.EthereumTokenIdentifiers
	from    persist.EthereumAddress
	to      persist.EthereumAddress
	fromAmt *big.Int
	toAmt   *big.Int
	block   persist.BlockNumber
}

type tokenURI struct {
	ti  persist.EthereumTokenIdentifiers
	uri persist.TokenURI
}

type transfersAtBlock struct {
	block     persist.BlockNumber
	transfers []rpc.Transfer
}

type ownerAtBlock struct {
	ti    persist.EthereumTokenIdentifiers
	owner persist.EthereumAddress
	block persist.BlockNumber
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

// indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type indexer struct {
	ethClient     *ethclient.Client
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	storageClient *storage.Client
	tokenRepo     persist.TokenRepository
	contractRepo  persist.ContractRepository
	contractDBMu  *sync.Mutex
	tokenDBMu     *sync.Mutex

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64
	lastSyncedBlock uint64

	isListening bool

	uniqueMetadatas uniqueMetadatas
}

// newIndexer sets up an indexer for retrieving the specified events that will process tokens
func newIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, pChain persist.Chain, pEvents []eventHash) *indexer {
	mostRecentBlockUint64, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	return &indexer{

		ethClient:     ethClient,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		storageClient: storageClient,
		tokenRepo:     tokenRepo,
		contractRepo:  contractRepo,
		contractDBMu:  &sync.Mutex{},
		tokenDBMu:     &sync.Mutex{},

		chain: pChain,

		eventHashes:     pEvents,
		mostRecentBlock: mostRecentBlockUint64,
		uniqueMetadatas: uniqueMetadataHandlers,
	}
}

// INITIALIZATION FUNCS ---------------------------------------------------------

// Start begins indexing events from the blockchain
func (i *indexer) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	lastSyncedBlock := defaultStartingBlock
	recentDBBlock, err := i.tokenRepo.MostRecentBlock(ctx)
	if err == nil && recentDBBlock > defaultStartingBlock {
		lastSyncedBlock = recentDBBlock
	}
	cancel()

	remainder := lastSyncedBlock % blocksPerLogsCall
	lastSyncedBlock -= (remainder + (blocksPerLogsCall * defaultWorkerPoolWaitSize))
	i.lastSyncedBlock = uint64(lastSyncedBlock)

	logrus.Infof("Starting indexer from block %d", lastSyncedBlock)

	wp := workerpool.New(defaultWorkerPoolSize)

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	go i.listenForNewBlocks()
	for ; lastSyncedBlock.Uint64() < atomic.LoadUint64(&i.mostRecentBlock); lastSyncedBlock += blocksPerLogsCall {
		input := lastSyncedBlock
		toQueue := func() {
			i.startPipeline(input, topics)
		}
		if wp.WaitingQueueSize() > defaultWorkerPoolWaitSize {
			wp.SubmitWait(toQueue)
		} else {
			wp.Submit(toQueue)
		}
	}
	wp.StopWait()
	logrus.Info("Finished processing old logs, subscribing to new logs...")
	i.lastSyncedBlock = uint64(lastSyncedBlock)
	for {
		timeAfterWait := <-time.After(time.Minute * 3)
		i.startNewBlocksPipeline(topics)
		logrus.Infof("Waiting for new blocks... Finished recent blocks in %s", time.Since(timeAfterWait))
	}
}

func (i *indexer) startPipeline(start persist.BlockNumber, topics [][]common.Hash) {
	startTime := time.Now()
	i.isListening = false
	uris := make(chan tokenURI)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	transfers := make(chan []transfersAtBlock)

	go i.processLogs(transfers, start, topics)
	go i.processTransfers(transfers, uris, owners, previousOwners, balances)
	i.processTokens(uris, owners, previousOwners, balances)
	if i.lastSyncedBlock < start.Uint64() {
		i.lastSyncedBlock = start.Uint64()
	}
	logrus.Warnf("Finished processing %d blocks from block %d in %s", blocksPerLogsCall, start.Uint64(), time.Since(startTime))
}
func (i *indexer) startNewBlocksPipeline(topics [][]common.Hash) {
	i.isListening = true
	uris := make(chan tokenURI)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	transfers := make(chan []transfersAtBlock)
	subscriptions := make(chan types.Log)
	go i.subscribeNewLogs(transfers, subscriptions, topics)
	go i.processTransfers(transfers, uris, owners, previousOwners, balances)
	i.processTokens(uris, owners, previousOwners, balances)
}

func (i *indexer) listenForNewBlocks() {
	for {
		<-time.After(time.Minute * 2)
		finalBlockUint, err := i.ethClient.BlockNumber(context.Background())
		if err != nil {
			panic(fmt.Sprintf("error getting block number: %s", err))
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logrus.Debugf("final block number: %v", finalBlockUint)
	}
}

// LOGS FUNCS ---------------------------------------------------------------

func (i *indexer) processLogs(transfersChan chan<- []transfersAtBlock, startingBlock persist.BlockNumber, topics [][]common.Hash) {
	defer close(transfersChan)

	curBlock := startingBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(blocksPerLogsCall)))

	logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var logsTo []types.Log
	reader, err := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock.String(), nextBlock.String())).NewReader(ctx)
	if err == nil {
		func() {
			defer reader.Close()
			err = json.NewDecoder(reader).Decode(&logsTo)
			if err != nil {
				panic(err)
			}
		}()
	} else {
		logrus.Errorf("error getting logs from GCP: %s", err)
	}
	if len(logsTo) > 0 {
		lastLog := logsTo[len(logsTo)-1]
		if nextBlock.Uint64()-lastLog.BlockNumber > (blocksPerLogsCall / 5) {
			logrus.Warnf("Last log is %d blocks old, skipping", nextBlock.Uint64()-lastLog.BlockNumber)
			logsTo = []types.Log{}
		}
	}
	if len(logsTo) == 0 {
		logsTo, err = i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: curBlock,
			ToBlock:   nextBlock,
			Topics:    topics,
		})
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("ERR-%s-%s", curBlock.String(), nextBlock.String())).NewWriter(ctx)
			defer storageWriter.Close()
			errData := map[string]interface{}{
				"from": curBlock.String(),
				"to":   nextBlock.String(),
				"err":  err.Error(),
			}
			logrus.Error(errData)
			err = json.NewEncoder(storageWriter).Encode(errData)
			if err != nil {
				panic(err)
			}
			return
		}

		obj := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock.String(), nextBlock.String()))
		obj.Delete(ctx)
		storageWriter := obj.NewWriter(ctx)
		defer storageWriter.Close()

		if err := json.NewEncoder(storageWriter).Encode(logsTo); err != nil {
			panic(err)
		}
	} else {
		logrus.Info("Found logs in cache...")
	}

	logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

	transfers := logsToTransfers(logsTo, i.ethClient)

	logrus.Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

	transfersAtBlocks := transfersToTransfersAtBlock(transfers)

	if len(transfersAtBlocks) > 0 && transfersAtBlocks != nil {
		logrus.Debugf("Sending %d total transfers to transfers channel", len(transfers))
		interval := len(transfersAtBlocks) / 4
		if interval == 0 {
			interval = 1
		}
		for j := 0; j < len(transfersAtBlocks); j += interval {
			to := j + interval
			if to > len(transfersAtBlocks) {
				to = len(transfersAtBlocks)
			}
			transfersChan <- transfersAtBlocks[j:to]
		}

	}
	logrus.Debug("Finished processing logs, closing transfers channel...")
}

func logsToTransfers(pLogs []types.Log, ethClient *ethclient.Client) []rpc.Transfer {

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
			})

			logrus.Debugf("Processed transfer event in %s", time.Since(initial))
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferSingleEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}

			eventData := map[string]interface{}{}
			err := erc1155ABI.UnpackIntoMap(eventData, "TransferSingle", pLog.Data)
			if err != nil {
				logrus.WithError(err).Error("Failed to unpack TransferSingle event")
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
			})
			logrus.Debugf("Processed single transfer event in %s", time.Since(initial))
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferBatchEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}

			eventData := map[string]interface{}{}
			err := erc1155ABI.UnpackIntoMap(eventData, "TransferBatch", pLog.Data)
			if err != nil {
				logrus.WithError(err).Error("Failed to unpack TransferBatch event")
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
				})
			}
			logrus.Debugf("Processed batch event in %s", time.Since(initial))
		default:
			logrus.WithFields(logrus.Fields{"address": pLog.Address, "block": pLog.BlockNumber, "event_type": pLog.Topics[0]}).Warn("unknown event")
		}
	}
	return result
}

func (i *indexer) subscribeNewLogs(transfersChan chan<- []transfersAtBlock, subscriptions chan types.Log, topics [][]common.Hash) {

	defer close(transfersChan)

	mostRecentBlock, err := i.ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	logrus.Infof("Subscribing to new logs from block %d starting with block %d", mostRecentBlock, i.lastSyncedBlock)

	wp := workerpool.New(10)
	for j := i.lastSyncedBlock; j <= mostRecentBlock; j += blocksPerLogsCall {
		curBlock := j
		wp.Submit(
			func() {
				nextBlock := curBlock + blocksPerLogsCall
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()

				logsTo, err := i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
					FromBlock: persist.BlockNumber(curBlock).BigInt(),
					ToBlock:   persist.BlockNumber(nextBlock).BigInt(),
					Topics:    topics,
				})
				if err != nil {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("ERR-%d-%d", i.lastSyncedBlock, mostRecentBlock)).NewWriter(ctx)
					defer storageWriter.Close()
					errData := map[string]interface{}{
						"from": curBlock,
						"to":   nextBlock,
						"err":  err.Error(),
					}
					logrus.Error(errData)
					err = json.NewEncoder(storageWriter).Encode(errData)
					if err != nil {
						panic(err)
					}
					return
				}

				logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock)

				transfers := logsToTransfers(logsTo, i.ethClient)

				logrus.Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

				transfersAtBlocks := transfersToTransfersAtBlock(transfers)

				if len(transfersAtBlocks) > 0 && transfersAtBlocks != nil {
					logrus.Debugf("Sending %d total transfers to transfers channel", len(transfers))
					interval := len(transfersAtBlocks) / 4
					if interval == 0 {
						interval = 1
					}
					for j := 0; j < len(transfersAtBlocks); j += interval {
						to := j + interval
						if to > len(transfersAtBlocks) {
							to = len(transfersAtBlocks)
						}
						transfersChan <- transfersAtBlocks[j:to]
					}
				}
			})
	}
	wp.StopWait()
	logrus.Infof("Processed logs from %d to %d.", i.lastSyncedBlock, mostRecentBlock)

	i.lastSyncedBlock = mostRecentBlock - 1
}

// TRANSFERS FUNCS -------------------------------------------------------------

func (i *indexer) processTransfers(incomingTransfers <-chan []transfersAtBlock, uris chan<- tokenURI, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances) {
	defer close(uris)
	defer close(owners)
	defer close(previousOwners)
	defer close(balances)

	wp := workerpool.New(20)

	logrus.Info("Starting to process transfers...")
	for transfers := range incomingTransfers {
		if transfers == nil || len(transfers) == 0 {
			continue
		}

		logrus.Debugf("Processing %d transfers", len(transfers))
		submit := transfers
		wp.Submit(func() {
			processTransfers(i, submit, uris, nil, owners, previousOwners, balances, nil, false)
		})
	}
	logrus.Info("Waiting for transfers to finish...")
	wp.StopWait()
	logrus.Info("Closing field channels...")
}

func (i *indexer) processNewTransfers(incomingTransfers <-chan []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances, medias chan<- tokenMedia) {
	defer close(uris)
	defer close(metadatas)
	defer close(owners)
	defer close(previousOwners)
	defer close(balances)
	defer close(medias)

	wp := workerpool.New(20)

	logrus.Info("Starting to process transfers...")
	for transfers := range incomingTransfers {
		if transfers == nil || len(transfers) == 0 {
			continue
		}

		logrus.Debugf("Processing %d transfers", len(transfers))
		submit := transfers
		wp.Submit(func() {
			processTransfers(i, submit, uris, metadatas, owners, previousOwners, balances, medias, true)
		})
	}
	logrus.Info("Waiting for transfers to finish...")
	wp.StopWait()
	logrus.Info("Closing field channels...")
}

func processTransfers(i *indexer, transfers []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances, medias chan<- tokenMedia, optionalFields bool) {

	for _, transferAtBlock := range transfers {
		for _, transfer := range transferAtBlock.transfers {
			initial := time.Now()
			func() {

				contractAddress := persist.EthereumAddress(transfer.ContractAddress.String())
				from := transfer.From
				to := transfer.To
				tokenID := transfer.TokenID

				key := persist.NewEthereumTokenIdentifiers(contractAddress, tokenID)
				// logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

				findFields(i, transfer, key, to, from, contractAddress, tokenID, balances, uris, metadatas, owners, previousOwners, medias, optionalFields)

				logrus.WithFields(logrus.Fields{"duration": time.Since(initial)}).Debugf("Processed transfer %s to %s and from %s ", key, to, from)
			}()
		}

	}

}

func findFields(i *indexer, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, to persist.EthereumAddress, from persist.EthereumAddress, contractAddress persist.EthereumAddress, tokenID persist.TokenID, balances chan<- tokenBalances, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, medias chan<- tokenMedia, optionalFields bool) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	if optionalFields {
		wg.Add(2)
	}
	switch persist.TokenType(transfer.TokenType) {
	case persist.TokenTypeERC721:

		wg.Add(2)

		go func() {
			defer wg.Done()
			curOwner := ownerAtBlock{key, to, transfer.BlockNumber}
			owners <- curOwner
		}()

		go func() {
			defer wg.Done()
			prevOwner := ownerAtBlock{key, from, transfer.BlockNumber}
			previousOwners <- prevOwner
		}()

	case persist.TokenTypeERC1155:
		wg.Add(1)

		go func() {
			defer wg.Done()

			bals, err := getBalances(contractAddress, from, tokenID, key, transfer.BlockNumber, to, i.ethClient)
			if err != nil {
				logrus.WithError(err).Errorf("error getting balance of %s for %s", from, key)
				storeErr(err, "ERR-BALANCE", from, key, transfer.BlockNumber, i.storageClient)
			}

			balances <- bals
		}()

	default:
		panic("unknown token type")
	}

	var uri persist.TokenURI

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	// defer cancel()

	// ct, tid, err := key.GetParts()
	// if err != nil {
	// 	logrus.WithError(err).Errorf("error getting parts of %s", key)
	// 	storeErr(err, "ERR-PARTS", from, key, transfer.BlockNumber, i.storageClient)
	// 	panic(err)
	// }
	// ts, _ := i.tokenRepo.GetByTokenIdentifiers(ctx, tid, ct, 1, 0)
	// if ts != nil && len(ts) > 0 {
	// 	first := ts[0]
	// 	if first.TokenURI != "" {
	// 		uri = persist.TokenURI(first.TokenURI)
	// 	}
	// 	if first.TokenMetadata != nil && len(first.TokenMetadata) > 0 {
	// 		metadata = persist.TokenMetadata(first.TokenMetadata)
	// 	}
	// }

	// if uri == "" {
	uri = getURI(contractAddress, tokenID, transfer.TokenType, i.ethClient)
	// }

	go func() {
		defer wg.Done()
		uris <- tokenURI{key, uri}
	}()

	if optionalFields {
		var metadata persist.TokenMetadata
		if metadata == nil {
			metadata, uri = getMetadata(contractAddress, uri, tokenID, i.uniqueMetadatas, i.ipfsClient, i.arweaveClient)
		}
		go func() {
			defer wg.Done()
			if len(metadata) > 0 {
				metadatas <- tokenMetadata{key, metadata}
			}
		}()
		go func() {
			defer wg.Done()
			findOptionalFields(i, key, to, from, uri, metadata, medias)
		}()
	}

	wg.Wait()
}

func findOptionalFields(i *indexer, key persist.EthereumTokenIdentifiers, to, from persist.EthereumAddress, tokenURI persist.TokenURI, metadata persist.TokenMetadata, medias chan<- tokenMedia) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	contractAddress, tokenID, err := key.GetParts()
	if err != nil {
		logrus.WithError(err).Errorf("error getting parts of %s", key)
		return
	}

	med, err := media.MakePreviewsForMetadata(ctx, metadata, contractAddress.String(), tokenID, tokenURI, i.chain, i.ipfsClient, i.arweaveClient, i.storageClient)
	if err != nil {
		logrus.WithError(err).Errorf("error making previews for %s", key)
		return
	}

	res := tokenMedia{ti: key, media: med}
	medias <- res
}

func getBalances(contractAddress persist.EthereumAddress, from persist.EthereumAddress, tokenID persist.TokenID, key persist.EthereumTokenIdentifiers, blockNumber persist.BlockNumber, to persist.EthereumAddress, ethClient *ethclient.Client) (tokenBalances, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	ierc1155, err := contracts.NewIERC1155Caller(contractAddress.Address(), ethClient)
	if err != nil {
		logrus.WithError(err).Errorf("error creating IERC1155 contract caller for %s", contractAddress)
		return tokenBalances{}, err
	}
	var fromBalance, toBalance *big.Int
	if from.String() != persist.ZeroAddress.String() {
		fromBalance, err = ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, from.Address(), tokenID.BigInt())
		if err != nil {
			return tokenBalances{}, err
		}
	}
	if to.String() != persist.ZeroAddress.String() {
		toBalance, err = ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, to.Address(), tokenID.BigInt())
		if err != nil {
			return tokenBalances{}, err
		}
	}
	bal := tokenBalances{key, from, to, fromBalance, toBalance, blockNumber}
	return bal, nil
}

func getURI(contractAddress persist.EthereumAddress, tokenID persist.TokenID, tokenType persist.TokenType, ethClient *ethclient.Client) persist.TokenURI {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	u, err := rpc.GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"id": tokenID, "contract": contractAddress}).Error("error getting URI for token")
		if strings.Contains(err.Error(), "execution reverted") {
			u = persist.InvalidTokenURI
		}
	}

	uriReplaced := u.ReplaceID(tokenID)
	return uriReplaced
}

func getMetadata(contractAddress persist.EthereumAddress, uriReplaced persist.TokenURI, tokenID persist.TokenID, um uniqueMetadatas, ipfsClient *shell.Shell, arweaveClient *goar.Client) (persist.TokenMetadata, persist.TokenURI) {
	var metadata persist.TokenMetadata
	var err error
	if handler, ok := um[contractAddress]; ok {
		uriReplaced, metadata, err = handler(uriReplaced, contractAddress, tokenID)
		if err != nil {
			logrus.WithError(err).WithField("uri", uriReplaced).Error("error getting metadata for token")
		}
	} else {
		if uriReplaced != "" && uriReplaced != persist.InvalidTokenURI {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			metadata, err = rpc.GetMetadataFromURI(ctx, uriReplaced, ipfsClient, arweaveClient)
			if err != nil {
				switch err.(type) {
				case rpc.ErrHTTP:
					if err.(rpc.ErrHTTP).Status == http.StatusNotFound {
						metadata = persist.TokenMetadata{"error": "not found"}
					}
				case *net.DNSError:
					metadata = persist.TokenMetadata{"error": "dns error"}
				}
				logrus.WithError(err).WithField("uri", uriReplaced).Error("error getting metadata for token")

			}
			cancel()
		}
	}
	return metadata, uriReplaced
}

// TOKENS FUNCS ---------------------------------------------------------------

func (i *indexer) processTokens(uris <-chan tokenURI, owners <-chan ownerAtBlock, previousOwners <-chan ownerAtBlock, balances <-chan tokenBalances) {

	wg := &sync.WaitGroup{}
	wg.Add(4)
	ownersMap := map[persist.EthereumTokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[persist.EthereumTokenIdentifiers][]ownerAtBlock{}
	balancesMap := map[persist.EthereumTokenIdentifiers]map[persist.EthereumAddress]balanceAtBlock{}
	metadatasMap := map[persist.EthereumTokenIdentifiers]tokenMetadata{}
	urisMap := map[persist.EthereumTokenIdentifiers]tokenURI{}

	go receiveBalances(wg, balances, balancesMap, i.tokenRepo)
	go receiveOwners(wg, owners, ownersMap, i.tokenRepo)
	go receiveURIs(wg, uris, urisMap)
	go receivePreviousOwners(wg, previousOwners, previousOwnersMap, i.tokenRepo)
	wg.Wait()

	logrus.Info("Done recieving field data, converting fields into tokens...")

	createTokens(i, ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap, map[persist.EthereumTokenIdentifiers]tokenMedia{})
}

func (i *indexer) processNewTokens(uris <-chan tokenURI, metadatas <-chan tokenMetadata, owners <-chan ownerAtBlock, previousOwners <-chan ownerAtBlock, balances <-chan tokenBalances, medias <-chan tokenMedia) {

	wg := &sync.WaitGroup{}
	wg.Add(6)
	ownersMap := map[persist.EthereumTokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[persist.EthereumTokenIdentifiers][]ownerAtBlock{}
	balancesMap := map[persist.EthereumTokenIdentifiers]map[persist.EthereumAddress]balanceAtBlock{}
	metadatasMap := map[persist.EthereumTokenIdentifiers]tokenMetadata{}
	urisMap := map[persist.EthereumTokenIdentifiers]tokenURI{}
	mediasMap := map[persist.EthereumTokenIdentifiers]tokenMedia{}

	go receiveBalances(wg, balances, balancesMap, i.tokenRepo)
	go receiveOwners(wg, owners, ownersMap, i.tokenRepo)
	go receiveMetadatas(wg, metadatas, metadatasMap)
	go receiveURIs(wg, uris, urisMap)
	go receivePreviousOwners(wg, previousOwners, previousOwnersMap, i.tokenRepo)
	go receiveMedias(wg, medias, mediasMap)
	wg.Wait()

	logrus.Info("Done recieving field data, converting fields into tokens...")

	createTokens(i, ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap, mediasMap)

}

func createTokens(i *indexer, ownersMap map[persist.EthereumTokenIdentifiers]ownerAtBlock, previousOwnersMap map[persist.EthereumTokenIdentifiers][]ownerAtBlock, balancesMap map[persist.EthereumTokenIdentifiers]map[persist.EthereumAddress]balanceAtBlock, metadatasMap map[persist.EthereumTokenIdentifiers]tokenMetadata, urisMap map[persist.EthereumTokenIdentifiers]tokenURI, mediasMap map[persist.EthereumTokenIdentifiers]tokenMedia) {
	tokens := i.fieldMapsToTokens(ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap, mediasMap)
	if tokens == nil || len(tokens) == 0 {
		logrus.Info("No tokens to process")
		return
	}

	logrus.Info("Created tokens to insert into database...")

	timeout := (time.Minute * time.Duration((len(tokens) / 100)))
	logrus.Infof("Upserting %d tokens and contracts with a timeout of %s", len(tokens), timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := upsertTokensAndContracts(ctx, tokens, i.tokenRepo, i.contractRepo, i.ethClient, i.tokenDBMu, i.contractDBMu)
	if err != nil {
		logrus.WithError(err).Error("error upserting tokens and contracts")
		randKey := util.RandStringBytes(24)
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("DB-ERR-%s", randKey)).NewWriter(ctx)
		defer storageWriter.Close()
		errData := map[string]interface{}{
			"tokens": tokens,
		}
		logrus.Error(errData)
		newErr := json.NewEncoder(storageWriter).Encode(errData)
		if newErr != nil {
			panic(newErr)
		}
		panic(fmt.Sprintf("error upserting tokens and contracts: %s - error key: %s", err, randKey))
	}

	logrus.Info("Done upserting tokens and contracts")
}

func receiveURIs(wg *sync.WaitGroup, uris <-chan tokenURI, uriMap map[persist.EthereumTokenIdentifiers]tokenURI) {
	defer wg.Done()

	for uri := range uris {
		uriMap[uri.ti] = uri
	}
}

func receiveMetadatas(wg *sync.WaitGroup, metadatas <-chan tokenMetadata, metaMap map[persist.EthereumTokenIdentifiers]tokenMetadata) {
	defer wg.Done()

	for meta := range metadatas {
		metaMap[meta.ti] = meta
	}
}

func receiveMedias(wg *sync.WaitGroup, medias <-chan tokenMedia, metaMap map[persist.EthereumTokenIdentifiers]tokenMedia) {
	defer wg.Done()

	for media := range medias {
		metaMap[media.ti] = media
	}
}

func receivePreviousOwners(wg *sync.WaitGroup, prevOwners <-chan ownerAtBlock, prevOwnersMap map[persist.EthereumTokenIdentifiers][]ownerAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for previousOwner := range prevOwners {
		currentPreviousOwners, ok := prevOwnersMap[previousOwner.ti]
		if !ok {
			currentPreviousOwners = make([]ownerAtBlock, 0, 20)
		}
		currentPreviousOwners = append(currentPreviousOwners, previousOwner)
		prevOwnersMap[previousOwner.ti] = currentPreviousOwners
	}
}

func receiveBalances(wg *sync.WaitGroup, balanceChan <-chan tokenBalances, balances map[persist.EthereumTokenIdentifiers]map[persist.EthereumAddress]balanceAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for balance := range balanceChan {

		balanceMap, ok := balances[balance.ti]
		if !ok {
			balanceMap = make(map[persist.EthereumAddress]balanceAtBlock)
		}
		toBal := balanceMap[balance.to]
		if toBal.block < balance.block {
			toBal.block = balance.block
			toBal.amnt = balance.toAmt
			balanceMap[balance.to] = toBal
		}

		fromBal := balanceMap[balance.from]
		if fromBal.block < balance.block {
			fromBal.block = balance.block
			fromBal.amnt = balance.fromAmt
			balanceMap[balance.from] = fromBal
		}

		if len(balanceMap) > 0 {
			balances[balance.ti] = balanceMap
		}

	}
}

func receiveOwners(wg *sync.WaitGroup, ownersChan <-chan ownerAtBlock, owners map[persist.EthereumTokenIdentifiers]ownerAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for owner := range ownersChan {
		owners[owner.ti] = owner
	}
}

func (i *indexer) fieldMapsToTokens(owners map[persist.EthereumTokenIdentifiers]ownerAtBlock, previousOwners map[persist.EthereumTokenIdentifiers][]ownerAtBlock, balances map[persist.EthereumTokenIdentifiers]map[persist.EthereumAddress]balanceAtBlock, metadatas map[persist.EthereumTokenIdentifiers]tokenMetadata, uris map[persist.EthereumTokenIdentifiers]tokenURI, medias map[persist.EthereumTokenIdentifiers]tokenMedia) []persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v)
	}
	result := make([]persist.Token, 0, len(owners)+totalBalances)

	for k, v := range owners {
		contractAddress, tokenID, err := k.GetParts()
		if err != nil {
			logrus.WithError(err).Errorf("error getting parts from %s: - %s | val: %+v", k, err, v)
			continue
		}
		previousOwnerAddresses := make([]persist.EthereumAddressAtBlock, len(previousOwners[k]))
		for i, w := range previousOwners[k] {
			previousOwnerAddresses[i] = persist.EthereumAddressAtBlock{Address: w.owner, Block: w.block}
		}
		delete(previousOwners, k)
		metadata := metadatas[k]
		delete(metadatas, k)
		var name, description string

		if w, ok := findFirstFieldFromMetadata(metadata.md, "name").(string); ok {
			name = w
		}
		if w, ok := findFirstFieldFromMetadata(metadata.md, "description").(string); ok {
			description = w
		}

		uri := uris[k]
		delete(uris, k)

		media := medias[k]
		delete(medias, k)

		t := persist.Token{
			TokenID:          tokenID,
			ContractAddress:  contractAddress,
			OwnerAddress:     v.owner,
			Quantity:         persist.HexString("1"),
			Name:             persist.NullString(validate.SanitizationPolicy.Sanitize(name)),
			Description:      persist.NullString(validate.SanitizationPolicy.Sanitize(description)),
			OwnershipHistory: previousOwnerAddresses,
			TokenType:        persist.TokenTypeERC721,
			TokenMetadata:    metadata.md,
			TokenURI:         uri.uri,
			Chain:            i.chain,
			BlockNumber:      v.block,
			Media:            media.media,
		}

		result = append(result, t)
		delete(owners, k)
	}
	for k, v := range balances {
		contractAddress, tokenID, err := k.GetParts()
		if err != nil {
			logrus.WithError(err).Errorf("error getting parts from %s: - %s | val: %+v", k, err, v)
			continue
		}

		metadata := metadatas[k]
		delete(metadatas, k)
		var name, description string

		if v, ok := findFirstFieldFromMetadata(metadata.md, "name").(string); ok {
			name = v
		}
		if v, ok := findFirstFieldFromMetadata(metadata.md, "description").(string); ok {
			description = v
		}

		uri := uris[k]
		delete(uris, k)

		media := medias[k]
		delete(medias, k)

		for addr, balance := range v {

			t := persist.Token{
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				OwnerAddress:    addr,
				Quantity:        persist.HexString(balance.amnt.Text(16)),
				TokenType:       persist.TokenTypeERC1155,
				TokenMetadata:   metadata.md,
				TokenURI:        uri.uri,
				Name:            persist.NullString(validate.SanitizationPolicy.Sanitize(name)),
				Description:     persist.NullString(validate.SanitizationPolicy.Sanitize(description)),
				Chain:           i.chain,
				BlockNumber:     balance.block,
				Media:           media.media,
			}
			result = append(result, t)
			delete(balances, k)
		}
	}

	return result
}

func upsertTokensAndContracts(ctx context.Context, t []persist.Token, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, tokenMu *sync.Mutex, contractMu *sync.Mutex) error {

	err := func() error {
		tokenMu.Lock()
		defer tokenMu.Unlock()
		now := time.Now()
		logrus.Debugf("Upserting %d tokens", len(t))
		if err := tokenRepo.BulkUpsert(ctx, t); err != nil {
			return fmt.Errorf("err upserting %d tokens: %s", len(t), err.Error())
		}
		logrus.Debugf("Upserted %d tokens in %v time", len(t), time.Since(now))
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
				contract := fillContractFields(ethClient, to.ContractAddress, to.BlockNumber)
				logrus.Debugf("Processing contract %s", contract.Address)
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
	contractMu.Lock()
	defer contractMu.Unlock()
	logrus.Debugf("Upserting %d contracts", len(allContracts))
	err = contractRepo.BulkUpsert(ctx, allContracts)
	if err != nil {
		return fmt.Errorf("err upserting contracts: %s", err.Error())
	}
	logrus.Debugf("Upserted %d contracts in %v time", len(allContracts), time.Since(finalNow))
	return nil
}

func fillContractFields(ethClient *ethclient.Client, contractAddress persist.EthereumAddress, lastSyncedBlock persist.BlockNumber) persist.Contract {
	c := persist.Contract{
		Address:     contractAddress,
		LatestBlock: lastSyncedBlock,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	cMetadata, err := rpc.GetTokenContractMetadata(ctx, contractAddress, ethClient)
	if err != nil {
		// TODO figure out what type of error this is
		logrus.WithError(err).WithField("address", contractAddress).Error("error getting contract metadata")
	} else {
		c.Name = persist.NullString(cMetadata.Name)
		c.Symbol = persist.NullString(cMetadata.Symbol)
	}
	return c
}

// HELPER FUNCS ---------------------------------------------------------------

func findFirstFieldFromMetadata(metadata persist.TokenMetadata, fields ...string) interface{} {

	for _, field := range fields {
		if val := util.GetValueFromMapUnsafe(metadata, field, util.DefaultSearchDepth); val != nil {
			return val
		}
	}
	return nil
}

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
	return allTransfersAtBlock
}

func storeErr(err error, prefix string, from persist.EthereumAddress, key persist.EthereumTokenIdentifiers, atBlock persist.BlockNumber, storageClient *storage.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	storageWriter := storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s-%s-%s", prefix, from, key, atBlock)).NewWriter(ctx)
	defer storageWriter.Close()
	errData := map[string]interface{}{
		"from":  from,
		"key":   key,
		"block": atBlock,
		"err":   err.Error(),
	}
	logrus.Error(errData)
	err = json.NewEncoder(storageWriter).Encode(errData)
	if err != nil {
		panic(err)
	}
}
