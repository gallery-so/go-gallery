package feedbot

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/shurcooL/graphql"
	"github.com/spf13/viper"
)

func Init() {
	setDefaults()

	sentryutil.InitSentry()
	logger.InitLogger()

	router := coreInit(postgres.NewClient())
	http.Handle("/", router)
}

func coreInit(pqClient *sql.DB) *gin.Engine {
	logger.For(nil).Info("initializing server...")

	router := gin.Default()
	router.Use(middleware.ErrLogger(), middleware.Sentry(true), middleware.Tracing())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
	}

	gql := graphql.NewClient(viper.GetString("GALLERY_API"), http.DefaultClient)

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
	viper.SetDefault("CHANNEL_ID", "977428719402627092")
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
