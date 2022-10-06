package indexer

import (
	"context"
	"math/big"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/bits-and-blooms/bloom"
	"github.com/ethereum/go-ethereum/ethclient"
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
	uris     urisPlugin
	balances balancesPlugin
	owners   ownersPlugin
	refresh  refreshPlugin
}

// PluginReceiver receives the results of a plugin.
type PluginReceiver func()

func startSpan(ctx context.Context, pluginName string) (*sentry.Span, context.Context) {
	return tracing.StartSpan(ctx, "indexer.runPlugin", pluginName)
}

// NewTransferPlugins returns a set of transfer plugins. Plugins have an `in` and an optional `out` channel that are handles to the service.
// The `in` channel is used to submit a transfer to a plugin, and the `out` channel is used to receive results from a plugin, if any.
// A plugin can be stopped by closing its `in` channel, which finishes the plugin and lets receivers know that its done.
func NewTransferPlugins(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, addressFilterRepo refresh.AddressFilterRepository, storageClient *storage.Client) TransferPlugins {
	return TransferPlugins{
		uris:     newURIsPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo, storageClient),
		balances: newBalancesPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo, storageClient),
		owners:   newOwnerPlugin(sentryutil.NewSentryHubContext(ctx)),
		refresh:  newRefreshPlugin(sentryutil.NewSentryHubContext(ctx), addressFilterRepo),
	}
}

// RunPlugins returns when all plugins have received the message.
func RunPlugins(ctx context.Context, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, plugins []chan<- PluginMsg) {
	span, _ := startSpan(ctx, "submitMessage")
	defer tracing.FinishSpan(span)

	msg := PluginMsg{
		transfer: transfer,
		key:      key,
	}
	for _, plugin := range plugins {
		plugin <- msg
	}
}

// ReceivePlugins blocks until all plugins have completed.
func ReceivePlugins(ctx context.Context, wg *sync.WaitGroup, receivers []PluginReceiver) {
	span, _ := startSpan(ctx, "receivePlugins")
	defer tracing.FinishSpan(span)

	for _, receiver := range receivers {
		go receiver()
	}
	wg.Wait()
}

// AddReceiver adds a receiver to the list of receivers to synchronize.
func AddReceiver(wg *sync.WaitGroup, receivers []PluginReceiver, receiver PluginReceiver) []PluginReceiver {
	wg.Add(1)
	return append(receivers, receiver)
}

// urisPlugin pulls URI information for a token.
type urisPlugin struct {
	in  chan PluginMsg
	out chan tokenURI
}

func newURIsPlugin(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, storageClient *storage.Client) urisPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenURI)

	go func() {
		span, ctx := startSpan(ctx, "uriPlugin")
		defer tracing.FinishSpan(span)
		defer close(out)

		for msg := range in {
			child := span.StartChild("handleMessage")

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
			}

			tracing.FinishSpan(child)
		}
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

func newBalancesPlugin(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, storageClient *storage.Client) balancesPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenBalances)

	go func() {
		span, ctx := startSpan(ctx, "balancePlugin")
		defer tracing.FinishSpan(span)
		defer close(out)

		for msg := range in {
			child := span.StartChild("handleMessage")

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
		}
	}()

	return balancesPlugin{
		in:  in,
		out: out,
	}
}

// ownersPlugin retrieves ownership information for a token.
type ownersPlugin struct {
	in  chan PluginMsg
	out chan ownersPluginResult
}

// ownersPluginResult is the result of running an ownersPlugin.
type ownersPluginResult struct {
	currentOwner  ownerAtBlock
	previousOwner ownerAtBlock
}

func newOwnerPlugin(ctx context.Context) ownersPlugin {
	in := make(chan PluginMsg)
	out := make(chan ownersPluginResult)

	go func() {
		span, _ := startSpan(ctx, "ownerPlugin")
		defer tracing.FinishSpan(span)
		defer close(out)

		for msg := range in {
			child := span.StartChild("handleMessage")

			if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC721 {
				out <- ownersPluginResult{
					currentOwner: ownerAtBlock{
						ti:    msg.key,
						owner: msg.transfer.To,
						block: msg.transfer.BlockNumber,
					},
					previousOwner: ownerAtBlock{
						ti:    msg.key,
						owner: msg.transfer.From,
						block: msg.transfer.BlockNumber,
					},
				}
			}

			tracing.FinishSpan(child)
		}
	}()

	return ownersPlugin{
		in:  in,
		out: out,
	}
}

// refreshPlugin stores additional data to enable deep refreshes.
type refreshPlugin struct {
	in  chan PluginMsg
	out chan error
}

func newRefreshPlugin(ctx context.Context, addressFilterRepo refresh.AddressFilterRepository) refreshPlugin {
	in := make(chan PluginMsg)
	out := make(chan error, 1)

	go func() {
		span, _ := startSpan(ctx, "refreshPlugin")
		defer tracing.FinishSpan(span)
		defer close(out)

		filters := make(map[persist.BlockRange]*bloom.BloomFilter)

		for msg := range in {
			child := span.StartChild("handleMessage")

			fromBlock := msg.transfer.BlockNumber - (msg.transfer.BlockNumber % blocksPerLogsCall)
			toBlock := fromBlock + blocksPerLogsCall
			key := persist.BlockRange{fromBlock, toBlock}

			if _, ok := filters[key]; !ok {
				filters[key] = bloom.NewWithEstimates(bloomFilterSize, falsePositiveRate)
			}

			filters[key] = filters[key].AddString(msg.transfer.From.String())
			filters[key] = filters[key].AddString(msg.transfer.To.String())

			tracing.FinishSpan(child)
		}

		out <- addressFilterRepo.BulkUpsert(ctx, filters)
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
		block:   msg.transfer.BlockNumber,
	}
}
