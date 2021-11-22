package features

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/pubsub/pstest"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/pubsub"
	"github.com/mikeydub/go-gallery/pubsub/gcp"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// Init starts the background process for keeping the access state up to date and handles requests
func Init() {
	router := coreInit()
	http.Handle("/", router)
}

func coreInit() *gin.Engine {

	setDefaults()

	userRepo, featuresRepo, accessRepo := newRepos()
	ec := newEthClient()

	go trackFeatures(context.Background(), userRepo, featuresRepo, accessRepo, ec)
	if viper.GetString("ENV") != "local" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
			defer cancel()
			err := listenForSignups(ctx, newGCPPubSub(), userRepo, featuresRepo, accessRepo, ec)
			if err != nil {
				panic(err)
			}
		}()
	}

	router := gin.Default()

	logrus.Info("Registering handlers...")
	return handlersInit(router, userRepo, featuresRepo, accessRepo, ec)
}

func setDefaults() {
	viper.SetDefault("RPC_URL", "wss://eth-mainnet.alchemyapi.io/v2/Lxc2B4z57qtwik_KfOS0I476UUUmXT86")
	viper.SetDefault("MONGO_URL", "mongodb://localhost:27017/")
	viper.SetDefault("ENV", "local")
	viper.SetDefault("SIGNUP_TOPIC", "user-signup")
	viper.AutomaticEnv()
}

func newEthClient() *ethclient.Client {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dialer := *websocket.DefaultDialer
	dialer.ReadBufferSize = 1024 * 20
	rpcClient, err := rpc.DialWebsocketWithDialer(ctx, viper.GetString("RPC_URL"), "", dialer)
	if err != nil {
		panic(err)
	}

	return ethclient.NewClient(rpcClient)

}

func newRepos() (persist.UserRepository, persist.FeatureFlagRepository, persist.AccessRepository) {
	mgoClient := newMongoClient()
	return mongodb.NewUserMongoRepository(mgoClient), mongodb.NewFeaturesMongoRepository(mgoClient), mongodb.NewAccessMongoRepository(mgoClient)
}

func newMongoClient() *mongo.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	mgoURL := viper.GetString("MONGO_URL")
	if viper.GetString("ENV") != "local" {
		mongoSecretName := viper.GetString("MONGO_SECRET_NAME")
		secret, err := util.AccessSecret(context.Background(), mongoSecretName)
		if err != nil {
			panic(err)
		}
		mgoURL = string(secret)
	}

	logrus.Infof("Connecting to mongo at %s", mgoURL)

	mOpts := options.Client().ApplyURI(mgoURL)
	mOpts.SetRegistry(mongodb.CustomRegistry)

	mClient, err := mongo.Connect(ctx, mOpts)
	if err != nil {
		panic(err)
	}

	err = mClient.Ping(ctx, readpref.Primary())
	if err != nil {
		panic(err)
	}

	return mClient
}

func newGCPPubSub() pubsub.PubSub {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	defer cancel()
	if viper.GetString("ENV") != "local" {
		pub, err := gcp.NewGCPPubSub(ctx, viper.GetString("GOOGLE_CLOUD_PROJECT"))
		if err != nil {
			panic(err)
		}
		return pub
	}
	srv := pstest.NewServer()
	// Connect to the server without using TLS.
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	// Use the connection when creating a pubsub client.
	client, err := gcp.NewGCPPubSub(ctx, viper.GetString("GOOGLE_PROJECT_ID"), option.WithGRPCConn(conn))
	if err != nil {
		panic(err)
	}

	client.CreateTopic(ctx, viper.GetString("SIGNUPS_TOPIC"))
	return client
}
