package main

import (
	"fmt"

	"github.com/spf13/viper"
)

const (
	appEnv           = "APP_ENV"
	baseURL          = "BASE_URL"
	webBaseURL       = "WEB_BASE_URL"
	port             = "PORT"
)

type applicationConfig struct {
	AppEnv           string
	BaseURL          string
	WebBaseURL       string
	Port             string
}

func getApplicationConfig() applicationConfig {
	setDefaultConfig()

	loadEnv()

	return applicationConfig{
		AppEnv:           viper.GetString(appEnv),
		BaseURL:          viper.GetString(baseURL),
		WebBaseURL:       viper.GetString(webBaseURL),
		Port:             viper.GetString(port),
	}
}

func setDefaultConfig() {
	viper.SetDefault(appEnv, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(webBaseURL, "http://localhost:3000")
	viper.SetDefault(port, "4000")
}

func loadEnv() {
	viper.SetConfigFile(".env")
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("Error reading in env file: %s", err))
	}
}