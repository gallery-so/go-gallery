package infra

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type eventHash string

const (
	transferEventHash       eventHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	transferSingleEventHash eventHash = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	transferBatchEventHash  eventHash = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
	uriEventHash            eventHash = "0x6bb7ff708619ba0610cba295a58592e0451dee2622938c8755667688daf3529b"
)

type ownerAtBlock struct {
	owner string
	block uint64
}

type indexer struct {
	runtime *runtime.Runtime

	mu             *sync.Mutex
	metadatas      map[string]map[string]interface{}
	uris           map[string]string
	types          map[string]string
	owners         map[string]ownerAtBlock
	previousOwners map[string][]ownerAtBlock

	eventHashes []eventHash

	lastBlockNumber uint64

	logs      chan []types.Log
	transfers chan []*transfer
	tokens    chan *persist.Token
	done      chan bool
}

// TODO figure out how to ensure that the event types are represented when going transfer -> token

func newIndexer(pEvents []eventHash, pRuntime *runtime.Runtime) *indexer {
	return &indexer{
		runtime:        pRuntime,
		mu:             &sync.Mutex{},
		metadatas:      make(map[string]map[string]interface{}),
		uris:           make(map[string]string),
		types:          make(map[string]string),
		owners:         make(map[string]ownerAtBlock),
		previousOwners: make(map[string][]ownerAtBlock),

		eventHashes: pEvents,

		logs:      make(chan []types.Log),
		transfers: make(chan []*transfer),
		tokens:    make(chan *persist.Token),
		done:      make(chan bool),
	}
}

func (i *indexer) processLogs() {
	finalBlockUint, err := i.runtime.InfraClients.ETHClient.BlockNumber(context.Background())
	if err != nil {
		logrus.Errorf("failed to get block number: %v", err)
		panic(err)
	}

	events := make([]common.Hash, len(i.eventHashes))
	for i, event := range i.eventHashes {
		events[i] = common.HexToHash(string(event))
	}

	topics := [][]common.Hash{events}

	finalBlock := new(big.Int).SetUint64(finalBlockUint)

	go func() {
		defer close(i.logs)
		curBlock := new(big.Int).SetUint64(i.lastBlockNumber)
		nextBlock := new(big.Int).Add(curBlock, big.NewInt(1800))
		for nextBlock.Cmp(finalBlock) == -1 {
			logsTo, err := i.runtime.InfraClients.ETHClient.FilterLogs(context.Background(), ethereum.FilterQuery{
				FromBlock: curBlock,
				ToBlock:   nextBlock,
				Topics:    topics,
			})
			if err != nil {
				logrus.WithError(err).Error("Error getting logs, trying again")
				continue
			}
			i.logs <- logsTo
			i.lastBlockNumber = curBlock.Uint64()
			curBlock.Add(curBlock, big.NewInt(1800))
			nextBlock.Add(nextBlock, big.NewInt(1800))

			logrus.Info("Getting logs from ", curBlock.String(), " to ", nextBlock.String())
		}
	}()

	for {
		select {
		case logs, ok := <-i.logs:
			for _, log := range logs {
				i.transfers <- logToTransfer(log)
			}
			if !ok {
				close(i.transfers)
				return
			}
		case <-i.done:
			return
		}
	}
}

func logToTransfer(pLog types.Log) []*transfer {
	switch pLog.Topics[0].Hex() {
	case string(transferEventHash):
		from := strings.TrimPrefix("0x", pLog.Topics[1].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		id := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		return []*transfer{
			{
				From:        from,
				To:          to,
				TokenID:     id,
				Amount:      1,
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber).Text(16),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
			},
		}
	case string(transferSingleEventHash):
		if len(pLog.Topics) < 4 {
			panic("invalid topic length for single transfer event")
		}
		from := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		id := new(big.Int).SetBytes(pLog.Data[:len(pLog.Data)/2])
		amount := new(big.Int).SetBytes(pLog.Data[len(pLog.Data)/2:])
		return []*transfer{
			{
				From:        from,
				To:          to,
				TokenID:     id.Text(16),
				Amount:      amount.Uint64(),
				BlockNumber: new(big.Int).SetUint64(pLog.BlockNumber).Text(16),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
			},
		}

	case string(transferBatchEventHash):
		from := strings.TrimPrefix("0x", pLog.Topics[2].Hex())
		to := strings.TrimPrefix("0x", pLog.Topics[3].Hex())
		amountOffset := len(pLog.Data) / 2
		total := amountOffset / 64
		result := make([]*transfer, total)

		for i := 0; i < total; i++ {
			id := new(big.Int).SetBytes(pLog.Data[i*64 : (i+1)*64])
			amount := new(big.Int).SetBytes(pLog.Data[(amountOffset)+(i*64) : (amountOffset)+((i+1)*64)])
			result[i] = &transfer{
				From:    from,
				To:      to,
				TokenID: id.Text(16),
				Amount:  amount.Uint64(),
				RawContract: contract{
					Address: pLog.Address.Hex(),
				},
			}
		}
		return result
	default:
		panic("unknown event hash")
	}
}

func (i *indexer) processTransfers() {
	for {
		select {
		case transfers, ok := <-i.transfers:
			for _, transfer := range transfers {
				bn, err := util.HexToBigInt(transfer.BlockNumber)
				if err != nil {
					panic(err)
				}
				i.mu.Lock()
				if it, ok := i.owners[transfer.RawContract.Address+"--"+transfer.TokenID]; ok {
					if it.owner != transfer.To {
						if it.block < bn.Uint64() {
							it.block = bn.Uint64()
							it.owner = transfer.To
						}
						i.previousOwners[transfer.RawContract.Address+"--"+transfer.TokenID] = append(i.previousOwners[transfer.RawContract.Address+"--"+transfer.TokenID], it)
					}
				} else {
					i.owners[transfer.RawContract.Address+"--"+transfer.TokenID] = ownerAtBlock{transfer.From, bn.Uint64()}
				}
				i.mu.Unlock()

			}
			if !ok {
				i.mu.Lock()
				for k, v := range i.owners {
					spl := strings.Split(k, "--")
					if len(spl) != 2 {
						panic("invalid key")
					}
					sort.Slice(i.previousOwners[k], func(l, m int) bool {
						return i.previousOwners[k][l].block < i.previousOwners[k][m].block
					})
					previousOwnerAddresses := make([]string, len(i.previousOwners[k]))
					for i, v := range i.previousOwners[k] {
						previousOwnerAddresses[i] = v.owner
					}
					token := &persist.Token{
						TokenID:         spl[1],
						ContractAddress: spl[0],
						OwnerAddress:    v.owner,
						PreviousOwners:  previousOwnerAddresses,
						Type:            i.types[k],
					}
					// TODO: this is a hack to compile code
					fmt.Println(token)
				}
				i.mu.Unlock()

				return
			}
		case <-i.done:
			return
		}
	}
}
