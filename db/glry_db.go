package db

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
type DB struct {
	Mongo *mongo.Database
}

//-------------------------------------------------------------
func Init(pMongoHostStr string,
	pMongoDBNamestr string,
	pRuntimeSys     *gfcore.Runtime_sys) (*DB, *gfcore.Gf_error) {

	mongoURLstr  := fmt.Sprintf("mongodb://%s", pMongoHostStr)
	log.WithFields(log.Fields{
		"host":    pMongoHostStr,
		"db_name": pMongoDBNamestr,
	}).Info("Mongo conn info")

	//-------------------------------------------------------------
	// GF_GET_DB
	GFgetDBfun := func() (*mongo.Database, *gfcore.Gf_error) {

		mongoDB, gErr := gfcore.Mongo__connect_new(mongoURLstr,
			pMongoDBNamestr,
			pRuntimeSys)
		if gErr != nil {
			return nil, gErr
		}
		log.Info("mongodb connected...")
		
		return mongoDB, nil
	}

	//-------------------------------------------------------------
	mongoDB, gErr := GFgetDBfun()
	if gErr != nil {
		return nil, gErr
	}

	db := &DB{
		Mongo: mongoDB,
	}

	return db, nil
}

//-------------------------------------------------------------