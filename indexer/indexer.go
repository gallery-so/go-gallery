package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

var defaultStartingBlock persist.BlockNumber = 6000000

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

type tokenBalanceChange struct {
	ti    tokenIdentifiers
	from  persist.Address
	to    persist.Address
	amt   *big.Int
	block persist.BlockNumber
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
	state int64

	ethClient    *ethclient.Client
	ipfsClient   *shell.Shell
	tokenRepo    persist.TokenRepository
	contractRepo persist.ContractRepository

	chain persist.Chain

	eventHashes []eventHash

	lastSyncedBlock persist.BlockNumber
	mostRecentBlock persist.BlockNumber

	metadatas      chan tokenMetadata
	uris           chan tokenURI
	owners         chan ownerAtBlock
	balances       chan tokenBalanceChange
	previousOwners chan ownerAtBlock

	subscriptions chan types.Log
	transfers     chan []*transfer
	tokens        chan []*persist.Token
	contracts     chan *persist.Contract
	done          chan error

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
		mostRecentBlock: persist.BlockNumber(mostRecentBlockUint64),

		metadatas:      make(chan tokenMetadata),
		uris:           make(chan tokenURI),
		owners:         make(chan ownerAtBlock),
		balances:       make(chan tokenBalanceChange),
		previousOwners: make(chan ownerAtBlock),

		subscriptions: make(chan types.Log),
		transfers:     make(chan []*transfer),
		tokens:        make(chan []*persist.Token),
		contracts:     make(chan *persist.Contract),
		done:          make(chan error),

		uniqueMetadatas: getUniqueMetadataHandlers(),
	}
}

// Start begins indexing events from the blockchain
func (i *Indexer) Start() {

	i.state = 1
	go i.listenForNewBlocks()
	go i.processLogs()
	go i.processTransfers()
	go i.processTokens()
	go i.processContracts()
	i.handleDone()
}

func (i *Indexer) processLogs() {

	// lastSyncedBlock := defaultStartingBlock
	lastSyncedBlock := persist.BlockNumber(13014050)

	// if mostRecent, err := tokenRepo.MostRecentBlock(context.Background()); err == nil && mostRecent > defaultStartingBlock {
	// 	lastSyncedBlock = mostRecent
	// } else {
	// 	logrus.Infof("No most recent block found, starting at %d", startingBlock)
	// 	logrus.WithError(err).Info("No most recent block found")
	// }

	defer func() {
		go i.subscribeNewLogs()
	}()

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	curBlock := lastSyncedBlock.BigInt()
	nextBlock := new(big.Int).Add(curBlock, big.NewInt(int64(50)))
	errs := 0
	for nextBlock.Cmp(i.mostRecentBlock.BigInt()) == -1 {
		if errs > 5 {
			nextBlock.Add(curBlock, big.NewInt(int64(5)))
			time.Sleep(time.Second * 20)
		}
		logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		logsTo, err := i.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: curBlock,
			ToBlock:   nextBlock,
			Topics:    topics,
		})
		cancel()
		if err != nil {
			errs++
			logrus.WithError(err).Error("error getting logs, trying again")
			time.Sleep(time.Second * 10)
			continue
		}
		if errs > 5 {
			nextBlock.Add(curBlock, big.NewInt(int64(50)))
		}
		errs = 0
		logrus.Infof("Found %d logs at block %d", len(logsTo), curBlock.Uint64())

		i.transfers <- logsToTransfer(logsTo)

		lastSyncedBlock = persist.BlockNumber(nextBlock.Uint64())
		i.lastSyncedBlock = lastSyncedBlock

		curBlock.Add(curBlock, big.NewInt(int64(50)))
		nextBlock.Add(nextBlock, big.NewInt(int64(50)))
	}

}

