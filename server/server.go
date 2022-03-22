package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/memstore/redis"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/service/pubsub/gcp"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

// Init initializes the server
func Init() {
	setDefaults()

	router := CoreInit(postgres.NewClient())

	http.Handle("/", router)
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit(pqClient *sql.DB) *gin.Engine {
	log.Info("initializing server...")

	log.SetReportCaller(true)

	if viper.GetString("ENV") != "production" {
		log.SetLevel(log.DebugLevel)
		gin.SetMode(gin.DebugMode)
	}

	router := gin.Default()
	router.Use(middleware.HandleCORS(), middleware.GinContextToContext(), middleware.ErrLogger())

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		log.Info("registering validation")
		validate.RegisterCustomValidators(v)
	}

	if err := redis.ClearCache(); err != nil {
		panic(err)
	}

	return handlersInit(router, newRepos(pqClient), rpc.NewEthClient(), rpc.NewIPFSShell(), rpc.NewArweaveClient(), newStorageClient(), newGCPPubSub())
}

func newStorageClient() *storage.Client {
	var s *storage.Client
	var err error
	if viper.GetString("ENV") != "local" {
		s, err = storage.NewClient(context.Background())
	} else {
		s, err = storage.NewClient(context.Background(), option.WithCredentialsFile("./_deploy/service-key.json"))
	}
	if err != nil {
		logrus.Errorf("error creating storage client: %v", err)
	}
	return s
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_SECRET", "Test-Secret")
	viper.SetDefault("JWT_TTL", 60*60*24*7)
	viper.SetDefault("PORT", 4000)
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("IPFS_URL", "https://ipfs.io")
	viper.SetDefault("GCLOUD_TOKEN_CONTENT_BUCKET", "token-content")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("GOOGLE_APPLICATION_CREDENTIALS", "_deploy/service-key.json")
	viper.SetDefault("CONTRACT_ADDRESSES", "0x93eC9b03a9C14a530F582aef24a21d7FC88aaC46=[0,1,2,3,4,5,6,7,8]")
	viper.SetDefault("CONTRACT_INTERACTION_URL", "https://eth-rinkeby.alchemyapi.io/v2/_2u--i79yarLYdOT4Bgydqa0dBceVRLD")
	viper.SetDefault("REQUIRE_NFTS", false)
	viper.SetDefault("ADMIN_PASS", "TEST_ADMIN_PASS")
	viper.SetDefault("MIXPANEL_TOKEN", "")
	viper.SetDefault("MIXPANEL_API_URL", "https://api.mixpanel.com/track")
	viper.SetDefault("SIGNUPS_TOPIC", "user-signup")
	viper.SetDefault("ADD_ADDRESS_TOPIC", "user-add-address")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("GCLOUD_SERVICE_KEY", "")
	viper.SetDefault("INDEXER_HOST", "http://localhost:4000")
	viper.SetDefault("SNAPSHOT_BUCKET", "gallery-dev-322005.appspot.com")
	viper.SetDefault("TASK_QUEUE_HOST", "localhost:8123")
	viper.SetDefault("GCLOUD_FEED_TASK_QUEUE", "projects/gallery-local/locations/here/queues/feed-event")
	viper.SetDefault("GCLOUD_FEED_TASK_BUFFER_SECS", 10) // Set low for debugging
	viper.SetDefault("FEEDBOT_SECRET", "feed-bot-secret")

	viper.AutomaticEnv()

	if viper.GetString("ENV") != "local" && viper.GetString("ADMIN_PASS") == "TEST_ADMIN_PASS" {
		panic("ADMIN_PASS must be set")
	}
}

func newRepos(db *sql.DB) *persist.Repositories {
	galleriesCache := redis.NewCache(0)
	galleriesCacheToken := redis.NewCache(1)
	galleryRepo := postgres.NewGalleryRepository(db, galleriesCache)
	galleryTokenRepo := postgres.NewGalleryTokenRepository(db, galleriesCacheToken)

	return &persist.Repositories{
		UserRepository:            postgres.NewUserRepository(db),
		NonceRepository:           postgres.NewNonceRepository(db),
		LoginRepository:           postgres.NewLoginRepository(db),
		NftRepository:             postgres.NewNFTRepository(db, galleryRepo),
		TokenRepository:           postgres.NewTokenRepository(db, galleryTokenRepo),
		CollectionRepository:      postgres.NewCollectionRepository(db, galleryRepo),
		CollectionTokenRepository: postgres.NewCollectionTokenRepository(db, galleryTokenRepo),
		GalleryRepository:         galleryRepo,
		GalleryTokenRepository:    galleryTokenRepo,
		ContractRepository:        postgres.NewContractRepository(db),
		BackupRepository:          postgres.NewBackupRepository(db),
		MembershipRepository:      postgres.NewMembershipRepository(db),
		UserEventRepository:       postgres.NewUserEventRepository(db),
		CollectionEventRepository: postgres.NewCollectionEventRepository(db),
		NftEventRepository:        postgres.NewNftEventRepository(db),
	}
}

func newGCPPubSub() pubsub.PubSub {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10)*time.Second)
	defer cancel()
	client, err := gcp.NewPubSub(ctx)
	if err != nil {
		panic(err)
	}
	client.CreateTopic(ctx, viper.GetString("SIGNUPS_TOPIC"))
	client.CreateTopic(ctx, viper.GetString("ADD_ADDRESS_TOPIC"))
	return client
}
