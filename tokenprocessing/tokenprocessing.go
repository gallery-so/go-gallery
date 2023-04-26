package tokenprocessing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/server"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// InitServer initializes the mediaprocessing server
func InitServer() {
	setDefaults()
	c := server.ClientInit(context.Background())
	provider := server.NewMultichainProvider(c)
	router := CoreInitServer(c, provider)
	logger.For(nil).Info("Starting tokenprocessing server...")
	http.Handle("/", router)
}

func CoreInitServer(c *server.Clients, mc *multichain.Provider) *gin.Engine {
	initSentry()
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Sentry(true), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if env.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	t := newThrottler()

	return handlersInitServer(router, mc, c.Repos, c.EthClient, c.IPFSClient, c.ArweaveClient, c.StorageClient, env.GetString("GCLOUD_TOKEN_CONTENT_BUCKET"), t)
}

func setDefaults() {
	viper.SetDefault("IPFS_URL", "https://gallery.infura-ipfs.io")
	viper.SetDefault("IPFS_API_URL", "https://ipfs.infura.io:5001")
	viper.SetDefault("IPFS_PROJECT_ID", "")
	viper.SetDefault("IPFS_PROJECT_SECRET", "")
	viper.SetDefault("CHAIN", 0)
	viper.SetDefault("ENV", "local")
	viper.SetDefault("GCLOUD_TOKEN_LOGS_BUCKET", "dev-eth-token-logs")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "dev-token-content")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("IMGIX_API_KEY", "")
	viper.SetDefault("VERSION", "")
	viper.SetDefault("ALCHEMY_API_URL", "")

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("tokenprocessing", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
		util.VarNotSetTo("VERSION", "")
	}
}

func newThrottler() *throttle.Locker {
	return throttle.NewThrottleLocker(redis.NewCache(redis.TokenProcessingThrottleDB), time.Minute*30)
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
		Release:          env.GetString("VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			event = updateMediaProccessingFingerprints(event, hint)
			event = excludeTokenSpam(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}

func updateMediaProccessingFingerprints(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event == nil || event.Exception == nil || hint == nil {
		return event
	}

	var mediaErr media.MediaProcessingError

	if errors.As(hint.OriginalException, &mediaErr) {
		if event.Tags["chain"] == "" || event.Tags["contractAddress"] == "" {
			return event
		}

		// Add both errors to the stack

		if mediaErr.AnimationError != nil {
			event.Exception = append(event.Exception, sentry.Exception{
				Type:  fmt.Sprintf("%T", mediaErr.AnimationError),
				Value: mediaErr.AnimationError.Error(),
			})
		}

		if mediaErr.ImageError != nil {
			event.Exception = append(event.Exception, sentry.Exception{
				Type:  fmt.Sprintf("%T", mediaErr.ImageError),
				Value: mediaErr.ImageError.Error(),
			})
		}

		// Move the original error to the end of the stack since the latest error is used as the title in Sentry
		if len(event.Exception) > 1 {
			title := fmt.Sprintf("chain=%s address=%s", event.Tags["chain"], event.Tags["contractAddress"])
			event.Exception = append(event.Exception[1:], event.Exception[0])
			event.Exception[len(event.Exception)-1].Type = title
		}

		// Group by the chain and contract
		event.Fingerprint = []string{event.Tags["chain"], event.Tags["contractAddress"]}
	}

	return event
}

func excludeTokenSpam(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event.Contexts[sentryutil.TokenContextName]["IsSpam"].(bool) == true {
		return nil
	}
	return event
}
