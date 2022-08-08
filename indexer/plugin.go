package indexer

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/sirupsen/logrus"
)

// PluginMsg is used to communicate to a plugin.
type PluginMsg struct {
	transfer rpc.Transfer
	key      persist.EthereumTokenIdentifiers
	wg       *sync.WaitGroup
}

// TransferPlugins are plugins that add contextual data to a transfer.
type TransferPlugins struct {
	uris       urisPlugin
	balances   balancesPlugin
	owners     ownersPlugin
	prevOwners ownersPlugin
}

// NewTransferPlugins returns a set of transfer plugins. Plugins have an `in` and `out` channel that are handles to the service.
// The `in` channel is used to submit a transfer to a plugin, and the `out` channel is used to receive results from a plugin.
// A plugin can be stopped by closing its `in` channel, which closes the plugin's `out` channel when finished to let receivers
// know that there are no more results.
func NewTransferPlugins(ctx context.Context, ethClient *ethclient.Client, tokenRepo persist.TokenRepository, storageClient *storage.Client) TransferPlugins {
	return TransferPlugins{
		uris:       newURIsPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, tokenRepo, storageClient),
		balances:   newBalancesPlugin(sentryutil.NewSentryHubContext(ctx), ethClient, storageClient),
		owners:     newOwnerPlugin(sentryutil.NewSentryHubContext(ctx)),
		prevOwners: newPreviousOwnerPlugin(sentryutil.NewSentryHubContext(ctx)),
	}
}

// RunPlugins returns when all plugins have finished.
func RunPlugins(ctx context.Context, transfer rpc.Transfer, key persist.EthereumTokenIdentifiers, plugins []chan<- PluginMsg) {
	var wg sync.WaitGroup
	wg.Add(len(plugins))
	msg := PluginMsg{
		transfer: transfer,
		key:      key,
		wg:       &wg,
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
		for msg := range in {
			defer msg.wg.Done()
			var uri persist.TokenURI

			ctx, cancel := context.WithTimeout(ctx, time.Second*3)
			defer cancel()

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
		defer close(out)
		for msg := range in {
			defer msg.wg.Done()

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
	out chan ownerAtBlock
}

// newOwnerPlugin returns a plugin that retrieves the current owner of a token.
func newOwnerPlugin(ctx context.Context) ownersPlugin {
	in := make(chan PluginMsg)
	out := make(chan ownerAtBlock)

	go func() {
		defer close(out)
		for msg := range in {
			defer msg.wg.Done()

			if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC721 {
				out <- ownerAtBlock{
					ti:    msg.key,
					owner: msg.transfer.To,
					block: msg.transfer.BlockNumber,
				}
			}
		}
	}()

	return ownersPlugin{
		in:  in,
		out: out,
	}
}

// newPreviousOwnerPlugin returns a plugin that retrieves the previous owner of a token.
func newPreviousOwnerPlugin(ctx context.Context) ownersPlugin {
	in := make(chan PluginMsg)
	out := make(chan ownerAtBlock)

	go func() {
		defer close(out)
		for msg := range in {
			defer msg.wg.Done()

			if persist.TokenType(msg.transfer.TokenType) == persist.TokenTypeERC721 {
				out <- ownerAtBlock{
					ti:    msg.key,
					owner: msg.transfer.From,
					block: msg.transfer.BlockNumber,
				}
			}
		}
	}()

	return ownersPlugin{
		in:  in,
		out: out,
	}
}
