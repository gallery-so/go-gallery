package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/gin-gonic/gin"
)

// RedisDB represents the database number to use for the redis client
type RedisDB int

const (
	// CollUnassignedRDB is a throttled cache for expensive queries finding unassigned NFTs
	CollUnassignedRDB RedisDB = iota
	// OpenseaRDB is a throttled cache for expensive queries finding Opensea NFTs
	OpenseaRDB
)

// Runtime represents the runtime of the application and its services
type Runtime struct {
	Config          *Config
	DB              *DB
	Redis           *Redis
	Router          *gin.Engine
	ContractsClient *ethclient.Client
}

// DB is an abstract represenation of a MongoDB database and Client to interact with it
type DB struct {
	MongoClient *mongo.Client
	MongoDB     *mongo.Database
}

// Redis represents the redis clients throughout the application
type Redis struct {
	OpenseaClient    *redis.Client
	UnassignedClient *redis.Client
}

// GetRuntime sets up the runtime to be used at the start of the application
func GetRuntime(pConfig *Config) (*Runtime, error) {

	//------------------
	// LOGS
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
	log.SetLevel(log.DebugLevel)

	//------------------
	// DB

	mongoURLstr := pConfig.MongoURL

	mongoDBnameStr := pConfig.MongoDBName

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
	runtime.setupContractsClient()
	runtime.setupRedis()
	log.Info("redis connected! ✅")

	return runtime, nil
}

func dbInit(pMongoURLstr string,
	pMongoDBNamestr string,
	pConfig *Config) (*DB, error) {

	log.WithFields(log.Fields{}).Info("connecting to mongo...")

	var tlsConf *tls.Config
	if pConfig.MongoUseTLS {
		tlsCerts, err := accessSecret(context.Background(), viper.GetString(mongoTLSSecretName))
		if err != nil {
			return nil, err
		}
		tlsConf, err = dbGetCustomTLSConfig(tlsCerts)
		if err != nil {
			return nil, err
		}
	}
	mongoDB, mongoClient, err := connectMongo(pMongoURLstr,
		pMongoDBNamestr,
		tlsConf)
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

func (r *Runtime) setupRedis() {
	opensea := redis.NewClient(&redis.Options{
		Addr:     r.Config.RedisURL,
		Password: r.Config.RedisPassword,
		DB:       int(OpenseaRDB),
	})
	if err := opensea.Ping().Err(); err != nil {
		panic(err)
	}
	unassigned := redis.NewClient(&redis.Options{
		Addr:     r.Config.RedisURL,
		Password: r.Config.RedisPassword,
		DB:       int(CollUnassignedRDB),
	})
	if err := unassigned.Ping().Err(); err != nil {
		panic(err)
	}
	r.Redis = &Redis{
		OpenseaClient:    opensea,
		UnassignedClient: unassigned,
	}
}
func (r *Runtime) setupContractsClient() {
	client, err := ethclient.Dial(r.Config.ContractInteractionURL)
	if err != nil {
		panic(err)
	}
	r.ContractsClient = client
}

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
			Sparse: &b,
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
