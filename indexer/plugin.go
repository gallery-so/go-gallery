package indexer

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/bits-and-blooms/bloom"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/indexer/refresh"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/sirupsen/logrus"
)

const (
	pluginPoolSize    = 32
	bloomFilterSize   = 100000
	falsePositiveRate = 0.01
)

// PluginMsg is used to communicate to a plugin.
type PluginMsg struct {
	transfer rpc.Transfer
	key      persist.EthereumTokenIdentifiers
}

// TransferPlugins are plugins that add contextual data to a transfer.
type TransferPlugins struct {
	uris           urisPlugin
	balances       balancesPlugin
	owners         ownersPlugin
	previousOwners previousOwnersPlugin
	refresh        refreshPlugin
}

type blockchainOrderInfo struct {
	blockNumber persist.BlockNumber
	txIndex     uint
}

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

// PluginReceiver receives the results of a plugin.
type PluginReceiver[T, V orderedBlockChainData] func(cur V, inc T) V

func startSpan(ctx context.Context, plugin, op string) (*sentry.Span, context.Context) {
	return tracing.StartSpan(ctx, "indexer.plugin", fmt.Sprintf("%s:%s", plugin, op))
}

// NewTransferPlugins returns a set of transfer plugins. Plugins have an `in` and an optional `out` channel that are handles to the service.
// The `in` channel is used to submit a transfer to a plugin, and the `out` channel is used to receive results from a plugin, if any.
// A plugin can be stopped by closing its `in` channel, which finishes the plugin and lets receivers know that its done.
func NewTransferPlugins(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, addressFilterRepo refresh.AddressFilterRepository) TransferPlugins {
	return TransferPlugins{
		uris:           newURIsPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo),
		balances:       newBalancesPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo),
		owners:         newOwnerPlugin(sentryutil.NewSentryHubContext(ctx)),
		refresh:        newRefreshPlugin(sentryutil.NewSentryHubContext(ctx), addressFilterRepo),
		previousOwners: newPreviousOwnersPlugin(sentryutil.NewSentryHubContext(ctx)),
	}
}

// RunPlugins returns when all plugins have received the message. Every plugin recieves the same message.
func RunPlugins(ctx context.Context, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, plugins []chan<- PluginMsg) {
	span, ctx := tracing.StartSpan(ctx, "indexer.plugin", "submitMessage")
	defer tracing.FinishSpan(span)

	msg := PluginMsg{
		transfer: transfer,
		key:      key,
	}
	for _, plugin := range plugins {
		plugin <- msg
	}
}

// RunPluginReceiver runs a plugin receiver and returns a channel that will return exactly one result once all of the incoming messages have been processed.
// If the incoming channel is nil, the result channel will return a single nil value immediately.
// The result will be a map of token identifiers to the result of the plugin, ensuring that whatever is returned by the plugin is the most recent result for that token.
// The caller is responsible with closing the channel returned by this function.
func RunPluginReceiver[T, V orderedBlockChainData](ctx context.Context, wg *sync.WaitGroup, receiver PluginReceiver[T, V], incoming <-chan T, out map[persist.EthereumTokenIdentifiers]V) {
	span, _ := tracing.StartSpan(ctx, "indexer.plugin", "runPluginReceiver")
	defer tracing.FinishSpan(span)

	wg.Add(1)
	defer wg.Done()

	if incoming == nil {

		return
	}

	go func() {

		for it := range incoming {

			if out != nil {
				processed := receiver(out[it.TokenIdentifiers()], it)
				cur := out[processed.TokenIdentifiers()]
				if cur.OrderInfo().Less(processed.OrderInfo()) {
					out[processed.TokenIdentifiers()] = processed
				}
			} else {
				out = make(map[persist.EthereumTokenIdentifiers]V)
				cur := receiver(out[it.TokenIdentifiers()], it)
				out[cur.TokenIdentifiers()] = cur
			}

		}

	}()

}

// urisPlugin pulls URI information for a token.
type urisPlugin struct {
	in  chan PluginMsg
	out chan tokenURI
}

func newURIsPlugin(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository) urisPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenURI)

	go func() {
		span, ctx := startSpan(ctx, "uriPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		for msg := range in {
			msg := msg
			wp.Submit(func() {
				child := span.StartChild("plugin.uriPlugin")
				child.Description = "handleMessage"

				var uri persist.TokenURI

				ct, tid, err := msg.key.GetParts()
				if err != nil {
					panic(err)
				}

				dbURI, _, _, err := tokenRepo.GetMetadataByTokenIdentifiers(ctx, tid, ct)
				if err == nil {
					if dbURI != "" {
						uri = dbURI
					}
				}

				if uri == "" && rpcEnabled {
					uri = getURI(ctx, msg.transfer.ContractAddress, msg.transfer.TokenID, msg.transfer.TokenType, ethClient)
				}

				out <- tokenURI{
					ti:  msg.key,
					uri: uri,
					boi: blockchainOrderInfo{
						blockNumber: msg.transfer.BlockNumber,
						txIndex:     msg.transfer.TxIndex,
					},
				}

				tracing.FinishSpan(child)

			})
		}

		wp.StopWait()
	}()

	return urisPlugin{
		in:  in,
		out: out,
	}
}

// balancesPlugin pulls balances for ERC-1155 tokens.
type balancesPlugin struct {
	in  chan PluginMsg
	out chan tokenBalances
}

