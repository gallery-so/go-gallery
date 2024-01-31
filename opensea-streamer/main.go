package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"net/http"
	"os"
	"strings"
	"sync"
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
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const (
	itemTransferredEventType = "item_transferred"

	// At this false positive rate (1 out of every 1,000,000), the bloom filter uses approximately 3-4 bytes per wallet.
	falsePositiveRate    = 0.000001
	numConcurrentStreams = 3
)

// TODO add more chains here
// https://docs.opensea.io/reference/supported-chains#mainnets
var enabledChains = map[string]bool{
	"base": true,
	"zora": true,
}

type openseaEvent struct {
	Event   string                      `json:"event"`
	Payload persist.OpenSeaWebhookInput `json:"payload"`
}

var numConnections atomic.Int32
var bloomFilter atomic.Pointer[bloom.BloomFilter]

var mapLock = sync.Mutex{}
var seenEvents = make(map[string]bool)

func main() {
	setDefaults()
	initSentry()
	router := gin.Default()

	logger.InitWithGCPDefaults()

	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)

	ctx := context.Background()
	taskClient := task.NewClient(ctx)

	err := generateBloomFilter(ctx, queries)
	if err != nil {
		panic(err)
	}

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
		// update the bloom filter every 15 minutes
		for {
			time.Sleep(15 * time.Minute)
			logger.For(ctx).Info("updating bloom filter...")

			err := generateBloomFilter(ctx, queries)
			if err != nil {
				err := fmt.Errorf("error updating bloom filter: %w", err)
				logger.For(ctx).Error(err)
				sentryutil.ReportError(ctx, err)
			}
		}
	}()

	go func() {
		go streamOpenseaTransfers(ctx, taskClient, "stream 1", 1)

		for i := 2; i <= numConcurrentStreams; i++ {
			time.Sleep(2 * time.Minute)
			go streamOpenseaTransfers(ctx, taskClient, fmt.Sprintf("stream %d", i), i)
		}
	}()

	err = router.Run(":3000")
	if err != nil {
		err = fmt.Errorf("error running router: %w", err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		panic(err)
	}
}

func hashMessage(messageBytes []byte) string {
	hasher := sha256.New()
	hasher.Write(messageBytes)
	hashBytes := hasher.Sum(nil)
	return string(hashBytes)
}

// addSeenEvent adds the event to the seenEvents map and returns false if the event was already seen,
// true otherwise
func addSeenEvent(messageBytes []byte) bool {
	key := hashMessage(messageBytes)
	mapLock.Lock()
	defer mapLock.Unlock()
	if _, ok := seenEvents[key]; ok {
		return false
	}

	seenEvents[key] = true

	// Remove the event from the map after 30 seconds. We don't need to hold onto the keys forever;
	// we just want to deduplicate events across our websocket streams.
	go func() {
		time.Sleep(30 * time.Second)
		mapLock.Lock()
		defer mapLock.Unlock()
		delete(seenEvents, key)
	}()

	return true
}

func onConnect(ctx context.Context) {
	numConnections.Add(1)
}

func onDisconnect(ctx context.Context) {
	current := numConnections.Add(-1)
	if current == 0 {
		logger.For(ctx).Errorf("there are no active websocket connections. events will be missed until we reconnect...")
	}
}

