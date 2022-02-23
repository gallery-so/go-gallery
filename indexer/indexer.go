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

var defaultStartingBlock persist.BlockNumber = 5000000

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

type tokenMetadata struct {
	ti persist.TokenIdentifiers
	md persist.TokenMetadata
}

type tokenBalances struct {
	ti      persist.TokenIdentifiers
	from    persist.Address
	to      persist.Address
	fromAmt *big.Int
	toAmt   *big.Int
	block   persist.BlockNumber
}

type tokenURI struct {
	ti  persist.TokenIdentifiers
	uri persist.TokenURI
}

type transfersAtBlock struct {
	block     persist.BlockNumber
	transfers []rpc.Transfer
}

type ownerAtBlock struct {
	ti    persist.TokenIdentifiers
	owner persist.Address
	block persist.BlockNumber
}

type balanceAtBlock struct {
	ti    persist.TokenIdentifiers
	block persist.BlockNumber
	amnt  *big.Int
}

type tokenMedia struct {
	ti    persist.TokenIdentifiers
	media persist.Media
}

// Indexer is the indexer for the blockchain that uses JSON RPC to scan through logs and process them
// into a format used by the application
type Indexer struct {
	ethClient     *ethclient.Client
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	storageClient *storage.Client
	tokenRepo     persist.TokenRepository
	contractRepo  persist.ContractRepository
	collRepo      persist.CollectionTokenRepository
	userRepo      persist.UserRepository
	contractDBMu  *sync.Mutex
	tokenDBMu     *sync.Mutex

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64

	isListening bool

	uniqueMetadatas uniqueMetadatas
}

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens
func NewIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, userRepo persist.UserRepository, collRepo persist.CollectionTokenRepository, pChain persist.Chain, pEvents []eventHash, statsFileName string) *Indexer {
	mostRecentBlockUint64, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	return &Indexer{

		ethClient:     ethClient,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveClient,
		storageClient: storageClient,
		tokenRepo:     tokenRepo,
		contractRepo:  contractRepo,
		collRepo:      collRepo,
		userRepo:      userRepo,
		contractDBMu:  &sync.Mutex{},
		tokenDBMu:     &sync.Mutex{},

		chain: pChain,

		eventHashes:     pEvents,
		mostRecentBlock: mostRecentBlockUint64,
		uniqueMetadatas: getUniqueMetadataHandlers(),
	}
}

// INITIALIZATION FUNCS ---------------------------------------------------------

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

	wp := workerpool.New(8)

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
		if wp.WaitingQueueSize() > 25 {
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
	i.isListening = false
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
	i.isListening = true
	uris := make(chan tokenURI)
	metadatas := make(chan tokenMetadata)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	medias := make(chan tokenMedia)
	transfers := make(chan []transfersAtBlock)
	subscriptions := make(chan types.Log)
	go i.subscribeNewLogs(start, transfers, subscriptions, topics)
	go i.processNewTransfers(transfers, uris, metadatas, owners, previousOwners, balances, medias)
	i.processNewTokens(uris, metadatas, owners, previousOwners, balances, medias)
}

func (i *Indexer) listenForNewBlocks() {
	for {
		finalBlockUint, err := i.ethClient.BlockNumber(context.Background())
		if err != nil {
			panic(fmt.Sprintf("error getting block number: %s", err))
		}
		atomic.StoreUint64(&i.mostRecentBlock, finalBlockUint)
		logrus.Debugf("final block number: %v", finalBlockUint)
		time.Sleep(time.Minute)
	}
}

// LOGS FUNCS ---------------------------------------------------------------

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

	logrus.Debugf("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

	transfers := logsToTransfers(logsTo, i.ethClient)

	logrus.Debugf("Processed %d logs into %d transfers", len(logsTo), len(transfers))

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
			lastSyncedBlock = persist.BlockNumber(log.BlockNumber)
			ts := logsToTransfers([]types.Log{log}, i.ethClient)
			transfers <- transfersToTransfersAtBlock(ts)
		case err := <-sub.Err():
			panic(fmt.Sprintf("error in log subscription: %s", err))
		}
	}
}