func newBalancesPlugin(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository) balancesPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenBalances)

	go func() {
		span, ctx := startSpan(ctx, "balancePlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		for msg := range in {
			msg := msg
			wp.Submit(func() {
				child := span.StartChild("plugin.balancePlugin")
				child.Description = "handleMessage"

				if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC1155 {
					if rpcEnabled {
						bals, err := getBalances(ctx, msg.transfer.ContractAddress, msg.transfer.From, msg.transfer.TokenID, msg.key, msg.transfer.BlockNumber, msg.transfer.To, ethClient)
						if err != nil {
							logger.For(ctx).WithError(err).WithFields(logrus.Fields{
								"fromAddress":     msg.transfer.From,
								"tokenIdentifier": msg.key,
								"block":           msg.transfer.BlockNumber,
							}).Errorf("error getting balance of %s for %s", msg.transfer.From, msg.key)
						} else {
							out <- bals
						}
					} else {
						bals := balancesFromRepo(ctx, tokenRepo, msg)
						out <- bals
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()
	}()

	return balancesPlugin{
		in:  in,
		out: out,
	}
}

// ownersPlugin retrieves ownership information for a token.
type ownersPlugin struct {
	in  chan PluginMsg
	out chan ownerAtBlock
}

func newOwnerPlugin(ctx context.Context) ownersPlugin {
	in := make(chan PluginMsg)
	out := make(chan ownerAtBlock)

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
					out <- ownerAtBlock{
						ti:    msg.key,
						owner: msg.transfer.To,
						boi: blockchainOrderInfo{
							blockNumber: msg.transfer.BlockNumber,
							txIndex:     msg.transfer.TxIndex,
						},
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()
	}()

	return ownersPlugin{
		in:  in,
		out: out,
	}
}

// ownersPlugin retrieves ownership information for a token.
type previousOwnersPlugin struct {
	in  chan PluginMsg
	out chan ownerAtBlock
}

func newPreviousOwnersPlugin(ctx context.Context) previousOwnersPlugin {
	in := make(chan PluginMsg)
	out := make(chan ownerAtBlock)

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
					out <- ownerAtBlock{
						ti:    msg.key,
						owner: msg.transfer.To,
						boi: blockchainOrderInfo{
							blockNumber: msg.transfer.BlockNumber,
							txIndex:     msg.transfer.TxIndex,
						},
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()
	}()

	return previousOwnersPlugin{
		in:  in,
		out: out,
	}
}

// refreshPlugin stores additional data to enable deep refreshes.
type refreshPlugin struct {
	in  chan PluginMsg
	out chan errForTokenAtBlockAndIndex
}

func newRefreshPlugin(ctx context.Context, addressFilterRepo refresh.AddressFilterRepository) refreshPlugin {
	in := make(chan PluginMsg)
	out := make(chan errForTokenAtBlockAndIndex, 1)

	go func() {
		span, _ := startSpan(ctx, "refreshPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		var lock sync.Mutex

		filters := make(map[persist.BlockRange]*bloom.BloomFilter)

		wp := workerpool.New(pluginPoolSize)

		for msg := range in {
			msg := msg
			wp.Submit(func() {
				child := span.StartChild("plugin.refreshPlugin")
				child.Description = "handleMessage"

				fromBlock := msg.transfer.BlockNumber - (msg.transfer.BlockNumber % blocksPerLogsCall)
				toBlock := fromBlock + blocksPerLogsCall
				key := persist.BlockRange{fromBlock, toBlock}

				lock.Lock()
				if _, ok := filters[key]; !ok {
					filters[key] = bloom.NewWithEstimates(bloomFilterSize, falsePositiveRate)
				}
				filters[key] = filters[key].AddString(msg.transfer.From.String())
				filters[key] = filters[key].AddString(msg.transfer.To.String())
				filters[key] = filters[key].AddString(msg.transfer.ContractAddress.String())
				lock.Unlock()

				tracing.FinishSpan(child)

			})
		}

		wp.StopWait()

		out <- errForTokenAtBlockAndIndex{err: addressFilterRepo.BulkUpsert(ctx, filters)}
	}()

	return refreshPlugin{
		in:  in,
		out: out,
	}
}

func balancesFromRepo(ctx context.Context, tokenRepo persist.TokenRepository, msg PluginMsg) tokenBalances {
	fromAmount := bigZero
	toAmount := big.NewInt(int64(msg.transfer.Amount))

	curToken, err := tokenRepo.GetByIdentifiers(ctx, msg.transfer.TokenID, msg.transfer.ContractAddress, msg.transfer.From)
	if err == nil {
		fromAmount = big.NewInt(0).Sub(curToken.Quantity.BigInt(), big.NewInt(int64(msg.transfer.Amount)))
	}

	toToken, err := tokenRepo.GetByIdentifiers(ctx, msg.transfer.TokenID, msg.transfer.ContractAddress, msg.transfer.To)
	if err == nil {
		toAmount = big.NewInt(0).Add(toToken.Quantity.BigInt(), big.NewInt(int64(msg.transfer.Amount)))
	}

	return tokenBalances{
		ti:      msg.key,
		from:    msg.transfer.From,
		to:      msg.transfer.To,
		fromAmt: fromAmount,
		toAmt:   toAmount,
		boi: blockchainOrderInfo{
			blockNumber: msg.transfer.BlockNumber,
			txIndex:     msg.transfer.TxIndex,
		},
	}
}
