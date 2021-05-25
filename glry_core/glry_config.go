package glry_core

import (
	// "fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

//-------------------------------------------------------------
const (
	env          = "GLRY_ENV"
	baseURL      = "GLRY_BASE_URL"
	port         = "GLRY_PORT"
	portMetrics  = "GLRY_PORT_METRIM"
 	mongoHost    = "GLRY_MONGO_HOST"
	mongoDBname  = "GLRY_MONGO_DB_NAME"
	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
)

type GLRYconfig struct {
	Env            string
	BaseURL        string
	Port           int
	PortMetrics    int
	MongoHostStr   string
	MongoDBnameStr string
	SentryEndpointStr string
	JWTtokenTTLsecInt int64
}

//-------------------------------------------------------------
func LoadConfig() *GLRYconfig {

	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(portMetrics, 4000)
	viper.SetDefault(mongoHost, "")
	viper.SetDefault(mongoDBname, "")
	viper.SetDefault(sentryEndpoint, "")
	viper.SetDefault(jwtTokenTTLsecInt, 60*60*24*3)

	viper.SetConfigFile("./.env")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"err": err,}).Fatal("Error reading in env file")
		panic(-1)
	}

	config := &GLRYconfig{
		Env:            viper.GetString(env),
		BaseURL:        viper.GetString(baseURL),
		Port:           viper.GetInt(port),
		PortMetrics:    viper.GetInt(portMetrics),
		MongoHostStr:   viper.GetString(mongoHost),
		MongoDBnameStr: viper.GetString(mongoDBname),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: viper.GetInt(),
	}
	return config
}
