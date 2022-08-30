package indexer

import (
	"context"

	"cloud.google.com/go/storage"
	"github.com/bits-and-blooms/bloom"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
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

func startSpan(ctx context.Context, pluginName string) (*sentry.Span, context.Context) {
	return tracing.StartSpan(ctx, "indexer.runPlugin", pluginName)
}

// NewTransferPlugins returns a set of transfer plugins. Plugins have an `in` and an optional `out` channel that are handles to the service.
// The `in` channel is used to submit a transfer to a plugin, and the `out` channel is used to receive results from a plugin, if any.
// A plugin can be stopped by closing its `in` channel, which closes the plugin and lets receivers know that there are no more results.
func NewTransferPlugins(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, blockFilterRepo postgres.BlockFilterRepository, storageClient *storage.Client) TransferPlugins {
	return TransferPlugins{
		uris:     newURIsPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo, storageClient),
		balances: newBalancesPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, storageClient),
		owners:   newOwnerPlugin(sentryutil.NewSentryHubContext(ctx)),
		refresh:  newRefreshPlugin(sentryutil.NewSentryHubContext(ctx), blockFilterRepo),
	}
}

// RunPlugins returns when all plugins have finished.
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

// urisPlugin pulls URI information for a token.
type urisPlugin struct {
	in  chan PluginMsg
	out chan tokenURI
}

func newURIsPlugin(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, storageClient *storage.Client) urisPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenURI)

	go func() {
		defer close(out)
		span, ctx := startSpan(ctx, "uriPlugin")
		defer tracing.FinishSpan(span)

		for msg := range in {
			child := span.StartChild("handleMessage")

			var uri persist.TokenURI

			ct, tid, err := msg.key.GetParts()
			if err != nil {
				logger.For(ctx).WithError(err).WithFields(logrus.Fields{
					"fromAddress": msg.transfer.From,
					"tokenKey":    msg.key,
					"block":       msg.transfer.BlockNumber,
				}).Errorf("error getting parts of %s", msg.key)
				storeErr(ctx, err, "ERR-PARTS", msg.transfer.From, msg.key, msg.transfer.BlockNumber, storageClient)
				panic(err)
			}
			dbURI, _, _, err := tokenRepo.GetMetadataByTokenIdentifiers(ctx, tid, ct)
			if err == nil {
				if dbURI != "" {
					uri = dbURI
				}
			}

			if uri == "" {
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

func newBalancesPlugin(ctx context.Context, ethClient *ethclient.Client, storageClient *storage.Client) balancesPlugin {
	in := make(chan PluginMsg)
	out := make(chan tokenBalances)

	go func() {
		span, ctx := startSpan(ctx, "balancePlugin")
		defer tracing.FinishSpan(span)
		defer close(out)

		for msg := range in {
			child := span.StartChild("handleMessage")

			if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC1155 {
				bals, err := getBalances(ctx, msg.transfer.ContractAddress, msg.transfer.From, msg.transfer.TokenID, msg.key, msg.transfer.BlockNumber, msg.transfer.To, ethClient)
				if err != nil {
					logger.For(ctx).WithError(err).WithFields(logrus.Fields{
						"fromAddress":     msg.transfer.From,
						"tokenIdentifier": msg.key,
						"block":           msg.transfer.BlockNumber,
					}).Errorf("error getting balance of %s for %s", msg.transfer.From, msg.key)
					storeErr(ctx, err, "ERR-BALANCE", msg.transfer.From, msg.key, msg.transfer.BlockNumber, storageClient)
				}
				out <- bals
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
	in chan PluginMsg
}

func newRefreshPlugin(ctx context.Context, blockFilterRepo postgres.BlockFilterRepository) refreshPlugin {
	in := make(chan PluginMsg)

	go func() {
		span, _ := startSpan(ctx, "refreshPlugin")
		defer tracing.FinishSpan(span)

		filters := make(map[[2]persist.BlockNumber]*bloom.BloomFilter)

		for msg := range in {
			child := span.StartChild("handleMessage")

			fromBlock := msg.transfer.BlockNumber - (msg.transfer.BlockNumber % blocksPerLogsCall)
			toBlock := fromBlock + blocksPerLogsCall
			key := [2]persist.BlockNumber{fromBlock, toBlock}

			if _, ok := filters[key]; !ok {
				filters[key] = bloom.NewWithEstimates(bloomFilterSize, falsePositiveRate)
			}

			filters[key] = filters[key].AddString(msg.transfer.From.String())
			filters[key] = filters[key].AddString(msg.transfer.To.String())

			tracing.FinishSpan(child)
		}

		if err := blockFilterRepo.BulkUpsert(ctx, filters); err != nil {
			logger.For(ctx).WithError(err).Error("failed to save block filters")
		}
	}()

	return refreshPlugin{
		in: in,
	}
}
