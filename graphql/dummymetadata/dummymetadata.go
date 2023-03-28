package dummymetadata

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// InitServer initializes the dummymetadata server
func InitServer() {
	setDefaults()
	router := CoreInitServer()
	logger.For(nil).Info("Starting dummymetadata server...")
	http.Handle("/", router)
}

func CoreInitServer() *gin.Engine {
	logger.InitWithGCPDefaults()

	http.DefaultClient = &http.Client{Transport: tracing.NewTracingTransport(http.DefaultTransport, false)}

	router := gin.Default()

	router.Use(middleware.GinContextToContext(), middleware.Tracing(), middleware.HandleCORS(), middleware.ErrLogger())

	if viper.GetString("ENV") != "production" {
		gin.SetMode(gin.DebugMode)
		logrus.SetLevel(logrus.DebugLevel)
	}

	logger.For(nil).Info("Registering handlers...")

	return handlersInitServer(router)
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.AutomaticEnv()
}
