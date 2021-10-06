package main

import (
	"github.com/mikeydub/go-gallery/server"
	"github.com/spf13/viper"
)

func main() {
	port := viper.GetString("SERVER_PORT")
	server.Init(port)
}
