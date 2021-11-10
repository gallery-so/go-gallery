package analytics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Init initializes the server
func init() {
	router := CoreInit()

	http.Handle("/", router)
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit() *gin.Engine {
	logrus.Info("initializing server...")

	logrus.SetReportCaller(true)

	setDefaults()

	router := gin.Default()
	router.Use(middleware.CORS(), middleware.ErrLogger())

	return handlersInit(router)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("MONGO_URL", "mongodb://localhost:27017/")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" && viper.GetString("ADMIN_PASS") == "TEST_ADMIN_PASS" {
		panic("ADMIN_PASS must be set")
	}
}
