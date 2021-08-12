package runtime

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	env            = "GLRY_ENV"
	baseURL        = "GLRY_BASE_URL"
	port           = "GLRY_PORT"
	portMetrics    = "GLRY_PORT_METRIM"
	allowedOrigins = "GLRY_ALLOWED_ORIGINS"

	mongoURLSecretName    = "projects/1066359838176/secrets/GLRY_MONGO_URL/versions/latest"
	mongoTLSSecretName    = "GLRY_TLS"
	mongoUseTLS           = "GLRY_MONGO_USE_TLS"
	mongoDBname           = "GLRY_MONGO_DB_NAME"

	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
)

// Config represents an application configuration that is determined at runtime start
type Config struct {
	EnvStr         string
	BaseURL        string
	Port           int
	PortMetrics    int
	AllowedOrigins string

	MongoURL              string
	MongoDBName           string
	MongoUseTLS           bool

	SentryEndpointStr string
	JWTtokenTTLsecInt int64
}

// ConfigLoad loads the runtime configuration from the viper config and grabs necessary secrets
// from GCP
func ConfigLoad() *Config {

	//------------------
	// DEFAULTS
	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(portMetrics, 4000)
	viper.SetDefault(allowedOrigins, "http://localhost:3000")

	viper.SetDefault(mongoDBname, "gallery")
	viper.SetDefault(mongoUseTLS, false)

	viper.SetDefault(sentryEndpoint, "")
	viper.SetDefault(jwtTokenTTLsecInt, 60*60*24*3)

	//------------------

	viper.Set("true", true)
	viper.Set("false", false)

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()

	envPath := getEnvPath()
	if envPath != "" {
		viper.SetConfigFile(envPath)
		if err := viper.ReadInConfig(); err != nil {
			log.WithFields(log.Fields{"err": err}).Fatal("Error reading config")
			panic(-1)
		}
	}

	config := &Config{
		EnvStr:         viper.GetString(env),
		BaseURL:        viper.GetString(baseURL),
		Port:           viper.GetInt(port),
		PortMetrics:    viper.GetInt(portMetrics),
		AllowedOrigins: viper.GetString(allowedOrigins),

		MongoUseTLS:           viper.GetBool(mongoUseTLS),
		MongoDBName:           viper.GetString(mongoDBname),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),
	}

	if config.EnvStr == "local" {
		config.MongoURL = "mongodb://localhost:27017/"
	} else {
		mgoURL, err := accessSecret(context.Background(), mongoURLSecretName)
		if err != nil {
			log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
			panic(-1)
		}
		config.MongoURL = string(mgoURL)
	}

	return config
}
