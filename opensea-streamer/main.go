package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"sync"
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
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/tokenprocessing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const (
	bloomFilterKey           = "walletsBloomFilter"
	walletCountKey           = "walletCount"
	itemTransferredEventType = "item_transferred"
	falsePositiveRate        = 0.01
)

var enabledChains = map[string]bool{
	"base": true,
	"zora": true,
}

type openseaEvent struct {
	Event   string                              `json:"event"`
	Payload tokenprocessing.OpenSeaWebhookInput `json:"payload"`
}

func main() {
	setDefaults()
	router := gin.Default()

	logger.InitWithGCPDefaults()

	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)
	bloomCache := redis.NewCache(redis.WalletsBloomFilterCache)

	initCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bf, err := initializeBloomFilter(initCtx, queries, bloomCache)
	if err != nil {
		panic(err)
	}

	logger.For(nil).Info("Initialized bloom filter, starting opensea streamer server...")

	// Health endpoint
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	router.GET("/updateBloomFilter", func(c *gin.Context) {
		bf, err = resetBloomFilter(initCtx, queries, bloomCache)
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
			logger.For(nil).Info("checking if bloom filter needs to be updated...")
			curWalletCountBs, _ := bloomCache.Get(initCtx, walletCountKey)
			if curWalletCountBs == nil {
				curWalletCountBs = []byte("0")
			}
			var curWalletCount int64
			err = json.Unmarshal(curWalletCountBs, &curWalletCount)
			if err != nil {
				logger.For(nil).Error(err)
				continue
			}

			dbWalletCount, err := queries.CountActiveWallets(initCtx)
			if err != nil {
				logger.For(nil).Error(err)
				continue
			}

			if dbWalletCount == curWalletCount {
				continue
			}

			bf, err = resetBloomFilter(initCtx, queries, bloomCache)
			if err != nil {
				logger.For(nil).Error(err)
			}
		}
	}()

	go streamOpenseaTranfsers(bf)

	router.Run(":3000")
}

func streamOpenseaTranfsers(bf *bloom.BloomFilter) {

	apiKey := env.GetString("OPENSEA_API_KEY")
	logger.For(nil).Debugf("using opensea api key: %s", apiKey)
	// Set up WebSocket connection
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial("wss://stream.openseabeta.com/socket/websocket?token="+apiKey, nil)
	if err != nil {
		log.Fatal("Error connecting to WebSocket:", err)
		return
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

	logger.For(nil).Info("subscribed to opensea events")

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Listen for messages
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("Error reading message:", err)
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

			logger.For(nil).Debugf("Received valid message: %s\n", message)

			// check if the wallet is in the bloom filter
			chainAddress, err := persist.NewL1ChainAddress(persist.Address(win.Payload.FromAccount.Address.String()), win.Payload.Item.NFTID.Chain).MarshalJSON()
			if err != nil {
				logger.For(nil).Error(err)
				continue
			}
			if bf.Test(chainAddress) {
				logger.For(nil).Infof("received user item transfer event for wallet %s on chain %d", win.Payload.FromAccount.Address.String(), win.Payload.Item.NFTID.Chain)

				// send to token processing service
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
					defer cancel()
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.GetString("TOKEN_PROCESSING_URL")+"/owners/process/opensea", bytes.NewBuffer(message))
					if err != nil {
						logger.For(nil).Error(err)
						return
					}
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", env.GetString("WEBHOOK_TOKEN"))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						logger.For(nil).Error(err)
						return
					}

					if resp.StatusCode != http.StatusOK {
						logger.For(nil).Errorf("non-200 response from token processing service: %d", resp.StatusCode)
						return
					}
				}()
			}
		}
	}()

	go func() {
		defer wg.Done()
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
				logger.For(nil).Debug("Sent heartbeat")
			}
		}
	}()

	wg.Wait()
}

func resetBloomFilter(ctx context.Context, q *coredb.Queries, r *redis.Cache) (*bloom.BloomFilter, error) {
	wallets, err := q.GetActiveWallets(ctx)
	if err != nil {
		return nil, err
	}

	bf := bloom.NewWithEstimates(uint(len(wallets)), falsePositiveRate)
	for _, w := range wallets {
		chainAddress, err := persist.NewL1ChainAddress(w.Address, w.Chain).MarshalJSON()
		if err != nil {
			return nil, err
		}
		bf.Add(chainAddress)
	}

	buf := &bytes.Buffer{}
	_, err = bf.WriteTo(buf)
	if err != nil {
		return nil, err
	}

	err = r.Set(ctx, bloomFilterKey, buf.Bytes(), time.Hour*24*7)
	if err != nil {
		return nil, err
	}

	walletCountJSON, err := json.Marshal(len(wallets))
	if err != nil {
		return nil, err
	}

	err = r.Set(ctx, walletCountKey, walletCountJSON, time.Hour*24*7)
	if err != nil {
		return nil, err
	}

	return bf, nil
}

func initializeBloomFilter(ctx context.Context, q *coredb.Queries, r *redis.Cache) (*bloom.BloomFilter, error) {
	cur, err := r.Get(ctx, bloomFilterKey)
	if err == nil && cur != nil && len(cur) > 0 {
		curWalletCountBs, err := r.Get(ctx, walletCountKey)
		if err != nil {
			return nil, err
		}

		var curWalletCount int64
		err = json.Unmarshal(curWalletCountBs, &curWalletCount)
		if err != nil {
			return nil, err
		}

		dbWalletCount, err := q.CountActiveWallets(ctx)
		if err != nil {
			return nil, err
		}

		if dbWalletCount == curWalletCount {
			// convert cur from bytes to uint64 array
			var curUint64 []uint64
			for i := 0; i < len(cur); i += 8 {
				curUint64 = append(curUint64, binary.BigEndian.Uint64(cur[i:i+8]))
			}

			bf := bloom.From(curUint64, 4)
			return bf, nil
		}
	}

	return resetBloomFilter(ctx, q, r)
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
}

func InitSentry() {
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