func (i *Indexer) processTransfers() {

	defer close(i.tokens)
	for transfers := range i.transfers {
		result := make([]*transfer, len(transfers))
		copy(result, transfers)
		if transfers != nil {
			logrus.Infof("Processing %d transfers", len(transfers))
			go processTransfers(i, result)
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
						logrus.WithError(err).WithFields(logrus.Fields{"id": token.TokenID, "contract": token.ContractAddress}).Error("error processing token")
					}
				}
			}(tokens)
		}
	}()

	mu := &sync.Mutex{}
	owners := map[tokenIdentifiers]ownerAtBlock{}
	previousOwners := map[tokenIdentifiers][]ownerAtBlock{}
	balances := map[tokenIdentifiers]map[persist.Address]balanceAtBlock{}
	metadatas := map[tokenIdentifiers]tokenMetadata{}
	uris := map[tokenIdentifiers]tokenURI{}

	go func() {
		for {
			<-time.After(time.Second * 20)
			mu.Lock()
			i.tokens <- i.storedDataToTokens(owners, previousOwners, balances, metadatas, uris)
			owners = map[tokenIdentifiers]ownerAtBlock{}
			previousOwners = map[tokenIdentifiers][]ownerAtBlock{}
			balances = map[tokenIdentifiers]map[persist.Address]balanceAtBlock{}
			metadatas = map[tokenIdentifiers]tokenMetadata{}
			uris = map[tokenIdentifiers]tokenURI{}
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
				contractAddress, tokenID, err := parseTokenIdentifiers(owner.ti)
				if err != nil {
					i.failWithMessage(err, "error parsing token identifiers")
					return
				}
				tokens, err := i.tokenRepo.GetByTokenIdentifiers(context.Background(), tokenID, contractAddress)
				if err == nil && len(tokens) == 1 {
					token := tokens[0]
					block := token.BlockNumber
					curOwner := token.OwnerAddress
					if owner.block > block {
						owners[owner.ti] = owner
					} else {
						owners[owner.ti] = ownerAtBlock{block: block, owner: curOwner}
					}
				} else {
					owners[owner.ti] = owner
				}

			}
		case balance := <-i.balances:
			if balances[balance.ti] == nil {
				balances[balance.ti] = map[persist.Address]balanceAtBlock{}
				contractAddress, tokenID, err := parseTokenIdentifiers(balance.ti)
				if err == nil && contractAddress != "" && tokenID != "" {
					tokens, err := i.tokenRepo.GetByTokenIdentifiers(context.Background(), tokenID, contractAddress)
					if err == nil {
						for _, token := range tokens {
							balances[balance.ti][token.OwnerAddress] = balanceAtBlock{block: token.BlockNumber, amnt: new(big.Int).SetUint64(token.Quantity)}
						}
					}
				}
			}
			balTo := balances[balance.ti][balance.to]
			balFrom := balances[balance.ti][balance.from]
			if balTo.amnt == nil {
				balTo.amnt = big.NewInt(0)
			}
			if balFrom.amnt == nil {
				balFrom.amnt = big.NewInt(0)
			}
			balTo.amnt.Add(balTo.amnt, balance.amt)
			balFrom.amnt.Sub(balFrom.amnt, balance.amt)
			balances[balance.ti][balance.from] = balFrom
			balances[balance.ti][balance.to] = balTo
		case metadata := <-i.metadatas:
			if len(metadata.md) == 0 || metadata.md == nil {
				contractAddress, tokenID, err := parseTokenIdentifiers(metadata.ti)
				if err == nil {
					tokens, err := i.tokenRepo.GetByTokenIdentifiers(context.Background(), tokenID, contractAddress)
					if err == nil && len(tokens) > 0 {
						token := tokens[0]
						metadata.md = token.TokenMetadata
					}
				}
			}
			metadatas[metadata.ti] = metadata
		case uri := <-i.uris:
			if uri.uri.String() == "" {
				contractAddress, tokenID, err := parseTokenIdentifiers(uri.ti)
				if err == nil {
					tokens, err := i.tokenRepo.GetByTokenIdentifiers(context.Background(), tokenID, contractAddress)
					if err == nil && len(tokens) > 0 {
						token := tokens[0]
						uri.uri = token.TokenURI
					}
				}
			}
			uris[uri.ti] = uri
		case previousOwner := <-i.previousOwners:
			if previousOwners[previousOwner.ti] == nil {
				previousOwners[previousOwner.ti] = []ownerAtBlock{}
				contractAddress, tokenID, err := parseTokenIdentifiers(previousOwner.ti)
				if err == nil && contractAddress != "" && tokenID != "" {
					tokens, err := i.tokenRepo.GetByTokenIdentifiers(context.Background(), tokenID, contractAddress)
					if err == nil && len(tokens) == 1 {
						token := tokens[0]
						ownersAtBlocks := make([]ownerAtBlock, len(token.PreviousOwners))
						for i, o := range token.PreviousOwners {
							ownersAtBlocks[i] = ownerAtBlock{
								ti:    previousOwner.ti,
								block: persist.BlockNumber(i),
								owner: o,
							}
						}
						previousOwners[previousOwner.ti] = ownersAtBlocks
					}
				}
			}
			previousOwners[previousOwner.ti] = append(previousOwners[previousOwner.ti], previousOwner)
		}
		mu.Unlock()
	}
}

