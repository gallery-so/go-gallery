package glry_core

import (
	// "fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

//-------------------------------------------------------------
const (
	env         = "ENV"
	baseURL     = "BASE_URL"
	webBaseURL  = "WEB_BASE_URL"
	port        = "PORT"
	mongoHost   = "MONGO_URI"
	mongoDBname = "MONGO_DB_NAME"
	jwtTokenTTLsecInt = "JWT_TOKEN_TTL_SECS"
)

type GLRYconfig struct {
	Env            string
	BaseURL        string // USED?
	WebBaseURL     string // USED?
	Port           int
	MongoHostStr   string
	MongoDBnameStr string
	JWTtokenTTLsecInt int64
}

//-------------------------------------------------------------
func LoadConfig() *GLRYconfig {

	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(webBaseURL, "http://localhost:3000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(mongoHost, "")
	viper.SetDefault(mongoDBname, "")
	viper.SetDefault(jwtTokenTTLsecInt, 60*60*24)

	viper.SetConfigFile("./.env")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"err": err,}).Fatal("Error reading in env file")
		panic(-1)
	}

	config := &GLRYconfig{
		Env:            viper.GetString(env),
		BaseURL:        viper.GetString(baseURL),
		WebBaseURL:     viper.GetString(webBaseURL),
		Port:           viper.GetInt(port),
		MongoHostStr:   viper.GetString(mongoHost),
		MongoDBnameStr: viper.GetString(mongoDBname),
		// PostgresURI: viper.GetString(postgresURI),
	}
	return config
}
