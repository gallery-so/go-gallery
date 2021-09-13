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

	mongoURLSecretName = "MONGO_URL_SECRET_NAME"
	mongoTLSSecretName = "MONGO_TLS_SECRET_NAME"
	mongoUseTLS        = "GLRY_MONGO_USE_TLS"

	redisURL            = "GLRY_REDIS_URL"
	redisPassSecretName = "REDIS_PASS_SECRET_NAME"

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

	MongoURL    string
	MongoUseTLS bool

	RedisURL      string
	RedisPassword string

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

	viper.SetDefault(mongoUseTLS, false)
	viper.SetDefault(mongoURLSecretName, "")
	viper.SetDefault(mongoTLSSecretName, "")

	viper.SetDefault(redisURL, "localhost:6379")
	viper.SetDefault(redisPassSecretName, "")

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

		MongoUseTLS: viper.GetBool(mongoUseTLS),

		RedisURL: viper.GetString(redisURL),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),
	}

	if config.EnvStr == "local" {
		config.MongoURL = "mongodb://localhost:27017/"
		config.RedisPassword = ""
	} else {
		mgoURL, err := accessSecret(context.Background(), viper.GetString(mongoURLSecretName))
		if err != nil {
			log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
			panic(-1)
		}
		// TODO no redis password at the moment
		// redisPassword, err := accessSecret(context.Background(), viper.GetString(redisPassSecretName))
		// if err != nil {
		// 	log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
		// 	panic(-1)
		// }
		// config.RedisPassword = string(redisPassword)
		config.MongoURL = string(mgoURL)
	}

	return config
}
