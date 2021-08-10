package runtime

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
const (
	env         = "GLRY_ENV"
	baseURL     = "GLRY_BASE_URL"
	port        = "GLRY_PORT"
	portMetrics = "GLRY_PORT_METRIM"

	mongoDBname           = "GLRY_MONGO_DB_NAME"
	mongoSslCAfilePathStr = "GLRY_MONGO_SSL_CA_FILE_PATH"

	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
)

type Config struct {
	EnvStr      string
	BaseURL     string
	Port        int
	PortMetrics int

	MongoURLstr           string
	MongoDBnameStr        string
	MongoSslCAfilePathStr string

	SentryEndpointStr string
	JWTtokenTTLsecInt int64
}

//-------------------------------------------------------------
func ConfigLoad() *Config {

	//------------------
	// DEFAULTS
	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(portMetrics, 4000)

	viper.SetDefault(mongoDBname, "gallery")
	viper.SetDefault(mongoSslCAfilePathStr, "")

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

	// TODO secret name
	mgoURL, err := accessSecret(context.Background(), "MONGO SECRET NAME HERE")
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Error reading secret")
		panic(-1)
	}

	config := &Config{
		EnvStr:      viper.GetString(env),
		BaseURL:     viper.GetString(baseURL),
		Port:        viper.GetInt(port),
		PortMetrics: viper.GetInt(portMetrics),

		MongoURLstr:           string(mgoURL),
		MongoDBnameStr:        viper.GetString(mongoDBname),
		MongoSslCAfilePathStr: viper.GetString(mongoSslCAfilePathStr),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),
	}

	return config
}

//-------------------------------------------------------------
// GET_AWS_SECRETS
// func ConfigGetAWSsecrets(pEnvStr string,
// 	pRuntimeSys *gf_core.Runtime_sys) (map[string]map[string]interface{}, *gf_core.Gf_error) {

// 	secretsLst := []string{
// 		"glry_mongo_url",
// 	}

// 	secretValuesMap := map[string]map[string]interface{}{}
// 	for _, secretNameStr := range secretsLst {

// 		secretFullNameStr := fmt.Sprintf("%s_%s", secretNameStr, pEnvStr)

// 		secretMap, gErr := gf_aws.AWS_SECMNGR__get_secret(secretFullNameStr, pRuntimeSys)
// 		if gErr != nil {
// 			return nil, gErr
// 		}

// 		secretValuesMap[secretNameStr] = secretMap
// 	}

// 	return secretValuesMap, nil
// }
