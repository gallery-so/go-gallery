package server

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/memstore"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/persist/mongodb"
	"github.com/mikeydub/go-gallery/util"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type repositories struct {
	userRepository       persist.UserRepository
	nonceRepository      persist.NonceRepository
	loginRepository      persist.LoginAttemptRepository
	tokenRepository      persist.TokenRepository
	collectionRepository persist.CollectionRepository
	galleryRepository    persist.GalleryRepository
	historyRepository    persist.OwnershipHistoryRepository
	accountRepository    persist.AccountRepository
	contractRepository   persist.ContractRepository
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit() *gin.Engine {
	log.Info("initializing server...")

	setDefaults()

	router := gin.Default()
	router.Use(handleCORS())

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		log.Info("registering validation")
		v.RegisterValidation("short_string", shortStringValidator)
		v.RegisterValidation("medium_string", mediumStringValidator)
		v.RegisterValidation("eth_addr", ethValidator)
		v.RegisterValidation("nonce", nonceValidator)
		v.RegisterValidation("signature", signatureValidator)
		v.RegisterValidation("username", usernameValidator)
	}

	ethClient := newEthClient()
	return handlersInit(router, ethClient)
}

// Init initializes the server
func Init() {
	router := CoreInit()
	if err := router.Run(fmt.Sprintf(":%s", viper.GetString("PORT"))); err != nil {
		panic(err)
	}
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_SECRET", "Test-Secret")
	viper.SetDefault("JWT_TTL", 60*60*24*3)
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("CONTRACT_ADDRESS", "0x970b6AFD5EcDCB4001dB8dBf5E2702e86c857E54")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-kovan.alchemyapi.io/v2/lZc9uHY6g2ak1jnEkrOkkopylNJXvE76")

	viper.AutomaticEnv()
}

func newRepos() *repositories {

	mgoClient := newMongoClient()
	redisClients := newMemstoreClients()
	return &repositories{
		nonceRepository:      mongodb.NewNonceMongoRepository(mgoClient),
		loginRepository:      mongodb.NewLoginMongoRepository(mgoClient),
		collectionRepository: mongodb.NewCollectionMongoRepository(mgoClient, redisClients),
		galleryRepository:    mongodb.NewGalleryMongoRepository(mgoClient),
		historyRepository:    mongodb.NewHistoryMongoRepository(mgoClient),
		tokenRepository:      mongodb.NewTokenMongoRepository(mgoClient),
		userRepository:       mongodb.NewUserMongoRepository(mgoClient),
		accountRepository:    mongodb.NewAccountMongoRepository(mgoClient),
		contractRepository:   mongodb.NewContractMongoRepository(mgoClient),
	}
}

func newMongoClient() *mongo.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	mgoURL := "mongodb://localhost:27017/"
	if viper.GetString("ENV") != "local" {
		mongoSecretName := viper.GetString("MONGO_SECRET_NAME")
		secret, err := util.AccessSecret(context.Background(), mongoSecretName)
		if err != nil {
			panic(err)
		}
		mgoURL = string(secret)
	}

	mOpts := options.Client().ApplyURI(string(mgoURL))

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

func newEthClient() *eth.Client {
	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	return eth.NewEthClient(client, viper.GetString("CONTRACT_ADDRESS"))
}

func newMemstoreClients() *memstore.Clients {
	redisURL := viper.GetString("REDIS_URL")
	redisPass := viper.GetString("REDIS_PASS")
	opensea := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       int(memstore.OpenseaRDB),
	})
	if err := opensea.Ping(context.Background()).Err(); err != nil {
		panic(err)
	}
	unassigned := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPass,
		DB:       int(memstore.CollUnassignedRDB),
	})
	if err := unassigned.Ping(context.Background()).Err(); err != nil {
		panic(err)
	}
	return memstore.NewMemstoreClients(opensea, unassigned)
}
