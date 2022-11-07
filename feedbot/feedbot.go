package feedbot

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	initSentry()
	logger.InitWithGCPDefaults()

	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {
	logger.For(nil).Info("initializing server...")

	router := gin.Default()
	router.Use(middleware.ErrLogger(), middleware.Sentry(true), middleware.Tracing())

	gql := graphql.NewClient(viper.GetString("GALLERY_API"), http.DefaultClient)

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	return handlersInit(router, gql)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("AGENT_NAME", "DiscordBot (github.com/gallery-so, 0.0.1)")
	viper.SetDefault("DISCORD_API", "https://discord.com/api/v9")
	viper.SetDefault("CHANNEL_ID", "977428719402627092")
	viper.SetDefault("BOT_TOKEN", "")
	viper.SetDefault("GALLERY_HOST", "http://localhost:3000")
	viper.SetDefault("GALLERY_API", "http://localhost:4000/glry/graphql/query")
	viper.SetDefault("FEEDBOT_SECRET", "feed-bot-secret")
	viper.SetDefault("SENTRY_DSN", "")

	viper.AutomaticEnv()

	util.EnvVarMustExist("BOT_TOKEN", "")
	if viper.GetString("ENV") != "local" {
		util.EnvVarMustExist("SENTRY_DSN", "")
	}
}

func initSentry() {
	if viper.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              viper.GetString("SENTRY_DSN"),
		Environment:      viper.GetString("ENV"),
		TracesSampleRate: viper.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = sentryutil.ScrubEventHeaders(event, hint)
			event = sentryutil.UpdateErrorFingerprints(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
