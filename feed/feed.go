package feed

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	sentryutil.InitSentry()
	logger.InitLogger()

	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {
	logger.For(nil).Info("initializing server...")

	router := gin.Default()
	router.Use(middleware.ErrLogger(), middleware.Sentry(true), middleware.Tracing())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	return handlersInit(router)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("PORT", 4123)
	viper.SetDefault("FEED_SECRET", "feed-secret")
	viper.SetDefault("SENTRY_DSN", "")
	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" && viper.GetString("SENTRY_DSN") == "" {
		panic("SENTRY_DSN must be set")
	}
}
