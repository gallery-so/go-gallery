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
	"unsafe"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/appengine"
)

var defaultStartingBlock persist.BlockNumber = 5000000

var logSize = unsafe.Sizeof(types.Log{})

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
)

type tokenIdentifiers string

type tokenMetadata struct {
	ti tokenIdentifiers
	md persist.TokenMetadata
}

type tokenBalances struct {
	ti      tokenIdentifiers
	from    persist.Address
	to      persist.Address
	fromAmt *big.Int
	toAmt   *big.Int
	block   persist.BlockNumber
}

type tokenURI struct {
	ti  tokenIdentifiers
	uri persist.TokenURI
}

type transfersAtBlock struct {
	block     persist.BlockNumber
	transfers []rpc.Transfer
}

type uniqueMetadataHandler func(*Indexer, persist.TokenURI, persist.Address, persist.TokenID) (persist.TokenMetadata, error)

type uniqueMetadatas map[persist.Address]uniqueMetadataHandler

type ownerAtBlock struct {
	ti    tokenIdentifiers
	owner persist.Address
	block persist.BlockNumber
}

type balanceAtBlock struct {
	ti    tokenIdentifiers
	block persist.BlockNumber
	amnt  *big.Int
}

// Indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type Indexer struct {
	ethClient     *ethclient.Client
	ipfsClient    *shell.Shell
	storageClient *storage.Client
	tokenRepo     persist.TokenRepository
	contractRepo  persist.ContractRepository
	userRepo      persist.UserRepository

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64

	badURIs uint64

	uniqueMetadatas uniqueMetadatas
}

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens
func NewIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, storageClient *storage.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, userRepo persist.UserRepository, pChain persist.Chain, pEvents []eventHash, statsFileName string) *Indexer {
	mostRecentBlockUint64, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	return &Indexer{

		ethClient:     ethClient,
		ipfsClient:    ipfsClient,
		storageClient: storageClient,
		tokenRepo:     tokenRepo,
		contractRepo:  contractRepo,
		userRepo:      userRepo,

		chain: pChain,

		eventHashes:     pEvents,
		mostRecentBlock: mostRecentBlockUint64,
		uniqueMetadatas: getUniqueMetadataHandlers(),
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	lastSyncedBlock := defaultStartingBlock
	recentDBBlock, err := i.tokenRepo.MostRecentBlock(ctx)
	if err == nil && recentDBBlock > defaultStartingBlock {
		lastSyncedBlock = recentDBBlock
	}
	cancel()

	if remainder := lastSyncedBlock % blocksPerLogsCall; remainder != 0 {
		lastSyncedBlock -= remainder
	}

	logrus.Infof("Starting indexer from block %d", lastSyncedBlock)

	wp := workerpool.New(10)

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	go i.listenForNewBlocks()
	for lastSyncedBlock.Uint64() < atomic.LoadUint64(&i.mostRecentBlock) {
		input := lastSyncedBlock
		toQueue := func() {
			i.startPipeline(input, topics)
		}
		if wp.WaitingQueueSize() > 100 {
			wp.SubmitWait(toQueue)
		} else {
			wp.Submit(toQueue)
		}
		lastSyncedBlock += blocksPerLogsCall
	}
	wp.StopWait()
	logrus.Info("Finished processing old logs, subscribing to new logs...")
	i.startNewBlocksPipeline(lastSyncedBlock, topics)
}

func (i *Indexer) startPipeline(start persist.BlockNumber, topics [][]common.Hash) {
	uris := make(chan tokenURI)
	metadatas := make(chan tokenMetadata)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	transfers := make(chan []transfersAtBlock)

	go i.processLogs(transfers, start, topics)
	go i.processTransfers(transfers, uris, metadatas, owners, previousOwners, balances)
	i.processTokens(uris, metadatas, owners, previousOwners, balances)
}
func (i *Indexer) startNewBlocksPipeline(start persist.BlockNumber, topics [][]common.Hash) {
	uris := make(chan tokenURI)
	metadatas := make(chan tokenMetadata)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	transfers := make(chan []transfersAtBlock)
	subscriptions := make(chan types.Log)
	go i.subscribeNewLogs(start, transfers, subscriptions, topics)
	go i.processTransfers(transfers, uris, metadatas, owners, previousOwners, balances)
	i.processTokens(uris, metadatas, owners, previousOwners, balances)
}

func (i *Indexer) processLogs(transfersChan chan<- []transfersAtBlock, startingBlock persist.BlockNumber, topics [][]common.Hash) {
	defer close(transfersChan)

	curBlock := startingBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(blocksPerLogsCall)))

	logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var logsTo []types.Log
	reader, err := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock.String(), nextBlock.String())).NewReader(ctx)
	if err == nil {
		defer reader.Close()
		logsTo = make([]types.Log, 0, 8000)
		err = json.NewDecoder(reader).Decode(&logsTo)
		if err != nil {
			panic(err)
		}
	} else {

		logsTo, err = i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: curBlock,
			ToBlock:   nextBlock,
			Topics:    topics,
		})
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
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

		storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("%s-%s", curBlock.String(), nextBlock.String())).NewWriter(ctx)
		defer storageWriter.Close()

		err := json.NewEncoder(storageWriter).Encode(logsTo)
		if err != nil {
			panic(err)
		}
	}

	logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

	transfers := logsToTransfers(logsTo, i.ethClient)

	logrus.Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

	transfersAtBlocks := transfersToTransfersAtBlock(transfers)

	if len(transfersAtBlocks) > 0 && transfersAtBlocks != nil {
		logrus.Infof("Sending %d total transfers to transfers channel", len(transfers))
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
	logrus.Info("Finished processing logs, closing transfers channel...")
}

