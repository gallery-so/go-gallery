package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-redis/redis"
	ipfs "github.com/ipfs/go-ipfs-api"
	"github.com/sirupsen/logrus"
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
	IPFS         *ipfs.Shell
	Cancel       chan os.Signal
}

// DB is an abstract represenation of a MongoDB database and Client to interact with it
type DB struct {
	MongoClient *mongo.Client
	GalleryDB   *mongo.Database
	InfraDB     *mongo.Database
}

// InfraClients is a wrapper for the alchemy clients necessary for json RPC and contract interaction
type InfraClients struct {
	RPCClient *rpc.Client
	ETHClient *ethclient.Client
}

// GetRuntime sets up the runtime to be used at the start of the application
func GetRuntime(pConfig *Config) (*Runtime, error) {

	//------------------
	// LOGS
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
	log.SetLevel(log.DebugLevel)

	// RUNTIME
	runtime := &Runtime{
		Config: pConfig,
		IPFS:   newIPFSShell(pConfig.IPFSURL),
		Cancel: make(chan os.Signal),
	}
	runtime.newInfraClients()
	//------------------
	// DB

	mongoURLstr := pConfig.MongoURL

	err := runtime.dbInit(mongoURLstr, pConfig)

	if err != nil {
		return nil, err
	}

	err = runtime.setupMongoIndexes()
	if err != nil {
		return nil, err
	}

	log.Info("RPC, ETH, and IPFS clients connected! ✅")

	// notify cancel channel when SIGINT or SIGTERM is received
	runtime.notifyOnCancel()

	// TEST REDIS CONNECTION
	client := redis.NewClient(&redis.Options{
		Addr:     pConfig.RedisURL,
		Password: pConfig.RedisPassword,
		DB:       0,
	})

	if err = client.Ping().Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %s\n connecting with URL %s", err, pConfig.RedisURL)
	}
	log.Info("redis working! ✅")

	return runtime, nil
}

func (r *Runtime) dbInit(pMongoURLstr string,
	pConfig *Config) error {
	r.DB = &DB{}

	log.WithFields(log.Fields{}).Info("connecting to mongo...")

	var tlsConf *tls.Config
	if pConfig.MongoUseTLS {
		tlsCerts, err := accessSecret(context.Background(), viper.GetString(mongoTLSSecretName))
		if err != nil {
			return err
		}
		tlsConf, err = dbGetCustomTLSConfig(tlsCerts)
		if err != nil {
			return err
		}
	}
	err := r.connectMongo(pMongoURLstr, tlsConf)
	if err != nil {
		return err
	}
	log.Info("mongo connected! ✅")
	r.DB.GalleryDB = r.DB.MongoClient.Database(GalleryDBName)
	r.DB.InfraDB = r.DB.MongoClient.Database(InfraDBName)

	return nil
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

func (r *Runtime) setupMongoIndexes() error {
	b := true
	r.DB.GalleryDB.Collection("users").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: bson.M{"username_idempotent": 1},
		Options: &options.IndexOptions{
			Unique: &b,
			Sparse: &b,
		},
	})
	// db.Collection("tokens").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
	// 	Keys: bson.M{"contract_address": 1, "token_id": 1},
	// })
	// db.Collection("tokens").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
	// 	Keys: bson.M{"owner_address": 1},
	// })
	// db.Collection("tokens").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
	// 	Keys: bson.M{"last_updated": -1},
	// })
	r.DB.GalleryDB.Collection("accounts").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: bson.M{"address": 1},
		Options: &options.IndexOptions{
			Unique: &b,
			Sparse: &b,
		},
	})
	r.DB.GalleryDB.Collection("contracts").Indexes().CreateOne(context.TODO(), mongo.IndexModel{
		Keys: bson.M{"address": 1},
		Options: &options.IndexOptions{
			Unique: &b,
			Sparse: &b,
		},
	})

	return nil
}

func (r *Runtime) connectMongo(pMongoURL string, pTLS *tls.Config) error {

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()

	mOpts := options.Client().ApplyURI(pMongoURL)

	// TLS
	if pTLS != nil {
		mOpts.SetTLSConfig(pTLS)
	}

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		return err
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		return err
	}

	r.DB.MongoClient = mClient
	return nil
}

func (r *Runtime) newInfraClients() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logrus.Debugf("Connecting to RPC with URL: %s", r.Config.RPCURL)
	client, err := rpc.DialContext(ctx, r.Config.RPCURL)
	if err != nil {
		panic(err)
	}

	ethClient := ethclient.NewClient(client)

	r.InfraClients = &InfraClients{
		RPCClient: client,
		ETHClient: ethClient,
	}
}

func newIPFSShell(url string) *ipfs.Shell {
	sh := ipfs.NewShell(url)
	sh.SetTimeout(time.Second * 2)
	return sh
}

func (r *Runtime) notifyOnCancel() {
	signal.Notify(r.Cancel, syscall.SIGINT, syscall.SIGTERM)
}
