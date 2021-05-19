package main

import (
	// log "github.com/sirupsen/logrus"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	// "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/server"
)

//-------------------------------------------------------------
func main() {
	
	

	config := glry_core.LoadConfig()
	
	
	

	portStr := config.Port
	mongoDBhostStr := config.MongoHostStr
	mongoDBnameStr := config.MongoDBnameStr


	

	// RUNTIME
	runtime, gErr := glry_core.RuntimeGet(mongoDBhostStr, mongoDBnameStr, config)
	if gErr != nil {
		panic(gErr.Error)
	}

	// SERVER_INIT
	server.Init(portStr, runtime)
}
