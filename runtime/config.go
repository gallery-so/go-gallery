package runtime

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	// "github.com/davecgh/go-spew/spew"
)

const (
	env         = "GLRY_ENV"
	baseURL     = "GLRY_BASE_URL"
	port        = "GLRY_PORT"
	portMetrics = "GLRY_PORT_METRIM"

	mongoURLSecretName = "projects/1066359838176/secrets/GLRY_MONGO_URL/versions/latest"
	mongoTLSSecretName = "projects/1066359838176/secrets/GLRY_TLS/versions/latest"
	mongoUseTLS        = "GLRY_MONGO_USE_TLS"
	mongoDBname        = "GLRY_MONGO_DB_NAME"

	redisURL            = "REDIS_URL"
	redisPassSecretName = "projects/1066359838176/secrets/GLRY_REDIS_PASS/versions/latest"

	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
)

// Config represents an application configuration that is determined at runtime start
type Config struct {
	EnvStr      string
	BaseURL     string
	Port        int
	PortMetrics int

	MongoURL    string
	MongoDBName string
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

	viper.SetDefault(mongoDBname, "gallery")
	viper.SetDefault(mongoUseTLS, false)

	viper.SetDefault(redisURL, "localhost:6379")

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
		EnvStr:      viper.GetString(env),
		BaseURL:     viper.GetString(baseURL),
		Port:        viper.GetInt(port),
		PortMetrics: viper.GetInt(portMetrics),

		MongoUseTLS: viper.GetBool(mongoUseTLS),
		MongoDBName: viper.GetString(mongoDBname),

		RedisURL: viper.GetString(redisURL),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),
	}

	if config.EnvStr == "local" {
		config.MongoURL = "mongodb://localhost:27017/"
		config.RedisPassword = ""
	} else {
		mgoURL, err := accessSecret(context.Background(), mongoURLSecretName)
		if err != nil {
			log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
			panic(-1)
		}
		redisPassword, err := accessSecret(context.Background(), redisPassSecretName)
		if err != nil {
			log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
			panic(-1)
		}
		config.RedisPassword = string(redisPassword)
		config.MongoURL = string(mgoURL)
	}

	return config
}
