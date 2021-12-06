package indexer

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var defaultStartingBlock persist.BlockNumber = 11300000

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

// type tokenBalanceChange struct {
// 	ti    tokenIdentifiers
// 	from  persist.Address
// 	to    persist.Address
// 	amt   *big.Int
// 	block persist.BlockNumber
// }

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
	ethClient    *ethclient.Client
	ipfsClient   *shell.Shell
	tokenRepo    persist.TokenRepository
	contractRepo persist.ContractRepository

	chain persist.Chain

	eventHashes []eventHash

	mostRecentBlock uint64

	badURIs uint64

	uniqueMetadatas uniqueMetadatas
}

// NewIndexer sets up an indexer for retrieving the specified events that will process tokens
func NewIndexer(ethClient *ethclient.Client, ipfsClient *shell.Shell, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, pChain persist.Chain, pEvents []eventHash, statsFileName string) *Indexer {
	mostRecentBlockUint64, err := ethClient.BlockNumber(context.Background())
	if err != nil {
		panic(err)
	}

	return &Indexer{

		ethClient:    ethClient,
		ipfsClient:   ipfsClient,
		tokenRepo:    tokenRepo,
		contractRepo: contractRepo,

		chain: pChain,

		eventHashes:     pEvents,
		mostRecentBlock: mostRecentBlockUint64,
		uniqueMetadatas: getUniqueMetadataHandlers(),
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
	lastSyncedBlock := defaultStartingBlock
	recentDBBlock, err := i.tokenRepo.MostRecentBlock(ctx)
	if err == nil && recentDBBlock > defaultStartingBlock {
		lastSyncedBlock = recentDBBlock
	}
	cancel()

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
		if wp.WaitingQueueSize() > 500 {
			wp.SubmitWait(toQueue)
		} else {
			wp.Submit(toQueue)
		}
		lastSyncedBlock += blocksPerLogsCall
	}
	wp.StopWait()
	logrus.Info("Finished processing old logs, subscribing to new logs...")
	time.Sleep(time.Second)
	i.startNewBlocksPipeline(lastSyncedBlock, topics)
}

func (i *Indexer) startPipeline(start persist.BlockNumber, topics [][]common.Hash) {
	uris := make(chan tokenURI)
	metadatas := make(chan tokenMetadata)
	balances := make(chan tokenBalances)
	owners := make(chan ownerAtBlock)
	previousOwners := make(chan ownerAtBlock)
	transfers := make(chan []*transfer)

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
	transfers := make(chan []*transfer)
	subscriptions := make(chan types.Log)
	go i.subscribeNewLogs(start, transfers, subscriptions, topics)
	go i.processTransfers(transfers, uris, metadatas, owners, previousOwners, balances)
	i.processTokens(uris, metadatas, owners, previousOwners, balances)
}

func (i *Indexer) processLogs(transfersChan chan<- []*transfer, startingBlock persist.BlockNumber, topics [][]common.Hash) {
	defer close(transfersChan)

	curBlock := startingBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(blocksPerLogsCall)))

	logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	logsTo, err := i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: curBlock,
		ToBlock:   nextBlock,
		Topics:    topics,
	})
	if err != nil {
		logrus.WithError(err).Error("Error getting logs")
		return
	}

	logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

	transfers := logsToTransfers(logsTo, i.ethClient)

	logrus.Infof("Processed %d logs into %d transfers", len(logsTo), len(transfers))

	if len(transfers) > 0 && transfers != nil {
		logrus.Infof("Sending %d total transfers to transfers channel", len(transfers))
		interval := len(transfers) / 4
		if interval == 0 {
			interval = 1
		}
		for j := 0; j < len(transfers); j += interval {
			to := j + interval
			if to > len(transfers) {
				to = len(transfers)
			}
			transfersChan <- transfers[j:to]
		}

	}
	logrus.Info("Finished processing logs, closing transfers channel...")
}

