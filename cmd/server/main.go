package main

import (
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/server"
)

func main() {

	config := runtime.ConfigLoad()
	portStr := config.Port

	// RUNTIME
	runtime, err := runtime.GetRuntime(config)
	if err != nil {
		panic(err.Error())
	}

	//-------------
	// SERVER_INIT
	server.Init(portStr, runtime)
}