func (i *Indexer) processContracts() {
	for contract := range i.contracts {
		go func(c *persist.Contract) {
			logrus.Infof("Processing contract %s", c.Address)
			err := i.contractReceive(context.Background(), c)
			if err != nil {
				logrus.WithError(err).WithField("address", c.Address).Error("error processing contract")
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
		FromBlock: i.lastSyncedBlock.BigInt(),
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
			i.lastSyncedBlock = persist.BlockNumber(log.BlockNumber)
			i.transfers <- logsToTransfer([]types.Log{log})
		case err := <-sub.Err():
			i.failWithMessage(err, "subscription error")
			return
		}
	}
}

func (i *Indexer) handleDone() {
	for {
		err := <-i.done
		logrus.Errorf("Indexer done: %v", err)
		os.Exit(1)
		panic(err)
	}
}

func processTransfers(i *Indexer, transfers []*transfer) {

	for _, transfer := range transfers {
		contractAddress := transfer.contractAddress
		from := transfer.from
		to := transfer.to
		tokenID := transfer.tokenID

		key := makeKeyForToken(contractAddress, tokenID)
		logrus.Infof("Processing transfer %s to %s and from %s ", key, to, from)

		u, err := GetTokenURI(transfer.tokenType, contractAddress, tokenID, i.ethClient)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"id": tokenID, "contract": contractAddress}).Error("error getting URI for ERC1155 token")
		}
		switch persist.TokenType(transfer.tokenType) {
		case persist.TokenTypeERC721:
			i.owners <- ownerAtBlock{key, to, transfer.blockNumber}
			i.previousOwners <- ownerAtBlock{key, from, transfer.blockNumber}

		case persist.TokenTypeERC1155:
			i.balances <- tokenBalanceChange{key, from, to, new(big.Int).SetUint64(transfer.amount), transfer.blockNumber}

		default:
			i.failWithMessage(errors.New("token type"), "unknown token type")
		}

		id, err := util.HexToBigInt(string(tokenID))
		if err != nil {
			i.failWithMessage(err, "failed to convert token ID to big int")
			return
		}

		uriReplaced := persist.TokenURI(strings.ReplaceAll(u.String(), "{id}", id.String()))
		i.uris <- tokenURI{key, uriReplaced}
		go func() {
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
			i.metadatas <- tokenMetadata{key, metadata}
		}()
	}
}

func (i *Indexer) tokenReceive(ctx context.Context, t *persist.Token) error {
	// if t.TokenURI == "" {
	// 	return errors.New("token URI is empty")
	// }
	if t.OwnerAddress == "0x00000000000000000000000000000000000000" || t.OwnerAddress == "" {
		return errors.New("token owner is empty")
	}
	if err := i.tokenRepo.Upsert(ctx, t); err != nil {
		return err
	}
	go func() {
		i.contracts <- handleContract(i.ethClient, t.ContractAddress, t.BlockNumber)
	}()
	return nil
}

func (i *Indexer) contractReceive(ctx context.Context, contract *persist.Contract) error {
	return i.contractRepo.UpsertByAddress(ctx, contract.Address, contract)
}

func (i *Indexer) storedDataToTokens(owners map[tokenIdentifiers]ownerAtBlock, previousOwners map[tokenIdentifiers][]ownerAtBlock, balances map[tokenIdentifiers]map[persist.Address]balanceAtBlock, metadatas map[tokenIdentifiers]tokenMetadata, uris map[tokenIdentifiers]tokenURI) []*persist.Token {
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
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
		if err != nil {
			i.failWithMessage(err, "failed to parse key for token")
			return nil
		}
		previousOwnerAddresses := make([]persist.Address, len(previousOwners[k]))
		for i, w := range previousOwners[k] {
			previousOwnerAddresses[i] = w.owner
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
			TokenID:         tokenID,
			ContractAddress: contractAddress,
			OwnerAddress:    v.owner,
			Quantity:        1,
			Name:            name,
			Description:     description,
			PreviousOwners:  previousOwnerAddresses,
			TokenType:       persist.TokenTypeERC721,
			TokenMetadata:   metadata.md,
			TokenURI:        uris[k].uri,
			Chain:           i.chain,
			BlockNumber:     v.block,
		}
		j++
	}
	for k, v := range balances {
		contractAddress, tokenID, err := parseTokenIdentifiers(k)
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
				TokenID:         tokenID,
				ContractAddress: contractAddress,
				OwnerAddress:    addr,
				Quantity:        balance.amnt.Uint64(),
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
	}

	return result
}

func logsToTransfer(pLogs []types.Log) []*transfer {
	result := []*transfer{}
	for _, pLog := range pLogs {
		logrus.Infof("log %+v", pLog)
		switch {
		case strings.EqualFold(pLog.Topics[0].Hex(), string(transferEventHash)):

			if len(pLog.Topics) < 4 {
				logrus.WithFields(logrus.Fields{"address": pLog.Address, "block": pLog.BlockNumber, "topics": pLog.Topics}).Warn("wrong ERC721 Transfer topics length")
				continue
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
				logrus.WithFields(logrus.Fields{"address": pLog.Address, "block": pLog.BlockNumber, "topics": pLog.Topics}).Warn("wrong ERC1155 Single Transfer topics length")
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
				logrus.WithFields(logrus.Fields{"address": pLog.Address, "block": pLog.BlockNumber, "topics": pLog.Topics}).Warn("wrong ERC1155 Batch Transfer topics length")
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
			i.failWithMessage(err, "failed to get block number")
			return
		}
		i.mostRecentBlock = persist.BlockNumber(finalBlockUint)
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
		persist.Address("0xd4e4078ca3495DE5B1d4dB434BEbc5a986197782"): autoglyphs,
		persist.Address("0x60F3680350F65Beb2752788cB48aBFCE84a4759E"): colorglyphs,
	}
}

func findFirstFieldFromMetadata(metadata map[string]interface{}, fields ...string) interface{} {
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
