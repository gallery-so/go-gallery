package server

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/mongodb"
	"github.com/mikeydub/go-gallery/persist"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var repos *repositories

var env string

type repositories struct {
	userRepository       persist.UserRepository
	nonceRepository      persist.NonceRepository
	loginRepository      persist.LoginAttemptRepository
	nftRepository        persist.NFTRepository
	collectionRepository persist.CollectionRepository
	galleryRepository    persist.GalleryRepository
	historyRepository    persist.OwnershipHistoryRepository
}

func init() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("JWT_SECRET", "Test-Secret")
	viper.SetDefault("JWT_TTL", 60*60*24*3)

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()

	env = viper.GetString("ENV")
	allowedOrigins = viper.GetString("ALLOWED_ORIGINS")
	jwtSecret = viper.GetString("JWT_SECRET")
	jwtTTL = viper.GetInt64("JWT_TTL")

	repos = newRepos()
}

// CoreInit initializes core server functionality. This is abstracted
// so the test server can also utilize it
func CoreInit() *gin.Engine {
	log.Info("initializing server...")

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

	return handlersInit(router)
}

// Init initializes the server
func Init(port string,
) {

	router := CoreInit()

	if err := router.Run(fmt.Sprintf(":%s", port)); err != nil {
		panic(err)
	}
}

func newRepos() *repositories {
	return &repositories{
		nonceRepository:      mongodb.NewNonceMongoRepository(),
		loginRepository:      mongodb.NewLoginMongoRepository(),
		collectionRepository: mongodb.NewCollectionMongoRepository(),
		galleryRepository:    mongodb.NewGalleryMongoRepository(),
		historyRepository:    mongodb.NewHistoryMongoRepository(),
		nftRepository:        mongodb.NewNFTMongoRepository(),
		userRepository:       mongodb.NewUserMongoRepository(),
	}
}
