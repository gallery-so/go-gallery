package main

import (
	"os"
	log "github.com/sirupsen/logrus"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/core"
	"github.com/mikeydub/go-gallery/config"
	// "github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/server"
)

//-------------------------------------------------------------
func main() {
	
	log.SetOutput(os.Stdout)
	
	cfg := config.LoadConfig()
	
	
	

	portStr := cfg.Port
	mongoDBhostStr := cfg.MongoHostStr
	mongoDBnameStr := cfg.MongoDBnameStr


	

	// RUNTIME
	runtime, gErr := core.RuntimeGet(mongoDBhostStr, mongoDBnameStr)
	if gErr != nil {
		panic(gErr.Error)
	}

	// SERVER_INIT
	server.Init(portStr, runtime)
}
