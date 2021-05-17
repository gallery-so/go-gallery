package glry_core

import (
	"os"
	"fmt"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"github.com/go-playground/validator"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
type Runtime struct {
	DB         *DB
	Validator  *validator.Validate
	RuntimeSys *gfcore.Runtime_sys
}

type DB struct {
	MongoClient *mongo.Client
	MongoDB     *mongo.Database
}

//-------------------------------------------------------------
func RuntimeGet(pMongoDBhostStr string,
	pMongoDBnameStr string) (*Runtime, *gfcore.Gf_error) {
	
	//------------------
	// LOGS
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
	log.SetLevel(log.DebugLevel)

	//------------------

	// RUNTIME_SYS
	runtimeSys := &gfcore.Runtime_sys{
		Service_name_str: "gallery",
	}

	// DB
	db, gErr := DBinit(pMongoDBhostStr, pMongoDBnameStr, runtimeSys)
	if gErr != nil {
		log.WithFields(log.Fields{
			"db_host": pMongoDBhostStr,
			"db_name": pMongoDBnameStr,
		}).Fatal("Error acquiring database connection")

		return nil, gErr
	}

	runtimeSys.Mongo_db = db.MongoDB

	// CHECK!! - is Validator threadsafe, so that it can be used
	//           by several (possibly concurrently) threads.
	validator := validator.New()

	// RUNTIME
	runtime := &Runtime{
		DB:         db,
		Validator:  validator,
		RuntimeSys: runtimeSys,
	}



	
	return runtime, nil
}

//-------------------------------------------------------------
func DBinit(pMongoHostStr string,
	pMongoDBNamestr string,
	pRuntimeSys     *gfcore.Runtime_sys) (*DB, *gfcore.Gf_error) {

	mongoURLstr := fmt.Sprintf("mongodb://%s", pMongoHostStr)
	log.WithFields(log.Fields{
		"host":    pMongoHostStr,
		"db_name": pMongoDBNamestr,
	}).Info("Mongo conn info")

	//-------------------------------------------------------------
	// GF_GET_DB
	GFgetDBfun := func() (*mongo.Database, *mongo.Client, *gfcore.Gf_error) {

		mongoDB, mongoClient, gErr := gfcore.Mongo__connect_new(mongoURLstr,
			pMongoDBNamestr,
			pRuntimeSys)
		if gErr != nil {
			return nil, nil, gErr
		}
		log.Info("mongodb connected...")
		
		return mongoDB, mongoClient, nil
	}

	//-------------------------------------------------------------
	mongoDB, mongoClient, gErr := GFgetDBfun()
	if gErr != nil {
		return nil, gErr
	}

	db := &DB{
		MongoClient: mongoClient,
		MongoDB:     mongoDB,
	}

	return db, nil
}

//-------------------------------------------------------------