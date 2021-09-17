package main

import (
	"github.com/mikeydub/go-gallery/infra"
	"github.com/mikeydub/go-gallery/runtime"
)

func main() {

	config := runtime.ConfigLoad()
	portStr := config.InfraPort

	// RUNTIME
	runtime, err := runtime.GetRuntime(config)
	if err != nil {
		panic(err.Error())
	}

	//-------------
	// SERVER_INIT
	infra.Init(portStr, runtime)

}