func logsToTransfers(pLogs []types.Log, ethClient *ethclient.Client) []rpc.Transfer {

	erc1155ABI, err := contracts.IERC1155MetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	result := make([]rpc.Transfer, 0, len(pLogs)*2)
	for _, pLog := range pLogs {
		initial := time.Now()
		switch {
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferEventHash)):

			if len(pLog.Topics) < 4 {
				continue
			}

			// t := time.Now()
			// erc20, err := contracts.NewIERC20Caller(pLog.Address, ethClient)
			// if err == nil {
			// 	_, err := erc20.Allowance(&bind.CallOpts{}, common.HexToAddress(pLog.Topics[1].Hex()), common.HexToAddress(pLog.Topics[2].Hex()))
			// 	if err == nil {
			// 		continue
			// 	}
			// }
			// logrus.Infof("Figured out if contract was ERC20 in %s", time.Since(t))

			result = append(result, rpc.Transfer{
				From:            persist.Address(pLog.Topics[1].Hex()),
				To:              persist.Address(pLog.Topics[2].Hex()),
				TokenID:         persist.TokenID(pLog.Topics[3].Hex()),
				Amount:          1,
				BlockNumber:     persist.BlockNumber(pLog.BlockNumber),
				ContractAddress: persist.Address(pLog.Address.Hex()),
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
				From:            persist.Address(pLog.Topics[2].Hex()),
				To:              persist.Address(pLog.Topics[3].Hex()),
				TokenID:         persist.TokenID(id.Text(16)),
				Amount:          value.Uint64(),
				BlockNumber:     persist.BlockNumber(pLog.BlockNumber),
				ContractAddress: persist.Address(pLog.Address.Hex()),
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
					From:            persist.Address(pLog.Topics[2].Hex()),
					To:              persist.Address(pLog.Topics[3].Hex()),
					TokenID:         persist.TokenID(ids[j].Text(16)),
					Amount:          values[j].Uint64(),
					ContractAddress: persist.Address(pLog.Address.Hex()),
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

func (i *Indexer) listenForNewBlocks() {
	for {
		finalBlockUint, err := i.ethClient.BlockNumber(context.Background())
		if err != nil {
			panic(fmt.Sprintf("error getting block number: %s", err))
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logrus.Infof("final block number: %v", finalBlockUint)
		time.Sleep(time.Minute)
	}
}

func (i *Indexer) processTransfers(incomingTransfers <-chan []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances) {
	defer close(uris)
	defer close(metadatas)
	defer close(owners)
	defer close(previousOwners)
	defer close(balances)

	wp := workerpool.New(20)

	logrus.Info("Starting to process transfers...")
	for transfers := range incomingTransfers {
		if transfers == nil || len(transfers) == 0 {
			continue
		}

		logrus.Infof("Processing %d transfers", len(transfers))
		it := make([]transfersAtBlock, len(transfers))
		copy(it, transfers)
		wp.Submit(func() {
			submit := it
			processTransfers(i, submit, uris, metadatas, owners, previousOwners, balances)
		})
	}
	logrus.Info("Waiting for transfers to finish...")
	wp.StopWait()
	logrus.Info("Closing field channels...")
}

func processTransfers(i *Indexer, transfers []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances) {

	for _, transferAtBlock := range transfers {
		for _, transfer := range transferAtBlock.transfers {
			initial := time.Now()
			func() {

				wg := &sync.WaitGroup{}
				contractAddress := transfer.ContractAddress
				from := transfer.From
				to := transfer.To
				tokenID := transfer.TokenID

				key := makeKeyForToken(contractAddress, tokenID)
				// logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

				switch persist.TokenType(transfer.TokenType) {
				case persist.TokenTypeERC721:

					wg.Add(4)

					go func() {
						defer wg.Done()
						owners <- ownerAtBlock{key, to, transfer.BlockNumber}
					}()

					go func() {
						defer wg.Done()
						previousOwners <- ownerAtBlock{key, from, transfer.BlockNumber}
					}()

				case persist.TokenTypeERC1155:
					wg.Add(3)

					go func() {
						defer wg.Done()
						ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
						defer cancel()
						ierc1155, err := contracts.NewIERC1155Caller(contractAddress.Address(), i.ethClient)
						if err != nil {
							logrus.WithError(err).Errorf("error creating IERC1155 contract caller for %s", contractAddress)
							return
						}
						var fromBalance, toBalance *big.Int
						if from.String() != "0x0000000000000000000000000000000000000000" {
							fromBalance, err = ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, from.Address(), tokenID.BigInt())
							if err != nil {
								logrus.WithError(err).Errorf("error getting balance of %s for %s", from, key)
								storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("ERR-BALANCE-%s-%s", from, key)).NewWriter(ctx)
								defer storageWriter.Close()
								errData := map[string]interface{}{
									"from": from,
									"key":  key,
									"err":  err.Error(),
								}
								logrus.Error(errData)
								err = json.NewEncoder(storageWriter).Encode(errData)
								if err != nil {
									panic(err)
								}
								return
							}
						}
						if to.String() != "0x0000000000000000000000000000000000000000" {
							toBalance, err = ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, to.Address(), tokenID.BigInt())
							if err != nil {
								logrus.WithError(err).Errorf("error getting balance of %s for %s", to, key)
								storageWriter := i.storageClient.Bucket(viper.GetString("GCLOUD_TOKEN_LOGS_BUCKET")).Object(fmt.Sprintf("ERR-BALANCE-%s-%s", to, key)).NewWriter(ctx)
								defer storageWriter.Close()
								errData := map[string]interface{}{
									"to":  to,
									"key": key,
									"err": err.Error(),
								}
								logrus.Error(errData)
								err = json.NewEncoder(storageWriter).Encode(errData)
								if err != nil {
									panic(err)
								}
								return
							}
						}

						balances <- tokenBalances{key, from, to, fromBalance, toBalance, transfer.BlockNumber}
					}()

				default:
					panic("unknown token type")
				}

				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				u, err := rpc.GetTokenURI(ctx, transfer.TokenType, contractAddress, tokenID, i.ethClient)
				if err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{"id": tokenID, "contract": contractAddress}).Error("error getting URI for token")
					if strings.Contains(err.Error(), "execution reverted") {
						u = persist.InvalidTokenURI
					}
				}
				cancel()

				id, err := util.HexToBigInt(string(tokenID))
				if err != nil {
					panic(fmt.Sprintf("error converting tokenID to bigint: %s", err))
				}

				uriReplaced := persist.TokenURI(strings.TrimSpace(strings.ReplaceAll(u.String(), "{id}", id.Text(16))))

				go func() {
					defer wg.Done()
					uris <- tokenURI{key, uriReplaced}
				}()
				var metadata persist.TokenMetadata
				if handler, ok := i.uniqueMetadatas[contractAddress]; ok {
					metadata, err = handler(i, uriReplaced, contractAddress, tokenID)
					if err != nil {
						logrus.WithError(err).WithField("uri", uriReplaced).Error("error getting metadata for token")
						atomic.AddUint64(&i.badURIs, 1)
					}
				} else {
					if uriReplaced != "" && uriReplaced != persist.InvalidTokenURI {
						ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
						metadata, err = rpc.GetMetadataFromURI(ctx, uriReplaced, i.ipfsClient)
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
							atomic.AddUint64(&i.badURIs, 1)
						}
						cancel()
					}
				}
				if len(metadata) > 0 {
					go func() {
						defer wg.Done()
						metadatas <- tokenMetadata{key, metadata}
					}()
				} else {
					wg.Done()
				}

				wg.Wait()
				logrus.WithFields(logrus.Fields{"duration": time.Since(initial)}).Debugf("Processed transfer %s to %s and from %s ", key, to, from)
			}()
		}

	}

}

