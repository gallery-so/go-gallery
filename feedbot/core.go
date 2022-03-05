package feedbot

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init() {
	router := coreInit(postgres.NewClient())
	http.Handle("/", router)
}

func coreInit(pqClient *sql.DB) *gin.Engine {
	setDefaults()
	router := gin.Default()
	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}
	return handlersInit(router, postgres.NewEventRepository(pqClient))

}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("AGENT_NAME", "DiscordBot (github.com/gallery-so, 0.0.1)")
	viper.SetDefault("DISCORD_API", "https://discord.com/api/v9")
	viper.SetDefault("CHANNEL_ID", "936895075076685845") // #gallery-feed-test channel
	viper.SetDefault("BOT_TOKEN", "")
	viper.SetDefault("PORT", 4123)
	viper.AutomaticEnv()
	if viper.GetString("BOT_TOKEN") == "" {
		panic("BOT_TOKEN must be set")
	}
}