func logsToTransfers(pLogs []types.Log, ethClient *ethclient.Client) []*transfer {
	result := make([]*transfer, 0, len(pLogs))
	for _, pLog := range pLogs {
		switch {
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferEventHash)):

			if len(pLog.Topics) < 4 {
				continue
			}

			erc20, err := contracts.NewIERC20Caller(pLog.Address, ethClient)
			if err == nil {
				_, err := erc20.Allowance(&bind.CallOpts{}, common.HexToAddress(pLog.Topics[1].Hex()), common.HexToAddress(pLog.Topics[2].Hex()))
				if err == nil {
					continue
				}
			}

			result = append(result, &transfer{
				from:            persist.Address(pLog.Topics[1].Hex()),
				to:              persist.Address(pLog.Topics[2].Hex()),
				tokenID:         persist.TokenID(pLog.Topics[3].Hex()),
				amount:          1,
				blockNumber:     persist.BlockNumber(pLog.BlockNumber),
				contractAddress: persist.Address(pLog.Address.Hex()),
				tokenType:       persist.TokenTypeERC721,
			})
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferSingleEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}

			result = append(result, &transfer{
				from:            persist.Address(pLog.Topics[2].Hex()),
				to:              persist.Address(pLog.Topics[3].Hex()),
				tokenID:         persist.TokenID(common.BytesToHash(pLog.Data[:len(pLog.Data)/2]).Hex()),
				amount:          common.BytesToHash(pLog.Data[len(pLog.Data)/2:]).Big().Uint64(),
				blockNumber:     persist.BlockNumber(pLog.BlockNumber),
				contractAddress: persist.Address(pLog.Address.Hex()),
				tokenType:       persist.TokenTypeERC1155,
			})

		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferBatchEventHash)):
			if len(pLog.Topics) < 4 {
				continue
			}
			from := persist.Address(pLog.Topics[2].Hex())
			to := persist.Address(pLog.Topics[3].Hex())
			amountOffset := len(pLog.Data) / 2
			total := amountOffset / 64
			this := make([]*transfer, total)

			for j := 0; j < total; j++ {
				this[j] = &transfer{
					from:            from,
					to:              to,
					tokenID:         persist.TokenID(common.BytesToHash(pLog.Data[j*64 : (j+1)*64]).Hex()),
					amount:          common.BytesToHash(pLog.Data[(amountOffset)+(j*64) : (amountOffset)+((j+1)*64)]).Big().Uint64(),
					contractAddress: persist.Address(pLog.Address.Hex()),
					tokenType:       persist.TokenTypeERC1155,
				}
			}
			result = append(result, this...)
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

func (i *Indexer) processTransfers(incomingTransfers <-chan []*transfer, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances) {
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
		it := make([]*transfer, len(transfers))
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

func processTransfers(i *Indexer, transfers []*transfer, uris chan<- tokenURI, metadatas chan<- tokenMetadata, owners chan<- ownerAtBlock, previousOwners chan<- ownerAtBlock, balances chan<- tokenBalances) {

	for _, transfer := range transfers {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()

			wg := &sync.WaitGroup{}
			contractAddress := transfer.contractAddress
			from := transfer.from
			to := transfer.to
			tokenID := transfer.tokenID

			key := makeKeyForToken(contractAddress, tokenID)
			logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

			switch persist.TokenType(transfer.tokenType) {
			case persist.TokenTypeERC721:

				wg.Add(4)

				go func() {
					defer wg.Done()
					owners <- ownerAtBlock{key, to, transfer.blockNumber}
				}()

				go func() {
					defer wg.Done()
					previousOwners <- ownerAtBlock{key, from, transfer.blockNumber}
				}()

			case persist.TokenTypeERC1155:
				wg.Add(3)

				go func() {
					defer wg.Done()
					ierc1155, err := contracts.NewIERC1155Caller(contractAddress.Address(), i.ethClient)
					if err != nil {
						logrus.WithError(err).Errorf("error creating IERC1155 contract caller for %s", contractAddress)
						return
					}
					fromBalance, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, from.Address(), tokenID.BigInt())
					if err != nil {
						logrus.WithError(err).Errorf("error getting balance of %s for %s", from, key)
						return
					}
					toBalance, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, to.Address(), tokenID.BigInt())
					if err != nil {
						logrus.WithError(err).Errorf("error getting balance of %s for %s", to, key)
						return
					}

					balances <- tokenBalances{key, from, to, fromBalance, toBalance, transfer.blockNumber}
				}()

			default:
				panic("unknown token type")
			}

			u, err := GetTokenURI(ctx, transfer.tokenType, contractAddress, tokenID, i.ethClient)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{"id": tokenID, "contract": contractAddress}).Error("error getting URI for ERC1155 token")
			}

			id, err := util.HexToBigInt(string(tokenID))
			if err != nil {
				panic(fmt.Sprintf("error converting tokenID to bigint: %s", err))
			}

			uriReplaced := persist.TokenURI(strings.ReplaceAll(u.String(), "{id}", id.String()))

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

				metadata, err = GetMetadataFromURI(uriReplaced, i.ipfsClient)
				if err != nil {
					logrus.WithError(err).WithField("uri", uriReplaced).Error("error getting metadata for token")
					atomic.AddUint64(&i.badURIs, 1)
				}
			}

			go func() {
				defer wg.Done()
				metadatas <- tokenMetadata{key, metadata}
			}()
			wg.Wait()
		}()
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