func (i *Indexer) processTokens(uris <-chan tokenURI, metadatas <-chan tokenMetadata, owners <-chan ownerAtBlock, previousOwners <-chan ownerAtBlock, balances <-chan tokenBalances) {

	wg := &sync.WaitGroup{}
	wg.Add(5)
	ownersMap := map[tokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[tokenIdentifiers][]ownerAtBlock{}
	balancesMap := map[tokenIdentifiers]map[persist.Address]balanceAtBlock{}
	metadatasMap := map[tokenIdentifiers]tokenMetadata{}
	urisMap := map[tokenIdentifiers]tokenURI{}

	go receiveBalances(wg, balances, balancesMap, i.tokenRepo)
	go receiveOwners(wg, owners, ownersMap, i.tokenRepo)
	go receiveMetadatas(wg, metadatas, metadatasMap)
	go receiveURIs(wg, uris, urisMap)
	go receivePreviousOwners(wg, previousOwners, previousOwnersMap, i.tokenRepo)
	wg.Wait()

	logrus.Info("Done recieving field data, convering fields into tokens...")

	tokens := i.storedDataToTokens(ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap)
	if tokens == nil || len(tokens) == 0 {
		logrus.Info("No tokens to process")
		return
	}

	logrus.Info("Created tokens to insert into database...")

	timeout := (time.Minute * time.Duration(len(tokens)/100)) + (time.Minute * 2)
	logrus.Info("Upserting tokens and contracts with a timeout of ", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := upsertTokensAndContracts(ctx, tokens, i.tokenRepo, i.contractRepo, i.ethClient)
	if err != nil {
		logrus.WithError(err).Error("error upserting tokens and contracts")
		panic(err)
	}

}

func receiveURIs(wg *sync.WaitGroup, uris <-chan tokenURI, uriMap map[tokenIdentifiers]tokenURI) {
	defer wg.Done()

	for uri := range uris {
		uriMap[uri.ti] = uri
	}
}

func receiveMetadatas(wg *sync.WaitGroup, metadatas <-chan tokenMetadata, metaMap map[tokenIdentifiers]tokenMetadata) {
	defer wg.Done()

	for meta := range metadatas {
		metaMap[meta.ti] = meta
	}
}

func receivePreviousOwners(wg *sync.WaitGroup, prevOwners <-chan ownerAtBlock, prevOwnersMap map[tokenIdentifiers][]ownerAtBlock, tokenRepo persist.TokenRepository) {
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

func receiveBalances(wg *sync.WaitGroup, balanceChan <-chan tokenBalances, balances map[tokenIdentifiers]map[persist.Address]balanceAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for balance := range balanceChan {

		balanceMap, ok := balances[balance.ti]
		if !ok {
			balanceMap = make(map[persist.Address]balanceAtBlock)
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

		balances[balance.ti] = balanceMap

	}
}

func receiveOwners(wg *sync.WaitGroup, ownersChan <-chan ownerAtBlock, owners map[tokenIdentifiers]ownerAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for owner := range ownersChan {
		owners[owner.ti] = owner
	}
}

func (i *Indexer) storedDataToTokens(owners map[tokenIdentifiers]ownerAtBlock, previousOwners map[tokenIdentifiers][]ownerAtBlock, balances map[tokenIdentifiers]map[persist.Address]balanceAtBlock, metadatas map[tokenIdentifiers]tokenMetadata, uris map[tokenIdentifiers]tokenURI) []persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v)
	}
	result := make([]persist.Token, len(owners)+totalBalances)
	j := 0

	for k, v := range owners {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			panic(err)
		}
		previousOwnerAddresses := make([]persist.AddressAtBlock, len(previousOwners[k]))
		for i, w := range previousOwners[k] {
			previousOwnerAddresses[i] = persist.AddressAtBlock{Address: w.owner, Block: w.block}
		}
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
		t := persist.Token{
			TokenID:          tokenID,
			ContractAddress:  contractAddress,
			OwnerAddress:     v.owner,
			Quantity:         persist.HexString("1"),
			Name:             persist.NullString(name),
			Description:      persist.NullString(description),
			OwnershipHistory: previousOwnerAddresses,
			TokenType:        persist.TokenTypeERC721,
			TokenMetadata:    metadata.md,
			TokenURI:         uri.uri,
			Chain:            i.chain,
			BlockNumber:      v.block,
		}
		if metadata.md != nil && len(metadata.md) > 0 {
			if _, ok := metadata.md["error"]; !ok {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
				exists, err := i.userRepo.ExistsByAddress(ctx, t.OwnerAddress)
				cancel()
				if err != nil {
					logrus.WithError(err).Error("error checking if user exists")
					panic(err)
				} else if exists {
					ctx, cancel = context.WithTimeout(context.Background(), time.Second*30)
					med, err := media.MakePreviewsForMetadata(ctx, metadata.md, contractAddress, tokenID, t.TokenURI, i.ipfsClient, i.storageClient)
					cancel()
					if err != nil {
						logrus.WithError(err).Error("error making previews")
					} else {
						t.Media = med
					}
				}
			}
		}
		result[j] = t
		j++
	}
	for k, v := range balances {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			panic(err)
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

		var m persist.Media
		hasMetadata := metadata.md != nil && len(metadata.md) > 0

		for addr, balance := range v {
			if hasMetadata && m.MediaType == "" && m.MediaURL == "" {
				if _, ok := metadata.md["error"]; !ok {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
					exists, err := i.userRepo.ExistsByAddress(ctx, addr)
					cancel()
					if err != nil {
						logrus.WithError(err).Error("error checking if user exists")
						panic(err)
					} else if exists {
						aeCtx := appengine.BackgroundContext()

						ctx, cancel = context.WithTimeout(aeCtx, time.Second*30)
						med, err := media.MakePreviewsForMetadata(ctx, metadata.md, contractAddress, tokenID, uri.uri, i.ipfsClient, i.storageClient)
						cancel()
						if err != nil {
							logrus.WithError(err).Error("error making previews")
						} else {
							m = med
						}

					}
				}
			}
			t := persist.Token{
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				OwnerAddress:    addr,
				Quantity:        persist.HexString(balance.amnt.Text(16)),
				TokenType:       persist.TokenTypeERC1155,
				TokenMetadata:   metadata.md,
				TokenURI:        uri.uri,
				Name:            persist.NullString(name),
				Description:     persist.NullString(description),
				Chain:           i.chain,
				BlockNumber:     balance.block,
				Media:           m,
			}
			result[j] = t
			j++
		}
	}

	return result
}

