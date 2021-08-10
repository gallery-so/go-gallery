package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/gin-gonic/gin"
)

//-------------------------------------------------------------
type Runtime struct {
	Config *Config
	DB     *DB
	Router *gin.Engine
}

type DB struct {
	MongoClient *mongo.Client
	MongoDB     *mongo.Database
}

//-------------------------------------------------------------
func GetRuntime(pConfig *Config) (*Runtime, error) {

	//------------------
	// LOGS
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
	log.SetLevel(log.DebugLevel)

	//------------------
	// DB

	mongoURLstr := pConfig.MongoURLstr

	mongoDBnameStr := pConfig.MongoDBnameStr

	db, err := dbInit(mongoURLstr,
		mongoDBnameStr,
		pConfig)

	if err != nil {
		return nil, err
	}

	err = setupMongoIndexes(db.MongoDB)
	if err != nil {
		return nil, err
	}

	// RUNTIME
	runtime := &Runtime{
		Config: pConfig,
		DB:     db,
	}

	return runtime, nil
}

//-------------------------------------------------------------
func dbInit(pMongoURLstr string,
	pMongoDBNamestr string,
	pConfig *Config) (*DB, error) {

	log.WithFields(log.Fields{}).Info("connecting to mongo...")

	// TODO secret name
	tlsCerts, err := accessSecret(context.Background(), "MONGO TLS SECRET NAME")
	// TLS_CONFIG
	tlsConfig, err := dbGetCustomTLSConfig(tlsCerts)
	if err != nil {
		return nil, err
	}

	mongoDB, mongoClient, err := connectMongo(pMongoURLstr,
		pMongoDBNamestr,
		tlsConfig)
	if err != nil {
		return nil, err
	}
	log.Info("mongo connected! ✅")

	db := &DB{
		MongoClient: mongoClient,
		MongoDB:     mongoDB,
	}

	return db, nil
}

//-------------------------------------------------------------
func dbGetCustomTLSConfig(pCerts []byte) (*tls.Config, error) {

	tlsConfig := new(tls.Config)
	tlsConfig.RootCAs = x509.NewCertPool()

	ok := tlsConfig.RootCAs.AppendCertsFromPEM(pCerts)
	if !ok {
		return nil, fmt.Errorf("unable to append certs from pem")
	}

	return tlsConfig, nil
}

func setupMongoIndexes(db *mongo.Database) error {
	b := true
	db.Collection("users").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: bson.M{"username_idempotent": 1},
		Options: &options.IndexOptions{
			Unique: &b,
		},
	})
	return nil
}

func connectMongo(pMongoURL string,
	pDbName string,
	pTLS *tls.Config,
) (*mongo.Database, *mongo.Client, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()

	mOpts := options.Client().ApplyURI(pMongoURL)

	// TLS
	if pTLS != nil {
		mOpts.SetTLSConfig(pTLS)
	}

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		return nil, nil, err
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, nil, err
	}

	db := mClient.Database(pDbName)

	return db, mClient, nil
}
