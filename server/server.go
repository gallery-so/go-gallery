package server

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/mongodb"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"google.golang.org/api/option"
)

type repositories struct {
	userRepository            persist.UserRepository
	nonceRepository           persist.NonceRepository
	loginRepository           persist.LoginAttemptRepository
	nftRepository             persist.NFTRepository
	tokenRepository           persist.TokenRepository
	collectionRepository      persist.CollectionRepository
	collectionTokenRepository persist.CollectionTokenRepository
	galleryRepository         persist.GalleryRepository
	galleryTokenRepository    persist.GalleryTokenRepository
	historyRepository         persist.OwnershipHistoryRepository
	contractRepository        persist.ContractRepository
	backupRepository          persist.BackupRepository
	membershipRepository      persist.MembershipRepository
}

// Init initializes the server
func init() {
	setDefaults()

	router := CoreInit(newMongoClient(), postgres.NewClient())

	http.Handle("/", router)
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(mgoClient *mongo.Client, pqClient *sql.DB) *gin.Engine {
	log.Info("initializing server...")

	log.SetReportCaller(true)

	router := gin.Default()
	router.Use(middleware.HandleCORS(), middleware.ErrLogger())

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		log.Info("registering validation")
		v.RegisterValidation("short_string", validate.ShortStringValidator)
		v.RegisterValidation("medium_string", validate.MediumStringValidator)
		v.RegisterValidation("eth_addr", validate.EthValidator)
		v.RegisterValidation("nonce", validate.NonceValidator)
		v.RegisterValidation("signature", validate.SignatureValidator)
		v.RegisterValidation("username", validate.UsernameValidator)

	}

	if err := redis.ClearCache(); err != nil {
		panic(err)
	}

	return handlersInit(router, newRepos(mgoClient, pqClient), newEthClient(), newIPFSShell(), newGCPPubSub(), newGCPStorageClient())
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_SECRET", "Test-Secret")
	viper.SetDefault("JWT_TTL", 60*60*24*7)
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("MONGO_URL", "mongodb://localhost:27017/")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("GOOGLE_APPLICATION_CREDENTIALS", "decrypted/service-key.json")
	viper.SetDefault("CONTRACT_ADDRESS", "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("REQUIRE_NFTS", false)
	viper.SetDefault("ADMIN_PASS", "TEST_ADMIN_PASS")
	viper.SetDefault("MIXPANEL_TOKEN", "")
	viper.SetDefault("MIXPANEL_API_URL", "https://api.mixpanel.com/track")
	viper.SetDefault("SIGNUPS_TOPIC", "user-signup")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("GCLOUD_SERVICE_KEY", "")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" && viper.GetString("ADMIN_PASS") == "TEST_ADMIN_PASS" {
		panic("ADMIN_PASS must be set")
	}
}

func newRepos(mgoClient *mongo.Client, db *sql.DB) *repositories {

	openseaCache, galleriesCache := redis.NewCache(0), redis.NewCache(1)
	galleriesCacheToken := redis.NewCache(2)
	nftsCache := redis.NewCache(3)
	return &repositories{
		userRepository:            postgres.NewUserRepository(db),
		nonceRepository:           postgres.NewNonceRepository(db),
		loginRepository:           postgres.NewLoginRepository(db),
		nftRepository:             postgres.NewNFTRepository(db, openseaCache, nftsCache),
		tokenRepository:           postgres.NewTokenRepository(db),
		collectionRepository:      postgres.NewCollectionRepository(db),
		collectionTokenRepository: postgres.NewCollectionTokenRepository(db),
		galleryRepository:         postgres.NewGalleryRepository(db, galleriesCache),
		galleryTokenRepository:    postgres.NewGalleryTokenRepository(db, galleriesCacheToken),
		historyRepository:         mongodb.NewHistoryRepository(mgoClient),
		contractRepository:        postgres.NewContractRepository(db),
		backupRepository:          postgres.NewBackupRepository(db),
		membershipRepository:      postgres.NewMembershipRepository(db),
	}
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
	logrus.Infof("Connecting to mongo at %s\n", mgoURL)

	mOpts := options.Client().ApplyURI(string(mgoURL))
	mOpts.SetRegistry(mongodb.CustomRegistry)
	mOpts.SetRetryWrites(true)
	mOpts.SetWriteConcern(writeconcern.New(writeconcern.WMajority()))

	return mongodb.NewMongoClient(ctx, mOpts)
}

func newEthClient() *eth.Client {
	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	return eth.NewEthClient(client, viper.GetString("CONTRACT_ADDRESS"))
}

func newIPFSShell() *shell.Shell {
	sh := shell.NewShell(viper.GetString("IPFS_URL"))
	sh.SetTimeout(time.Second * 15)
	return sh
}

func newGCPPubSub() pubsub.PubSub {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	defer cancel()
	client, err := gcp.NewPubSub(ctx)
	if err != nil {
		panic(err)
	}
	client.CreateTopic(ctx, viper.GetString("SIGNUPS_TOPIC"))
	return client
}

func newGCPStorageClient() *storage.Client {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	defer cancel()
	if viper.GetString("ENV") != "production" || viper.GetString("ENV") != "development" {

		appCredentials := viper.GetString("GOOGLE_APPLICATION_CREDENTIALS")
		_, err := os.Stat(appCredentials)
		if err != nil {
			_, err = os.Stat(filepath.Join("..", appCredentials))
			if err == nil {
				appCredentials = filepath.Join("..", appCredentials)
				viper.Set("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join("..", appCredentials))
			}
		}
		if err != nil {
			client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(viper.GetString("GCLOUD_SERVICE_KEY"))))
			if err != nil {
				panic(err)
			}
			return client
		}
		client, err := storage.NewClient(ctx, option.WithCredentialsFile(appCredentials))
		if err != nil {
			panic(err)
		}
		return client
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		panic(err)
	}
	return client

}