func upsertTokensAndContracts(ctx context.Context, t []persist.Token, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client) error {

	now := time.Now()
	logrus.Infof("Upserting %d tokens", len(t))
	if err := tokenRepo.BulkUpsert(ctx, t); err != nil {
		return fmt.Errorf("err upserting %d tokens: %s", len(t), err.Error())
	}
	logrus.Infof("Upserted %d tokens in %v time", len(t), time.Since(now))

	contracts := make(map[persist.Address]bool)

	nextNow := time.Now()

	toUpsert := make([]persist.Contract, 0, len(t))
	for _, token := range t {
		if contracts[token.ContractAddress] {
			continue
		}
		contract := handleContract(ethClient, token.ContractAddress, token.BlockNumber)
		logrus.Infof("Processing contract %s", contract.Address)
		toUpsert = append(toUpsert, contract)

		contracts[token.ContractAddress] = true
	}

	logrus.Infof("Processed %d contracts in %v time", len(toUpsert), time.Since(nextNow))

	finalNow := time.Now()
	err := contractRepo.BulkUpsert(ctx, toUpsert)
	if err != nil {
		return fmt.Errorf("err upserting contracts: %s", err.Error())
	}
	logrus.Infof("Upserted %d contracts in %v time", len(toUpsert), time.Since(finalNow))
	return nil
}