// TRANSFERS FUNCS -------------------------------------------------------------

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

		logrus.Debugf("Processing %d transfers", len(transfers))
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

func (i *Indexer) processNewTransfers(incomingTransfers <-chan []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances, medias chan<- tokenMedia) {
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
		it := make([]transfersAtBlock, len(transfers))
		copy(it, transfers)
		wp.Submit(func() {
			submit := it
			processNewTransfers(i, submit, uris, metadatas, owners, previousOwners, balances, medias)
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

				contractAddress := persist.Address(transfer.ContractAddress.String())
				from := transfer.From
				to := transfer.To
				tokenID := transfer.TokenID

				key := persist.NewTokenIdentifiers(contractAddress, tokenID)
				// logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

				findRequiredTokenFields(i, transfer, key, to, from, contractAddress, tokenID, balances, uris, metadatas, owners, previousOwners)

				logrus.WithFields(logrus.Fields{"duration": time.Since(initial)}).Debugf("Processed transfer %s to %s and from %s ", key, to, from)
			}()
		}

	}

}

func processNewTransfers(i *Indexer, transfers []transfersAtBlock, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances, medias chan<- tokenMedia) {

	for _, transferAtBlock := range transfers {
		for _, transfer := range transferAtBlock.transfers {
			initial := time.Now()
			func() {

				contractAddress := persist.Address(transfer.ContractAddress.String())
				from := transfer.From
				to := transfer.To
				tokenID := transfer.TokenID

				key := persist.NewTokenIdentifiers(contractAddress, tokenID)
				// logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

				_, _, bals, tokenURI, metadata := findRequiredTokenFields(i, transfer, key, to, from, contractAddress, tokenID, balances, uris, metadatas, owners, previousOwners)

				findOptionalFields(i, key, to, from, tokenURI, metadata, medias)

				runTransferSideEffects(i, contractAddress, tokenID, to, from, bals)

				logrus.WithFields(logrus.Fields{"duration": time.Since(initial)}).Debugf("Processed transfer %s to %s and from %s ", key, to, from)
			}()
		}

	}

}

func findRequiredTokenFields(i *Indexer, transfer rpc.Transfer, key persist.TokenIdentifiers, to persist.Address, from persist.Address, contractAddress persist.Address, tokenID persist.TokenID, balances chan<- tokenBalances, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock) (ownerAtBlock, ownerAtBlock, tokenBalances, persist.TokenURI, persist.TokenMetadata) {
	wg := &sync.WaitGroup{}
	var curOwner, prevOwner ownerAtBlock
	var bals tokenBalances
	switch persist.TokenType(transfer.TokenType) {
	case persist.TokenTypeERC721:

		wg.Add(4)

		curOwner := ownerAtBlock{key, to, transfer.BlockNumber}
		go func() {
			defer wg.Done()
			owners <- curOwner
		}()

		prevOwner := ownerAtBlock{key, from, transfer.BlockNumber}
		go func() {
			defer wg.Done()
			previousOwners <- prevOwner
		}()

	case persist.TokenTypeERC1155:
		wg.Add(3)

		b, err := getBalances(contractAddress, from, tokenID, key, transfer.BlockNumber, to, i.ethClient)
		if err != nil {
			logrus.WithError(err).Errorf("error getting balance of %s for %s", from, key)
			storeErr(err, "ERR-BALANCE", from, key, transfer.BlockNumber, i.storageClient)
		}
		bals = b
		go func() {
			defer wg.Done()
			balances <- bals
		}()

	default:
		panic("unknown token type")
	}

	uriReplaced := getURI(contractAddress, tokenID, transfer.TokenType, i.ethClient)

	metadata, uriReplaced := getMetadata(contractAddress, uriReplaced, tokenID, i.uniqueMetadatas, i.ipfsClient, i.arweaveClient)
	go func() {
		defer wg.Done()
		uris <- tokenURI{key, uriReplaced}
	}()
	if len(metadata) > 0 {
		go func() {
			defer wg.Done()
			metadatas <- tokenMetadata{key, metadata}
		}()
	} else {
		wg.Done()
	}
	wg.Wait()
	return curOwner, prevOwner, bals, uriReplaced, metadata
}

