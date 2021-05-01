package config

import (
	// "fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

//-------------------------------------------------------------
const (
	appEnv      = "APP_ENV"
	baseURL     = "BASE_URL"
	webBaseURL  = "WEB_BASE_URL"
	port        = "PORT"
	mongoHost   = "MONGO_URI"
	mongoDBname = "MONGO_DB_NAME"
	// postgresURI = "POSTGRES_URI"
)

type Config struct {
	AppEnv         string
	BaseURL        string
	WebBaseURL     string
	Port           int
	MongoHostStr   string
	MongoDBnameStr string
	// PostgresURI string
}

//-------------------------------------------------------------
func LoadConfig() *Config {

	viper.SetDefault(appEnv, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(webBaseURL, "http://localhost:3000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(mongoHost, "")
	viper.SetDefault(mongoDBname, "")
	// viper.SetDefault(postgresURI, "")

	viper.SetConfigFile(".env")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"err": err,}).Fatal("Error reading in env file")
		panic(-1)
	}

	config := &Config{
		AppEnv:         viper.GetString(appEnv),
		BaseURL:        viper.GetString(baseURL),
		WebBaseURL:     viper.GetString(webBaseURL),
		Port:           viper.GetInt(port),
		MongoHostStr:   viper.GetString(mongoHost),
		MongoDBnameStr: viper.GetString(mongoDBname),
		// PostgresURI: viper.GetString(postgresURI),
	}
	return config
}