func handleContract(ethClient *ethclient.Client, contractAddress persist.Address, lastSyncedBlock persist.BlockNumber) persist.Contract {
	c := persist.Contract{
		Address:     contractAddress,
		LatestBlock: lastSyncedBlock,
	}
	cMetadata, err := rpc.GetTokenContractMetadata(contractAddress, ethClient)
	if err != nil {
		// TODO figure out what type of error this is
		logrus.WithError(err).WithField("address", contractAddress).Error("error getting contract metadata")
	} else {
		c.Name = persist.NullString(cMetadata.Name)
		c.Symbol = persist.NullString(cMetadata.Symbol)
	}
	return c
}

func (i *Indexer) subscribeNewLogs(lastSyncedBlock persist.BlockNumber, transfers chan<- []transfersAtBlock, subscriptions chan types.Log, topics [][]common.Hash) {

	defer close(transfers)

	sub, err := i.ethClient.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: lastSyncedBlock.BigInt(),
		Topics:    topics,
	}, subscriptions)
	if err != nil {
		panic(fmt.Sprintf("error subscribing to logs: %s", err))
	}
	for {
		select {
		case log := <-subscriptions:
			logrus.Infof("Got log at: %d", log.BlockNumber)
			lastSyncedBlock = persist.BlockNumber(log.BlockNumber)
			ts := logsToTransfers([]types.Log{log}, i.ethClient)
			transfers <- transfersToTransfersAtBlock(ts)
		case err := <-sub.Err():
			panic(fmt.Sprintf("error in log subscription: %s", err))
		}
	}
}

