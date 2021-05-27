package glry_core

import (
	"os"
	"fmt"
	"os/exec"
	"io/ioutil"
	"crypto/x509"
	"crypto/tls"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"github.com/go-playground/validator"
	"github.com/gloflow/gloflow/go/gf_core"
	"github.com/davecgh/go-spew/spew"
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
func RuntimeGet(pConfig *GLRYconfig) (*Runtime, *gf_core.Gf_error) {
	



	


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



	// DBgetCustomTLSConfig(pConfig.MongoSslCAfilePathStr, runtimeSys)



	//------------------
	// ERRORS_SEND_TO_SENTRY
	if pConfig.SentryEndpointStr != "" {
		runtimeSys.Errors_send_to_sentry_bool = true
	}

	//------------------
	// DB

	var mongoURLstr           string
	// var mongoSslCAfilePathStr string
	// var mongoUserStr string
	// var mongoPassStr string

	if pConfig.AWSsecretsBool {
		
		log.WithFields(log.Fields{
			"env": pConfig.EnvStr,
		}).Info("Loading Mongo params from AWS Secrets Manager")

		secretsMap, gErr := ConfigGetAWSsecrets(pConfig.EnvStr, runtimeSys)
		if gErr != nil {
			return nil, gErr
		}

		// MONGO_URL
		mongoURLstr = secretsMap["glry_mongo_url"]["main"].(string)



		fmt.Println("==============")
		spew.Dump(secretsMap)


		/*//------------------
		// MONGO_SSL_CA_FILE
		mongoSslCAfilePathStr = "./glry_mongo_ssl_ca_file.pem"
		mongoSslCAbase64str := secretsMap["glry_mongo_ssl_ca_file"]["main"].(string)



		


		err = ioutil.WriteFile(mongoSslCAfilePathStr, []byte(mongoSslCAstr), 0644)
		if err != nil {
			panic(err)
		}

		//------------------*/

	} else {
		mongoURLstr           = pConfig.MongoURLstr
		// mongoSslCAfilePathStr = pConfig.MongoSslCAfilePathStr
	}

	mongoDBnameStr := pConfig.MongoDBnameStr

	db, gErr := DBinit(mongoURLstr,
		mongoDBnameStr,
		pConfig,
		runtimeSys)

	if gErr != nil {
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
func DBinit(pMongoURLstr string,
	pMongoDBNamestr string,
	pConfig         *GLRYconfig,
	pRuntimeSys     *gf_core.Runtime_sys) (*DB, *gf_core.Gf_error) {


	// AWS CONN STRING
	// mongodb://gallerydevmain:<insertYourPassword>@host:27017?
	// 		ssl=true
	// 		ssl_ca_certs=rds-combined-ca-bundle.pem
	// 		replicaSet=rs0
	//		readPreference=secondaryPreferred
	// 		retryWrites=false
	
	log.WithFields(log.Fields{}).Info("mongo connecting...")

	//-------------------------------------------------------------
	// GF_GET_DB
	GFgetDBfun := func() (*mongo.Database, *mongo.Client, *gf_core.Gf_error) {


		// wget https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem


		fmt.Println("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
		fmt.Println(pConfig.MongoSslCAfilePathStr)
		cmd := exec.Command("wget", "https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout

		err := cmd.Run()
		if err != nil {
			panic(err)
		}


		CAfilePathStr := "rds-combined-ca-bundle.pem"

		// TLS_CONFIG
		TLSconfig, gErr := DBgetCustomTLSConfig(CAfilePathStr, pRuntimeSys)
		if gErr != nil {
			return nil, nil, gErr
		}

		mongoDB, mongoClient, gErr := gf_core.Mongo__connect_new(pMongoURLstr,
			pMongoDBNamestr,
			TLSconfig,
			pRuntimeSys)
		if gErr != nil {
			return nil, nil, gErr
		}
		log.Info("mongo connected...")
		
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
func DBgetCustomTLSConfig(pCAfilePathStr string,
	pRuntimeSys *gf_core.Runtime_sys) (*tls.Config, *gf_core.Gf_error) {

	
	certs, err := ioutil.ReadFile(pCAfilePathStr)

	if err != nil {
		gErr := gf_core.Error__create("failed to read local CA file for mongo TLS connection",
			"file_read_error",
			map[string]interface{}{
				"ca_file_path": pCAfilePathStr,
			}, err, "glry_core", pRuntimeSys)
		return nil, gErr
	}

	tlsConfig        := new(tls.Config)
	tlsConfig.RootCAs = x509.NewCertPool()

	ok := tlsConfig.RootCAs.AppendCertsFromPEM(certs)
	if !ok {
		gErr := gf_core.Error__create("failed to parse local CA file for mongo TLS connection",
			"crypto_cert_ca_parse",
			map[string]interface{}{
				"ca_file_path": pCAfilePathStr,
			}, nil, "glry_core", pRuntimeSys)
		return nil, gErr
	}
	


	fmt.Println("###########################################")
	spew.Dump(tlsConfig)


	return tlsConfig, nil
}

