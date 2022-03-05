package feedbot

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func Init() {
	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {
	setDefaults()
	router := gin.Default()
	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}
	return handlersInit(router)
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
