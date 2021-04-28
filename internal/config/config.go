package config

import (
	"fmt"

	"github.com/spf13/viper"
)

//-------------------------------------------------------------
const (
	appEnv      = "APP_ENV"
	baseURL     = "BASE_URL"
	webBaseURL  = "WEB_BASE_URL"
	port        = "PORT"
	postgresURI = "POSTGRES_URI"
)

type Config struct {
	AppEnv      string
	BaseURL     string
	WebBaseURL  string
	Port        int
	PostgresURI string
}

//-------------------------------------------------------------
func LoadConfig() *Config {
	viper.SetDefault(appEnv, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(webBaseURL, "http://localhost:3000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(postgresURI, "")

	viper.SetConfigFile(".env")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("Error reading in env file: %s", err))
	}

	return &Config{
		AppEnv:      viper.GetString(appEnv),
		BaseURL:     viper.GetString(baseURL),
		WebBaseURL:  viper.GetString(webBaseURL),
		Port:        viper.GetInt(port),
		PostgresURI: viper.GetString(postgresURI),
	}
}
