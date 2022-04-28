package feedbot

import (
	"database/sql"
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()
	initSentry()
	router := coreInit(postgres.NewClient())
	http.Handle("/", router)
}

func coreInit(pqClient *sql.DB) *gin.Engine {
	router := gin.Default()
	router.Use(middleware.ErrLogger(), sentrygin.New(sentrygin.Options{Repanic: true}))

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		log.SetLevel(log.DebugLevel)
	}
	return handlersInit(router, postgres.NewUserRepository(pqClient), postgres.NewUserEventRepository(pqClient), postgres.NewNftEventRepository(pqClient), postgres.NewCollectionEventRepository(pqClient))
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("AGENT_NAME", "DiscordBot (github.com/gallery-so, 0.0.1)")
	viper.SetDefault("DISCORD_API", "https://discord.com/api/v9")
	viper.SetDefault("CHANNEL_ID", "936895075076685845") // #gallery-feed-test channel
	viper.SetDefault("BOT_TOKEN", "")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("PORT", 4123)
	viper.SetDefault("GALLERY_HOST", "http://localhost:3000")
	viper.SetDefault("FEEDBOT_SECRET", "feed-bot-secret")
	viper.SetDefault("SENTRY_DSN", "")
	viper.AutomaticEnv()

	if viper.GetString("BOT_TOKEN") == "" {
		panic("BOT_TOKEN must be set")
	}

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}
}

func initSentry() {
	if viper.GetString("ENV") == "local" {
		log.Info("skipping sentry init")
		return
	}

	log.Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              viper.GetString("SENTRY_DSN"),
		Environment:      viper.GetString("ENV"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if event.Request == nil {
				return event
			}

			scrubbed := map[string]string{}
			for k, v := range event.Request.Headers {
				if k == "Authorization" {
					scrubbed[k] = "[Filtered]"
				} else {
					scrubbed[k] = v
				}
			}

			event.Request.Headers = scrubbed
			return event
		},
	})

	if err != nil {
		log.Fatalf("failed to start sentry: %s", err)
	}
}
