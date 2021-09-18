package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/mikeydub/go-gallery/contracts"

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

const (
	// GalleryDBName represents the database name for gallery related information
	GalleryDBName = "gallery"
	// InfraDBName represents the database name for eth infrastructure related information
	InfraDBName = "infra"
)

// Runtime represents the runtime of the application and its services
type Runtime struct {
	Config       *Config
	DB           *DB
	Router       *gin.Engine
	InfraClients *InfraClients
}

// DB is an abstract represenation of a MongoDB database and Client to interact with it
type DB struct {
	MongoClient *mongo.Client
	GalleryDB   *mongo.Database
	InfraDB     *mongo.Database
}

// InfraClients is a wrapper for the alchemy clients necessary for json RPC and contract interaction
type InfraClients struct {
	RPCClient    *rpc.Client
	ETHClient    *ethclient.Client
	TransferLogs map[string]chan *contracts.IERC721Transfer
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

	db, err := dbInit(mongoURLstr, pConfig)

	if err != nil {
		return nil, err
	}

	err = setupMongoIndexes(db.MongoClient.Database(GalleryDBName))
	if err != nil {
		return nil, err
	}

	// RUNTIME
	runtime := &Runtime{
		Config: pConfig,
		DB:     db,
	}
	runtime.InfraClients = newInfraClients(pConfig.AlchemyURL)

	// TEST REDIS CONNECTION
	client := redis.NewClient(&redis.Options{
		Addr:     pConfig.RedisURL,
		Password: pConfig.RedisPassword,
		DB:       0,
	})
	if err = client.Ping().Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %s\n connecting with URL %s", err, pConfig.RedisURL)
	}
	log.Info("redis connected! ✅")

	return runtime, nil
}

func dbInit(pMongoURLstr string,
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
	mongoClient, err := connectMongo(pMongoURLstr, tlsConf)
	if err != nil {
		return nil, err
	}
	log.Info("mongo connected! ✅")

	db := &DB{
		MongoClient: mongoClient,
		GalleryDB:   mongoClient.Database(GalleryDBName),
		InfraDB:     mongoClient.Database(InfraDBName),
	}

	return db, nil
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
	pTLS *tls.Config,
) (*mongo.Client, error) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()

	mOpts := options.Client().ApplyURI(pMongoURL)

	// TLS
	if pTLS != nil {
		mOpts.SetTLSConfig(pTLS)
	}

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		return nil, err
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, err
	}

	return mClient, nil
}

func newInfraClients(alchemyURL string) *InfraClients {
	client, err := rpc.Dial(alchemyURL)
	if err != nil {
		panic(err)
	}

	ethClient, err := ethclient.Dial(alchemyURL)
	if err != nil {
		panic(err)
	}

	return &InfraClients{
		RPCClient:    client,
		ETHClient:    ethClient,
		TransferLogs: make(map[string]chan *contracts.IERC721Transfer),
	}
}
