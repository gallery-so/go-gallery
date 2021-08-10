package main

import (
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/server"
)

//-------------------------------------------------------------
func main() {

	config := runtime.ConfigLoad()
	portStr := config.Port

	// RUNTIME
	runtime, gErr := runtime.GetRuntime(config)
	if gErr != nil {
		panic(gErr.Error)
	}

	//-------------
	// SERVER_INIT
	server.Init(portStr, runtime)
}