func findOptionalFields(i *Indexer, key persist.TokenIdentifiers, to, from persist.Address, tokenURI persist.TokenURI, metadata persist.TokenMetadata, medias chan<- tokenMedia) tokenMedia {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := i.userRepo.GetByAddress(ctx, to)
	if err != nil {
		logrus.WithError(err).Errorf("error getting user for %s", to)
		return tokenMedia{}
	}

	contractAddress, tokenID, err := key.GetParts()
	if err != nil {
		logrus.WithError(err).Errorf("error getting parts of %s", key)
		return tokenMedia{}
	}

	med, err := media.MakePreviewsForMetadata(ctx, metadata, contractAddress, tokenID, tokenURI, i.ipfsClient, i.arweaveClient, i.storageClient)
	if err != nil {
		logrus.WithError(err).Errorf("error making previews for %s", key)
		return tokenMedia{}
	}

	res := tokenMedia{ti: key, media: med}
	medias <- res
	return res
}

func runTransferSideEffects(i *Indexer, contractAddress persist.Address, tokenID persist.TokenID, to, from persist.Address, bals tokenBalances) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	user, err := i.userRepo.GetByAddress(ctx, from)
	if err != nil {
		return
	}
	if bals.fromAmt != nil {
		if bals.fromAmt.Cmp(bigZero) != 0 {
			return
		}
	}
	err = updateCollections(ctx, contractAddress, tokenID, user.ID, i.collRepo)
	if err != nil {
		logrus.WithError(err).Errorf("error updating collections for %s: %s", persist.NewTokenIdentifiers(contractAddress, tokenID), err)
		return
	}
}

func updateCollections(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, userID persist.DBID, collRepo persist.CollectionTokenRepository) error {
	colls, err := collRepo.GetByUserID(ctx, userID)
	if err != nil {

		return fmt.Errorf("error getting collections for %s", userID)
	}

	update := map[persist.DBID]persist.CollectionToken{}
	for _, coll := range colls {
		didUpdate := false
		for i, nft := range coll.NFTs {
			if nft.ContractAddress.String() == contractAddress.String() && nft.TokenID.String() == tokenID.String() {
				coll.NFTs = append(coll.NFTs[:i], coll.NFTs[i+1:]...)
				didUpdate = true
			}
		}
		if didUpdate {
			update[coll.ID] = coll
		}
	}

	for id, coll := range update {
		nftIDs := make([]persist.DBID, len(coll.NFTs))
		for i, nft := range coll.NFTs {
			nftIDs[i] = nft.ID
		}
		err = collRepo.UpdateNFTsUnsafe(ctx, id, persist.CollectionTokenUpdateNftsInput{NFTs: nftIDs})
		if err != nil {
			return fmt.Errorf("error updating collection %s", id)
		}
	}
	return nil
}

func getBalances(contractAddress persist.Address, from persist.Address, tokenID persist.TokenID, key persist.TokenIdentifiers, blockNumber persist.BlockNumber, to persist.Address, ethClient *ethclient.Client) (tokenBalances, error) {
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

func getURI(contractAddress persist.Address, tokenID persist.TokenID, tokenType persist.TokenType, ethClient *ethclient.Client) persist.TokenURI {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	u, err := rpc.GetTokenURI(ctx, tokenType, contractAddress, tokenID, ethClient)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"id": tokenID, "contract": contractAddress}).Error("error getting URI for token")
		if strings.Contains(err.Error(), "execution reverted") {
			u = persist.InvalidTokenURI
		}
	}
	cancel()

	uriReplaced := u.ReplaceID(tokenID)
	return uriReplaced
}