func getUniqueMetadataHandlers() uniqueMetadatas {
	return uniqueMetadatas{
		persist.Address("0xd4e4078ca3495DE5B1d4dB434BEbc5a986197782"): autoglyphs,
		persist.Address("0x60F3680350F65Beb2752788cB48aBFCE84a4759E"): colorglyphs,
		persist.Address("0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e"): ens,
	}
}

func findFirstFieldFromMetadata(metadata persist.TokenMetadata, fields ...string) interface{} {

	for _, field := range fields {
		if val := util.GetValueFromMapUnsafe(metadata, field, util.DefaultSearchDepth); val != nil {
			return val
		}
	}
	return nil
}

// a function that removes the left padded zeros from a large hex string
func removeLeftPaddedZeros(hex string) string {
	if strings.HasPrefix(hex, "0x") {
		hex = hex[2:]
	}
	for i := 0; i < len(hex); i++ {
		if hex[i] != '0' {
			return "0x" + hex[i:]
		}
	}
	return "0x" + hex
}

func makeKeyForToken(contractAddress persist.Address, tokenID persist.TokenID) tokenIdentifiers {
	return tokenIdentifiers(fmt.Sprintf("%s_%s", contractAddress, tokenID))
}

func parseTokenIdentifiers(key tokenIdentifiers) (persist.Address, persist.TokenID, error) {
	parts := strings.Split(string(key), "_")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid key: %s", key)
	}
	return persist.Address(parts[0]), persist.TokenID(parts[1]), nil
}

func transfersToTransfersAtBlock(transfers []rpc.Transfer) []transfersAtBlock {
	transfersMap := map[persist.BlockNumber]transfersAtBlock{}

	for _, transfer := range transfers {
		if _, ok := transfersMap[transfer.BlockNumber]; !ok {
			transfers := make([]rpc.Transfer, 0, 10)
			transfers = append(transfers, transfer)
			transfersMap[transfer.BlockNumber] = transfersAtBlock{
				block:     transfer.BlockNumber,
				transfers: transfers,
			}
		} else {
			tab := transfersMap[transfer.BlockNumber]
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
