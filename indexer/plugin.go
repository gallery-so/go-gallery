package indexer

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	pluginPoolSize = 32
	pluginTimeout  = 2 * time.Minute
)

// TransferPluginMsg is used to communicate to a plugin.
type TransferPluginMsg struct {
	transfer rpc.Transfer
	key      persist.EthereumTokenIdentifiers
}

// TransferPlugins are plugins that add contextual data to a transfer.
type TransferPlugins struct {
	contracts contractTransfersPlugin
	tokens    tokenTransfersPlugin
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
func NewTransferPlugins(ctx context.Context) TransferPlugins {
	ctx = sentryutil.NewSentryHubContext(ctx)
	return TransferPlugins{
		contracts: newContractsPlugin(ctx),
		tokens:    newTokensPlugin(ctx),
	}
}

// RunTransferPlugins returns when all plugins have received the message. Every plugin recieves the same message.
func RunTransferPlugins(ctx context.Context, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, plugins []chan<- TransferPluginMsg) {
	span, _ := tracing.StartSpan(ctx, "indexer.plugin", "submitMessage")
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

func newContractsPlugin(ctx context.Context) contractTransfersPlugin {
	in := make(chan TransferPluginMsg)
	out := make(chan contractAtBlock)

	go func() {
		span, _ := startSpan(ctx, "contractTransfersPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		seenContracts := map[persist.EthereumAddress]bool{}

		for msg := range in {

			if seenContracts[msg.transfer.ContractAddress] {
				continue
			}

			msg := msg
			wp.Submit(func() {

				child := span.StartChild("plugin.contractTransfersPlugin")
				child.Description = "handleMessage"

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

				tracing.FinishSpan(child)
			})

			seenContracts[msg.transfer.ContractAddress] = true
		}

		wp.StopWait()
		logger.For(ctx).Info("contracts plugin finished sending")
	}()

	return contractTransfersPlugin{
		in:  in,
		out: out,
	}
}

type tokenTransfersPlugin struct {
	in  chan TransferPluginMsg
	out chan tokenAtBlock
}

func newTokensPlugin(ctx context.Context) tokenTransfersPlugin {
	in := make(chan TransferPluginMsg)
	out := make(chan tokenAtBlock)

	go func() {
		span, _ := startSpan(ctx, "tokenTransfersPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		seenTokens := map[persist.TokenUniqueIdentifiers]bool{}

		for msg := range in {

			contract, tokenID, err := msg.key.GetParts()
			if err != nil {
				panic(err)
			}
			ids := persist.TokenUniqueIdentifiers{
				// TODO currently this can only be ETH but we should transition away from persist.EthereumAddress and hard coded ETH on the indexer if we could see ourselves indexing other EVMs
				Chain:           persist.ChainETH,
				ContractAddress: persist.Address(contract),
				TokenID:         tokenID,
				OwnerAddress:    persist.Address(msg.transfer.To.String()),
			}

			if seenTokens[ids] && msg.transfer.TokenType != persist.TokenTypeERC1155 {
				continue
			}

			msg := msg
			wp.Submit(func() {

				child := span.StartChild("plugin.tokenTransfersPlugin")
				child.Description = "handleMessage"

				out <- tokenAtBlock{
					ti: msg.key,
					boi: blockchainOrderInfo{
						blockNumber: msg.transfer.BlockNumber,
						txIndex:     msg.transfer.TxIndex,
					},
					token: persist.Token{
						TokenType:       msg.transfer.TokenType,
						Chain:           persist.ChainETH,
						TokenID:         tokenID,
						OwnerAddress:    msg.transfer.To,
						BlockNumber:     msg.transfer.BlockNumber,
						ContractAddress: contract,
						Quantity:        "1",
					},
				}

				tracing.FinishSpan(child)
			})

			seenTokens[ids] = true
		}

		wp.StopWait()
		logger.For(ctx).Info("tokens plugin finished sending")
	}()

	return tokenTransfersPlugin{
		in:  in,
		out: out,
	}
}
