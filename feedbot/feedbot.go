package feedbot

import (
	"database/sql"
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/shurcooL/graphql"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	initSentry()
	initLogger()

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

	gql := graphql.NewClient(viper.GetString("GALLERY_API"), nil)

	repos := persist.Repositories{
		UserEventRepository:       postgres.NewUserEventRepository(pqClient),
		NftEventRepository:        postgres.NewNftEventRepository(pqClient),
		CollectionEventRepository: postgres.NewCollectionEventRepository(pqClient),
	}

	return handlersInit(router, repos, gql)
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
	viper.SetDefault("GALLERY_API", "http://localhost:4000/glry/graphql/query")
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

func initLogger() {
	logger.SetLoggerOptions(func(logger *log.Logger) {
		logger.SetReportCaller(true)

		if viper.GetString("ENV") != "production" {
			logger.SetLevel(log.DebugLevel)
		}

		if viper.GetString("ENV") == "local" {
			logger.SetFormatter(&log.TextFormatter{DisableQuote: true})
		} else {
			// Use a JSONFormatter for non-local environments because Google Cloud Logging works well with JSON-formatted log entries
			logger.SetFormatter(&log.JSONFormatter{})
		}
	})
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
					scrubbed[k] = "[filtered]"
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
