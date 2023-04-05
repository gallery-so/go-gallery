package indexer

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/sirupsen/logrus"
)

const (
	pluginPoolSize    = 32
	pluginTimeout     = 2 * time.Minute
	bloomFilterSize   = 100000
	falsePositiveRate = 0.01
)

// TransferPluginMsg is used to communicate to a plugin.
type TransferPluginMsg struct {
	transfer rpc.Transfer
	key      persist.EthereumTokenIdentifiers
}

// TransferPlugins are plugins that add contextual data to a transfer.
type TransferPlugins struct {
	contracts contractTransfersPlugin
}

type blockchainOrderInfo struct {
	blockNumber persist.BlockNumber
	txIndex     uint
}

// Less returns true if the current block number and tx index are less than the other block number and tx index.
func (b blockchainOrderInfo) Less(other blockchainOrderInfo) bool {
	if b.blockNumber < other.blockNumber {
		return true
	}
	if b.blockNumber > other.blockNumber {
		return false
	}
	return b.txIndex < other.txIndex
}

type orderedBlockChainData interface {
	TokenIdentifiers() persist.EthereumTokenIdentifiers
	OrderInfo() blockchainOrderInfo
}

// TransferPluginReceiver receives the results of a plugin.
type TransferPluginReceiver[T, V orderedBlockChainData] func(cur V, inc T) V

func startSpan(ctx context.Context, plugin, op string) (*sentry.Span, context.Context) {
	return tracing.StartSpan(ctx, "indexer.plugin", fmt.Sprintf("%s:%s", plugin, op))
}

// NewTransferPlugins returns a set of transfer plugins. Plugins have an `in` and an optional `out` channel that are handles to the service.
// The `in` channel is used to submit a transfer to a plugin, and the `out` channel is used to receive results from a plugin, if any.
// A plugin can be stopped by closing its `in` channel, which finishes the plugin and lets receivers know that its done.
func NewTransferPlugins(ctx context.Context, ethClient *ethclient.Client, httpClient *http.Client, contractOwnerStats *sync.Map) TransferPlugins {
	return TransferPlugins{
		contracts: newContractsPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, httpClient, contractOwnerStats),
	}
}

// RunTransferPlugins returns when all plugins have received the message. Every plugin recieves the same message.
func RunTransferPlugins(ctx context.Context, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, plugins []chan<- TransferPluginMsg) {
	span, ctx := tracing.StartSpan(ctx, "indexer.plugin", "submitMessage")
	defer tracing.FinishSpan(span)

	msg := TransferPluginMsg{
		transfer: transfer,
		key:      key,
	}
	for _, plugin := range plugins {
		plugin <- msg
	}
}

// RunTransferPluginReceiver runs a plugin receiver and will update the out map with the results of the receiver, ensuring that the most recent data is kept.
// If the incoming channel is nil, the function will return immediately.
func RunTransferPluginReceiver[T, V orderedBlockChainData](ctx context.Context, wg *sync.WaitGroup, mu *sync.Mutex, receiver TransferPluginReceiver[T, V], incoming <-chan T, out map[persist.EthereumTokenIdentifiers]V) {
	span, _ := tracing.StartSpan(ctx, "indexer.plugin", "runPluginReceiver")
	defer tracing.FinishSpan(span)

	if incoming == nil {
		return
	}

	if out == nil {
		panic("out map must not be nil")
	}

	wg.Add(1)

	go func() {
		defer wg.Done()

		for it := range incoming {
			func() {
				processed := receiver(out[it.TokenIdentifiers()], it)
				cur, ok := out[processed.TokenIdentifiers()]
				if !ok || cur.OrderInfo().Less(processed.OrderInfo()) {
					mu.Lock()
					defer mu.Unlock()
					out[processed.TokenIdentifiers()] = processed
				}
			}()
		}

		logger.For(ctx).WithFields(logrus.Fields{"incoming_type": fmt.Sprintf("%T", *new(T)), "outgoing_type": fmt.Sprintf("%T", *new(V))}).Info("plugin finished receiving")
	}()

}

// contractTransfersPlugin retrieves ownership information for a token.
type contractTransfersPlugin struct {
	in  chan TransferPluginMsg
	out chan contractAtBlock
}

func newContractsPlugin(ctx context.Context, ethClient *ethclient.Client, httpClient *http.Client, contractOwnerStats *sync.Map) contractTransfersPlugin {
	in := make(chan TransferPluginMsg)
	out := make(chan contractAtBlock)

	go func() {
		span, _ := startSpan(ctx, "ownerPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		for msg := range in {
			msg := msg
			wp.Submit(func() {

				child := span.StartChild("plugin.ownerPlugin")
				child.Description = "handleMessage"

				if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC721 {
					if rpcEnabled {

						contract := fillContractFields(ctx, ethClient, httpClient, msg.transfer.ContractAddress, msg.transfer.BlockNumber, contractOwnerStats)
						out <- contractAtBlock{
							ti: msg.key,
							boi: blockchainOrderInfo{
								blockNumber: msg.transfer.BlockNumber,
								txIndex:     msg.transfer.TxIndex,
							},
							contract: contract,
						}

					} else {
						out <- contractAtBlock{
							ti: msg.key,
							boi: blockchainOrderInfo{
								blockNumber: msg.transfer.BlockNumber,
								txIndex:     msg.transfer.TxIndex,
							},
							contract: persist.Contract{
								Address:     msg.transfer.ContractAddress,
								LatestBlock: msg.transfer.BlockNumber,
							},
						}
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()
		logger.For(ctx).Info("contracts plugin finished sending")
	}()

	return contractTransfersPlugin{
		in:  in,
		out: out,
	}
}
