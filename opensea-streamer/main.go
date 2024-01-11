package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	itemTransferredEventType = "item_transferred"
	falsePositiveRate        = 0.01
)

// TODO add more chains here
// https://docs.opensea.io/reference/supported-chains#mainnets
var enabledChains = map[string]bool{
	"base": true,
	"zora": true,
}

type openseaEvent struct {
	Event   string                              `json:"event"`
	Payload tokenprocessing.OpenSeaWebhookInput `json:"payload"`
}

var bf atomic.Pointer[bloom.BloomFilter]

func main() {
	setDefaults()
	router := gin.Default()

	logger.InitWithGCPDefaults()

	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)

	ctx := context.Background()

	err := generateBloomFilter(ctx, queries)
	if err != nil {
		panic(err)
	}

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"approximate_bloom_size": bf.Load().ApproximatedSize(),
	})

	// Health endpoint
	router.GET("/health", util.HealthCheckHandler())

	router.GET("/updateBloomFilter", func(c *gin.Context) {
		err := generateBloomFilter(c, queries)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.String(http.StatusOK, "OK")
	})

	go func() {
		// check hourly if the bloom filter needs to be updated
		for {
			time.Sleep(time.Hour)
			logger.For(ctx).Info("checking if bloom filter needs to be updated...")

			err := generateBloomFilter(ctx, queries)
			if err != nil {
				panic(err)
			}
		}
	}()

	go streamOpenseaTransfers(ctx, false)

	router.Run(":3000")
}

func streamOpenseaTransfers(ctx context.Context, recursed bool) {

	apiKey := env.GetString("OPENSEA_API_KEY")
	// Set up WebSocket connection
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial("wss://stream.openseabeta.com/socket/websocket?token="+apiKey, nil)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Subscribe to events
	subscribeMessage := map[string]interface{}{
		"topic":   "collection:*",
		"event":   "phx_join",
		"payload": map[string]interface{}{},
		"ref":     0,
	}
	conn.WriteJSON(subscribeMessage)

	logger.For(ctx).Info("subscribed to opensea events")

	errChan := make(chan error)

	go func() {
		// Listen for messages
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				errChan <- err
				break
			}

			var oe openseaEvent
			err = json.Unmarshal(message, &oe)
			if err != nil {
				continue
			}

			win := oe.Payload

			if win.EventType != itemTransferredEventType {
				continue
			}

			if !enabledChains[win.Payload.Chain] {
				continue
			}

			// check if the wallet is in the bloom filter
			chainAddress, err := persist.NewL1ChainAddress(persist.Address(win.Payload.ToAccount.Address.String()), win.Payload.Item.NFTID.Chain).MarshalJSON()
			if err != nil {
				logger.For(ctx).Error(err)
				continue
			}
			if !bf.Load().Test(chainAddress) {
				continue
			}

			logger.For(ctx).Infof("received user item transfer event for wallet %s on chain %d", win.Payload.ToAccount.Address.String(), win.Payload.Item.NFTID.Chain)

			// send to token processing service
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				payloadJSON, err := json.Marshal(win)
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.GetString("TOKEN_PROCESSING_URL")+"/owners/process/opensea", bytes.NewBuffer(payloadJSON))
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", env.GetString("WEBHOOK_TOKEN"))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					logger.For(ctx).Error(err)
					return
				}

				if resp.StatusCode != http.StatusOK {
					logger.For(ctx).Errorf("non-200 response from token processing service: %d", resp.StatusCode)
					return
				}
			}()

		}
	}()

	// Heartbeat
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			heartbeat := map[string]interface{}{
				"topic":   "phoenix",
				"event":   "heartbeat",
				"payload": map[string]interface{}{},
				"ref":     0,
			}
			conn.WriteJSON(heartbeat)
			logger.For(ctx).Debug("Sent heartbeat")
		case err := <-errChan:
			if err != nil {
				if recursed {
					panic(err)
				} else {
					logger.For(ctx).Error(err)
					logger.For(ctx).Info("reconnecting to opensea...")
					streamOpenseaTransfers(ctx, true)
				}
			}
		}
	}
}

func generateBloomFilter(ctx context.Context, q *coredb.Queries) error {
	wallets, err := q.GetActiveWallets(ctx)
	if err != nil {
		return err
	}

	logger.For(nil).Infof("resetting bloom filter with %d wallets", len(wallets))

	bfp := bloom.NewWithEstimates(uint(len(wallets)), falsePositiveRate)
	for _, w := range wallets {
		chainAddress, err := persist.NewL1ChainAddress(w.Address, w.Chain).MarshalJSON()
		if err != nil {
			return err
		}
		bfp.Add(chainAddress)
	}

	buf := &bytes.Buffer{}
	_, err = bfp.WriteTo(buf)
	if err != nil {
		return err
	}

	bf.Store(bfp)

	return nil
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("WEBHOOK_TOKEN", "")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("SENTRY_TRACES_SAMPLE_RATE", 0.2)

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("opensea-streamer", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
	}
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
