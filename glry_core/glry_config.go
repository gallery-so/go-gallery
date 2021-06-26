package glry_core

import (
	"fmt"
	"github.com/gloflow/gloflow/go/gf_aws"
	"github.com/gloflow/gloflow/go/gf_core"
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

	mongoURL              = "GLRY_MONGO_URL"
	mongoDBname           = "GLRY_MONGO_DB_NAME"
	mongoSslCAfilePathStr = "GLRY_MONGO_SSL_CA_FILE_PATH"

	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
	awsSecrets        = "GLRY_AWS_SECRETS"
)

type GLRYconfig struct {
	EnvStr      string
	BaseURL     string
	Port        int
	PortMetrics int

	MongoURLstr           string
	MongoDBnameStr        string
	MongoSslCAfilePathStr string

	SentryEndpointStr string
	JWTtokenTTLsecInt int64

	AWSsecretsBool bool
}

//-------------------------------------------------------------
func ConfigLoad() *GLRYconfig {

	//------------------
	// DEFAULTS
	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(portMetrics, 4000)

	viper.SetDefault(mongoURL, "mongodb://localhost:27017")
	viper.SetDefault(mongoDBname, "glry")
	viper.SetDefault(mongoSslCAfilePathStr, "")

	viper.SetDefault(sentryEndpoint, "")
	viper.SetDefault(jwtTokenTTLsecInt, 60*60*24*3)
	viper.SetDefault(awsSecrets, false)

	//------------------

	viper.Set("true", true)
	viper.Set("false", false)

	viper.SetConfigFile("./.env")

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Error reading config")
		panic(-1)
	}

	config := &GLRYconfig{
		EnvStr:      viper.GetString(env),
		BaseURL:     viper.GetString(baseURL),
		Port:        viper.GetInt(port),
		PortMetrics: viper.GetInt(portMetrics),

		MongoURLstr:           viper.GetString(mongoURL),
		MongoDBnameStr:        viper.GetString(mongoDBname),
		MongoSslCAfilePathStr: viper.GetString(mongoSslCAfilePathStr),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),

		AWSsecretsBool: viper.GetBool(awsSecrets),
	}

	fmt.Println("CONFIG----------------------------------")
	// spew.Dump(config)

	return config
}

//-------------------------------------------------------------
// GET_AWS_SECRETS
func ConfigGetAWSsecrets(pEnvStr string,
	pRuntimeSys *gf_core.Runtime_sys) (map[string]map[string]interface{}, *gf_core.Gf_error) {

	secretsLst := []string{
		"glry_mongo_url",
	}

	secretValuesMap := map[string]map[string]interface{}{}
	for _, secretNameStr := range secretsLst {

		secretFullNameStr := fmt.Sprintf("%s_%s", secretNameStr, pEnvStr)

		secretMap, gErr := gf_aws.AWS_SECMNGR__get_secret(secretFullNameStr, pRuntimeSys)
		if gErr != nil {
			return nil, gErr
		}

		secretValuesMap[secretNameStr] = secretMap
	}

	return secretValuesMap, nil
}