func (i *Indexer) storedDataToTokens(owners map[tokenIdentifiers]ownerAtBlock, previousOwners map[tokenIdentifiers][]ownerAtBlock, balances map[tokenIdentifiers]map[persist.Address]balanceAtBlock, metadatas map[tokenIdentifiers]tokenMetadata, uris map[tokenIdentifiers]tokenURI) []*persist.Token {
	totalBalances := 0
	for _, v := range balances {
		totalBalances += len(v)
	}
	result := make([]*persist.Token, len(owners)+totalBalances, len(owners)+totalBalances+len(metadatas)+len(uris))
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
		var name, description string

		if w, ok := findFirstFieldFromMetadata(metadata.md, "name").(string); ok {
			name = w
		}
		if w, ok := findFirstFieldFromMetadata(metadata.md, "description").(string); ok {
			description = w
		}

		uri := uris[k]
		result[j] = &persist.Token{
			TokenID:          tokenID,
			ContractAddress:  contractAddress,
			OwnerAddress:     v.owner,
			Quantity:         persist.HexString("1"),
			Name:             name,
			Description:      description,
			OwnershipHistoty: previousOwnerAddresses,
			TokenType:        persist.TokenTypeERC721,
			TokenMetadata:    metadata.md,
			TokenURI:         uri.uri,
			Chain:            i.chain,
			BlockNumber:      v.block,
		}
		if uri.uri != "" {
			delete(uris, k)
		}
		if metadata.md != nil && len(metadata.md) > 0 {
			delete(metadatas, k)
		}
		j++
	}
	for k, v := range balances {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			panic(err)
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
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				OwnerAddress:    addr,
				Quantity:        persist.HexString(balance.amnt.Text(16)),
				TokenType:       persist.TokenTypeERC1155,
				TokenMetadata:   metadata.md,
				TokenURI:        uri.uri,
				Name:            name,
				Description:     description,
				Chain:           i.chain,
				BlockNumber:     balance.block,
			}
			j++
		}
		if uri.uri != "" {
			delete(uris, k)
		}
		if metadata.md != nil && len(metadata.md) > 0 {
			delete(metadatas, k)
		}
	}

	for k, v := range uris {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			panic(err)
		}
		if v.uri != "" {
			result = append(result, &persist.Token{
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				TokenURI:        v.uri,
				Chain:           i.chain,
			})
		}
		delete(uris, k)
	}

	for k, v := range metadatas {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			panic(err)
		}
		if v.md != nil && len(v.md) > 0 {
			result = append(result, &persist.Token{
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				TokenMetadata:   v.md,
			})
		}
		delete(metadatas, k)
	}

	return result
}

func upsertTokensAndContracts(ctx context.Context, t []*persist.Token, tokenRepo persist.TokenRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client) error {

	now := time.Now()
	logrus.Infof("Upserting %d tokens", len(t))
	if err := tokenRepo.BulkUpsert(ctx, t); err != nil {
		return fmt.Errorf("err upserting %d tokens: %s", len(t), err.Error())
	}
	logrus.Infof("Upserted %d tokens in %v time", len(t), time.Since(now))

	contracts := make(map[persist.Address]bool)

	nextNow := time.Now()

	toUpsert := make([]*persist.Contract, 0, len(t))
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

func handleContract(ethClient *ethclient.Client, contractAddress persist.Address, lastSyncedBlock persist.BlockNumber) *persist.Contract {
	c := &persist.Contract{
		Address:     contractAddress,
		LatestBlock: lastSyncedBlock,
	}
	cMetadata, err := getTokenContractMetadata(contractAddress, ethClient)
	if err != nil {
		logrus.WithError(err).WithField("address", contractAddress).Error("error getting contract metadata")
	} else {
		c.Name = cMetadata.Name
		c.Symbol = cMetadata.Symbol
	}
	return c
}

func (i *Indexer) subscribeNewLogs(lastSyncedBlock persist.BlockNumber, transfers chan<- []*transfer, subscriptions chan types.Log, topics [][]common.Hash) {

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
			transfers <- logsToTransfers([]types.Log{log}, i.ethClient)
		case err := <-sub.Err():
			panic(fmt.Sprintf("error in log subscription: %s", err))
		}
	}
}

func getUniqueMetadataHandlers() uniqueMetadatas {
	return uniqueMetadatas{
		persist.Address("0xd4e4078ca3495DE5B1d4dB434BEbc5a986197782"): autoglyphs,
		persist.Address("0x60F3680350F65Beb2752788cB48aBFCE84a4759E"): colorglyphs,
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
