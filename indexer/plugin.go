package indexer

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

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
	pluginTimeout     = 2 * time.Minute
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
		owners:         newOwnerPlugin(sentryutil.NewSentryHubContext(ctx), ethClient),
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

// RunPluginReceiver runs a plugin receiver and will update the out map with the results of the receiver, ensuring that the most recent data is kept.
// If the incoming channel is nil, the function will return immediately.
func RunPluginReceiver[T, V orderedBlockChainData](ctx context.Context, wg *sync.WaitGroup, mu *sync.Mutex, receiver PluginReceiver[T, V], incoming <-chan T, out map[persist.EthereumTokenIdentifiers]V) {
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
				innerCtx, cancel := context.WithTimeout(ctx, pluginTimeout)
				defer cancel()
				child := span.StartChild("plugin.uriPlugin")
				child.Description = "handleMessage"

				var uri persist.TokenURI

				ct, tid, err := msg.key.GetParts()
				if err != nil {
					panic(err)
				}

				dbURI, _, _, err := tokenRepo.GetMetadataByTokenIdentifiers(innerCtx, tid, ct)
				if err == nil {
					if dbURI != "" {
						uri = dbURI
					}
				}

				if uri == "" && rpcEnabled {
					uri = getURI(innerCtx, msg.transfer.ContractAddress, msg.transfer.TokenID, msg.transfer.TokenType, ethClient)
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
		logger.For(ctx).Info("uri plugin finished sending")
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
				innerCtx, cancel := context.WithTimeout(ctx, pluginTimeout)
				defer cancel()
				child := span.StartChild("plugin.balancePlugin")
				child.Description = "handleMessage"

				if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC1155 {
					if rpcEnabled {
						bals, err := getBalances(innerCtx, msg.transfer.ContractAddress, msg.transfer.From, msg.transfer.TokenID, msg.key, msg.transfer.BlockNumber, msg.transfer.TxIndex, msg.transfer.To, ethClient)
						if err != nil {
							logger.For(innerCtx).WithError(err).WithFields(logrus.Fields{
								"fromAddress":     msg.transfer.From,
								"tokenIdentifier": msg.key,
								"block":           msg.transfer.BlockNumber,
							}).Errorf("error getting balance of %s for %s", msg.transfer.From, msg.key)
						} else {
							out <- bals
						}
					} else {
						bals := balancesFromRepo(innerCtx, tokenRepo, msg)
						out <- bals
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()

		logger.For(ctx).Info("balance plugin finished sending")
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

func newOwnerPlugin(ctx context.Context, ethClient *ethclient.Client) ownersPlugin {
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
					if rpcEnabled {
						owner, err := getOwner(ctx, msg.transfer.ContractAddress, msg.transfer.TokenID, msg.key, msg.transfer.BlockNumber, msg.transfer.TxIndex, ethClient)
						if err != nil {
							logger.For(ctx).WithError(err).WithFields(logrus.Fields{
								"tokenIdentifier": msg.key,
								"block":           msg.transfer.BlockNumber,
							}).Errorf("error getting owner of %s", msg.key)
							out <- ownerAtBlock{
								ti:    msg.key,
								owner: msg.transfer.To,
								boi: blockchainOrderInfo{
									blockNumber: msg.transfer.BlockNumber,
									txIndex:     msg.transfer.TxIndex,
								},
							}
						} else {
							out <- owner
						}
					} else {
						out <- ownerAtBlock{
							ti:    msg.key,
							owner: msg.transfer.To,
							boi: blockchainOrderInfo{
								blockNumber: msg.transfer.BlockNumber,
								txIndex:     msg.transfer.TxIndex,
							},
						}
					}
				}

				tracing.FinishSpan(child)
			})
		}

		wp.StopWait()
		logger.For(ctx).Info("owners plugin finished sending")
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
		span, _ := startSpan(ctx, "previousOwnerPlugin", "handleBatch")
		defer tracing.FinishSpan(span)
		defer close(out)

		wp := workerpool.New(pluginPoolSize)

		for msg := range in {
			msg := msg
			wp.Submit(func() {
				child := span.StartChild("plugin.previousOwnerPlugin")
				child.Description = "handleMessage"

				if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC721 {
					out <- ownerAtBlock{
						ti:    msg.key,
						owner: msg.transfer.From,
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
		logger.For(ctx).Info("previous owners plugin finished sending")
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

		logger.For(ctx).Info("refresh plugin finished sending")
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