func getMetadata(contractAddress persist.Address, uriReplaced persist.TokenURI, tokenID persist.TokenID, um uniqueMetadatas, ipfsClient *shell.Shell, arweaveClient *goar.Client) (persist.TokenMetadata, persist.TokenURI) {
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

func (i *Indexer) processTokens(uris <-chan tokenURI, metadatas <-chan tokenMetadata, owners <-chan ownerAtBlock, previousOwners <-chan ownerAtBlock, balances <-chan tokenBalances) {

	wg := &sync.WaitGroup{}
	wg.Add(5)
	ownersMap := map[persist.TokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[persist.TokenIdentifiers][]ownerAtBlock{}
	balancesMap := map[persist.TokenIdentifiers]map[persist.Address]balanceAtBlock{}
	metadatasMap := map[persist.TokenIdentifiers]tokenMetadata{}
	urisMap := map[persist.TokenIdentifiers]tokenURI{}

	go receiveBalances(wg, balances, balancesMap, i.tokenRepo)
	go receiveOwners(wg, owners, ownersMap, i.tokenRepo)
	go receiveMetadatas(wg, metadatas, metadatasMap)
	go receiveURIs(wg, uris, urisMap)
	go receivePreviousOwners(wg, previousOwners, previousOwnersMap, i.tokenRepo)
	wg.Wait()

	logrus.Info("Done recieving field data, converting fields into tokens...")

	createTokens(i, ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap, map[persist.TokenIdentifiers]tokenMedia{})
}

func (i *Indexer) processNewTokens(uris <-chan tokenURI, metadatas <-chan tokenMetadata, owners <-chan ownerAtBlock, previousOwners <-chan ownerAtBlock, balances <-chan tokenBalances, medias <-chan tokenMedia) {

	wg := &sync.WaitGroup{}
	wg.Add(6)
	ownersMap := map[persist.TokenIdentifiers]ownerAtBlock{}
	previousOwnersMap := map[persist.TokenIdentifiers][]ownerAtBlock{}
	balancesMap := map[persist.TokenIdentifiers]map[persist.Address]balanceAtBlock{}
	metadatasMap := map[persist.TokenIdentifiers]tokenMetadata{}
	urisMap := map[persist.TokenIdentifiers]tokenURI{}
	mediasMap := map[persist.TokenIdentifiers]tokenMedia{}

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

func createTokens(i *Indexer, ownersMap map[persist.TokenIdentifiers]ownerAtBlock, previousOwnersMap map[persist.TokenIdentifiers][]ownerAtBlock, balancesMap map[persist.TokenIdentifiers]map[persist.Address]balanceAtBlock, metadatasMap map[persist.TokenIdentifiers]tokenMetadata, urisMap map[persist.TokenIdentifiers]tokenURI, mediasMap map[persist.TokenIdentifiers]tokenMedia) {
	tokens := i.fieldMapsToTokens(ownersMap, previousOwnersMap, balancesMap, metadatasMap, urisMap, mediasMap)
	if tokens == nil || len(tokens) == 0 {
		logrus.Info("No tokens to process")
		return
	}

	logrus.Info("Created tokens to insert into database...")

	timeout := (time.Minute * time.Duration(len(tokens)/100)) + (time.Minute * 2)
	logrus.Info("Upserting tokens and contracts with a timeout of ", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := upsertTokensAndContracts(ctx, tokens, i.tokenRepo, i.contractRepo, i.ethClient, i.tokenDBMu, i.contractDBMu)
	if err != nil {
		logrus.WithError(err).Error("error upserting tokens and contracts")
		panic(err)
	}

}

func receiveURIs(wg *sync.WaitGroup, uris <-chan tokenURI, uriMap map[persist.TokenIdentifiers]tokenURI) {
	defer wg.Done()

	for uri := range uris {
		uriMap[uri.ti] = uri
	}
}

func receiveMetadatas(wg *sync.WaitGroup, metadatas <-chan tokenMetadata, metaMap map[persist.TokenIdentifiers]tokenMetadata) {
	defer wg.Done()

	for meta := range metadatas {
		metaMap[meta.ti] = meta
	}
}

func receiveMedias(wg *sync.WaitGroup, medias <-chan tokenMedia, metaMap map[persist.TokenIdentifiers]tokenMedia) {
	defer wg.Done()

	for media := range medias {
		metaMap[media.ti] = media
	}
}

func receivePreviousOwners(wg *sync.WaitGroup, prevOwners <-chan ownerAtBlock, prevOwnersMap map[persist.TokenIdentifiers][]ownerAtBlock, tokenRepo persist.TokenRepository) {
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

func receiveBalances(wg *sync.WaitGroup, balanceChan <-chan tokenBalances, balances map[persist.TokenIdentifiers]map[persist.Address]balanceAtBlock, tokenRepo persist.TokenRepository) {
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

func receiveOwners(wg *sync.WaitGroup, ownersChan <-chan ownerAtBlock, owners map[persist.TokenIdentifiers]ownerAtBlock, tokenRepo persist.TokenRepository) {
	defer wg.Done()
	for owner := range ownersChan {
		owners[owner.ti] = owner
	}
}

func (i *Indexer) fieldMapsToTokens(owners map[persist.TokenIdentifiers]ownerAtBlock, previousOwners map[persist.TokenIdentifiers][]ownerAtBlock, balances map[persist.TokenIdentifiers]map[persist.Address]balanceAtBlock, metadatas map[persist.TokenIdentifiers]tokenMetadata, uris map[persist.TokenIdentifiers]tokenURI, medias map[persist.TokenIdentifiers]tokenMedia) []persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v)
	}
	result := make([]persist.Token, len(owners)+totalBalances)
	j := 0

	for k, v := range owners {
		contractAddress, tokenID, err := k.GetParts()
		if err != nil {
			panic(err)
		}
		previousOwnerAddresses := make([]persist.AddressAtBlock, len(previousOwners[k]))
		for i, w := range previousOwners[k] {
			previousOwnerAddresses[i] = persist.AddressAtBlock{Address: w.owner, Block: w.block}
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

		result[j] = t
		j++
		delete(owners, k)
	}
	for k, v := range balances {
		contractAddress, tokenID, err := k.GetParts()
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
			result[j] = t
			j++
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

	contracts := make(map[persist.Address]bool)

	nextNow := time.Now()

	toUpsert := make([]persist.Contract, 0, len(t))
	for _, token := range t {
		if contracts[token.ContractAddress] {
			continue
		}
		contract := fillContractFields(ethClient, token.ContractAddress, token.BlockNumber)
		logrus.Debugf("Processing contract %s", contract.Address)
		toUpsert = append(toUpsert, contract)

		contracts[token.ContractAddress] = true
	}

	logrus.Debugf("Processed %d contracts in %v time", len(toUpsert), time.Since(nextNow))

	finalNow := time.Now()
	return func() error {
		contractMu.Lock()
		defer contractMu.Unlock()
		err = contractRepo.BulkUpsert(ctx, toUpsert)
		if err != nil {
			return fmt.Errorf("err upserting contracts: %s", err.Error())
		}
		logrus.Debugf("Upserted %d contracts in %v time", len(toUpsert), time.Since(finalNow))
		return nil
	}()
}

func fillContractFields(ethClient *ethclient.Client, contractAddress persist.Address, lastSyncedBlock persist.BlockNumber) persist.Contract {
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

// HELPER FUNCS ---------------------------------------------------------------

func getUniqueMetadataHandlers() uniqueMetadatas {
	return uniqueMetadatas{
		persist.Address("0xd4e4078ca3495de5b1d4db434bebc5a986197782"): autoglyphs,
		persist.Address("0x60f3680350f65beb2752788cb48abfce84a4759e"): colorglyphs,
		persist.Address("0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85"): ens,
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

func storeErr(err error, prefix string, from persist.Address, key persist.TokenIdentifiers, atBlock persist.BlockNumber, storageClient *storage.Client) {
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