func openWebsocket(ctx context.Context, streamName string, refID int) *websocket.Conn {
	apiKey := env.GetString("OPENSEA_API_KEY")

	for {
		var dialer *websocket.Dialer

		conn, _, err := dialer.Dial("wss://stream.openseabeta.com/socket/websocket?token="+apiKey, nil)
		if err != nil {
			logger.For(ctx).Errorf("[%s] error connecting to opensea: %s", streamName, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Subscribe to events
		subscribeMessage := map[string]interface{}{
			"topic":   "collection:*",
			"event":   "phx_join",
			"payload": map[string]interface{}{},
			"ref":     refID,
		}

		if err := conn.WriteJSON(subscribeMessage); err != nil {
			conn.Close()
			logger.For(ctx).Errorf("[%s] error subscribing to collection updates: %s", streamName, err)
			time.Sleep(5 * time.Second)
			continue
		}

		onConnect(ctx)
		return conn
	}
}

func dispatchToTokenProcessing(ctx context.Context, taskClient *task.Client, payload persist.OpenSeaWebhookInput) {
	err := taskClient.CreateTaskForOpenseaStreamerTokenProcessing(ctx, payload)
	if err != nil {
		err = fmt.Errorf("error creating task for token processing: %w", err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
	}

	return
	
	//ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	//defer cancel()
	//
	//payloadJSON, err := json.Marshal(payload)
	//if err != nil {
	//	err = fmt.Errorf("error marshaling payload: %w", err)
	//	logger.For(ctx).Error(err)
	//	sentryutil.ReportError(ctx, err)
	//	return
	//}
	//
	//req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.GetString("TOKEN_PROCESSING_URL")+"/owners/process/opensea", bytes.NewBuffer(payloadJSON))
	//if err != nil {
	//	logger.For(ctx).Error(err)
	//	return
	//}
	//
	//req.Header.Set("Content-Type", "application/json")
	//req.Header.Set("Authorization", env.GetString("WEBHOOK_TOKEN"))
	//
	//resp, err := http.DefaultClient.Do(req)
	//if err != nil {
	//	logger.For(ctx).Error(err)
	//	return
	//}
	//
	//if resp.StatusCode != http.StatusOK {
	//	logger.For(ctx).Errorf("non-200 response from token processing service: %d", resp.StatusCode)
	//	return
	//}
}

func sendHeartbeat(ctx context.Context, conn *websocket.Conn, streamName string, refID int) {
	heartbeat := map[string]interface{}{
		"topic":   "phoenix",
		"event":   "heartbeat",
		"payload": map[string]interface{}{},
		"ref":     refID,
	}

	err := conn.WriteJSON(heartbeat)
	if err != nil {
		logger.For(ctx).Errorf("[%s] error sending heartbeat: %s", streamName, err)
	}

	//logger.For(ctx).Debug("Sent heartbeat")
}

func streamOpenseaTransfers(ctx context.Context, taskClient *task.Client, streamName string, refID int) {
	conn := openWebsocket(ctx, streamName, refID)
	defer conn.Close()

	logger.For(ctx).Infof("[%s] subscribed to opensea events", streamName)

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
				// No need to log these errors; they're expected and spammy. But if we see any other errors, we want to know about them!
				errStr := err.Error()
				if !strings.HasPrefix(errStr, "invalid opensea chain") &&
					!strings.HasPrefix(errStr, "json: cannot unmarshal object into Go struct field .payload.payload.chain of type string") &&
					!strings.HasPrefix(errStr, "json: cannot unmarshal number") {
					logger.For(ctx).Errorf("[%s] unmarshaling error: %s", streamName, err)
				}
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
				err = fmt.Errorf("error marshaling chain address: %w", err)
				logger.For(ctx).Error(err)
				continue
			}

			if !bloomFilter.Load().Test(chainAddress) {
				continue
			}

			logger.For(ctx).Infof("[%s] received user item transfer event (%d bytes) for token (contract=%s, tokenID=%s) transferred to wallet %s on chain %d",
				streamName, len(message), win.Payload.Item.NFTID.ContractAddress.String(), win.Payload.Item.NFTID.TokenID.String(), win.Payload.ToAccount.Address.String(), win.Payload.Item.NFTID.Chain)

			if !addSeenEvent(message) {
				continue
			}

			// send to token processing service
			go dispatchToTokenProcessing(ctx, taskClient, win)
		}
	}()

	// Per OpenSea docs, we need to send a heartbeat every 30 seconds
	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			sendHeartbeat(ctx, conn, streamName, refID)
		case err := <-errChan:
			if err != nil {
				onDisconnect(ctx)
				logger.For(ctx).Errorf("[%s] encountered error: %s", streamName, err)
				logger.For(ctx).Infof("[%s] reconnecting to opensea...", streamName)
				go streamOpenseaTransfers(ctx, taskClient, streamName, refID)
				return
			}
		}
	}
}

func generateBloomFilter(ctx context.Context, q *coredb.Queries) error {
	wallets, err := q.GetActiveWallets(ctx)
	if err != nil {
		return err
	}

	logger.For(ctx).Infof("resetting bloom filter with %d wallets", len(wallets))

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

	bloomFilter.Store(bfp)

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
