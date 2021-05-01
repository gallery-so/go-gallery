package main

import (
	"os"
	log "github.com/sirupsen/logrus"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/config"
	"github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/server"
	
)

//-------------------------------------------------------------
func main() {
	
	log.SetOutput(os.Stdout)
	
	cfg := config.LoadConfig()
	
	
	runtimeSys := &gfcore.Runtime_sys{
		Service_name_str: "gallery",
	}

	portStr := cfg.Port
	mongoDBhostStr := cfg.MongoHostStr
	mongoDBnameStr := cfg.MongoDBnameStr


	db, gErr := db.Init(mongoDBhostStr, mongoDBnameStr, runtimeSys)
	if gErr != nil {
		log.WithFields(log.Fields{
			"db_host": mongoDBhostStr,
			"db_name": mongoDBnameStr,
		}).Fatal("Error acquiring database connection")

		panic(gErr.Error)
	}

	server.Init(portStr, db, runtimeSys)
}
