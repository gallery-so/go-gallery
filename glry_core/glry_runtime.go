package glry_core

import (
	"os"
	"fmt"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"github.com/go-playground/validator"
	"github.com/gloflow/gloflow/go/gf_core"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
type Runtime struct {
	Config     *GLRYconfig
	DB         *DB
	Validator  *validator.Validate
	RuntimeSys *gf_core.Runtime_sys
}

type DB struct {
	MongoClient *mongo.Client
	MongoDB     *mongo.Database
}

//-------------------------------------------------------------
func RuntimeGet(pMongoDBhostStr string,
	pMongoDBnameStr string,
	pConfig         *GLRYconfig) (*Runtime, *gf_core.Gf_error) {
	
	//------------------
	// LOGS
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
	log.SetLevel(log.DebugLevel)

	//------------------
	// RUNTIME_SYS
	runtimeSys := &gf_core.Runtime_sys{
		Service_name_str:            "gallery",
		Names_prefix_str:            "glry",
		Errors_send_to_mongodb_bool: true,
	}

	//------------------
	// ERRORS_SEND_TO_SENTRY
	if pConfig.SentryEndpointStr != "" {
		runtimeSys.Errors_send_to_sentry_bool = true
	}

	//------------------
	// DB

	var mongoUserStr string
	var mongoPassStr string
	if pConfig.AWSsecretsBool {
		secretsMap, gErr := ConfigGetAWSsecrets(pConfig.EnvStr, runtimeSys)
		if gErr != nil {
			return nil, gErr
		}
		mongoUserStr = secretsMap["glry_mongo_user"]
		mongoPassStr = secretsMap["glry_mongo_pass"]
	} else {
		mongoUserStr = pConfig.MongoUserStr
		mongoPassStr = pConfig.MongoPassStr
	}

	db, gErr := DBinit(pMongoDBhostStr,
		pMongoDBnameStr,
		mongoUserStr,
		mongoPassStr,
		runtimeSys)

	if gErr != nil {
		log.WithFields(log.Fields{
			"db_host": pMongoDBhostStr,
			"db_name": pMongoDBnameStr,
		}).Fatal("Error acquiring database connection")

		return nil, gErr
	}

	runtimeSys.Mongo_db = db.MongoDB

	//------------------
	// CHECK!! - is Validator threadsafe, so that it can be used
	//           by several (possibly concurrently) threads.
	validator := validator.New()

	// RUNTIME
	runtime := &Runtime{
		Config:     pConfig,
		// DB:         db,
		Validator:  validator,
		RuntimeSys: runtimeSys,
	}



	
	return runtime, nil
}

//-------------------------------------------------------------
func DBinit(pMongoHostStr string,
	pMongoDBNamestr string,
	pMongoUserStr string,
	pMongoPassStr string,
	pRuntimeSys   *gf_core.Runtime_sys) (*DB, *gf_core.Gf_error) {


	// AWS CONN STRING
	// mongodb://gallerydevmain:<insertYourPassword>@gallerydev.cluster-ckak4r22p2u9.us-east-1.docdb.amazonaws.com:27017?
	// 		ssl=true
	// 		ssl_ca_certs=rds-combined-ca-bundle.pem
	// 		replicaSet=rs0
	//		readPreference=secondaryPreferred
	// 		retryWrites=false
	
	var mongoURLstr string
	if pMongoUserStr != "" && pMongoPassStr != "" {

		mongoURLstr = fmt.Sprintf("mongodb://%s:%s@%s", pMongoUserStr,
			pMongoHostStr,
			pMongoPassStr)
	} else {
		mongoURLstr = fmt.Sprintf("mongodb://%s", pMongoHostStr)
	}


	log.WithFields(log.Fields{
		"host":    pMongoHostStr,
		"db_name": pMongoDBNamestr,
	}).Info("Mongo conn info")

	//-------------------------------------------------------------
	// GF_GET_DB
	GFgetDBfun := func() (*mongo.Database, *mongo.Client, *gf_core.Gf_error) {

		mongoDB, mongoClient, gErr := gf_core.Mongo__connect_new(mongoURLstr,
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