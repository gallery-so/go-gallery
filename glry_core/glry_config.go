package glry_core

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/gloflow/gloflow/go/gf_aws"
	"github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
const (
	env          = "GLRY_ENV"
	baseURL      = "GLRY_BASE_URL"
	port         = "GLRY_PORT"
	portMetrics  = "GLRY_PORT_METRIM"
 	mongoHost    = "GLRY_MONGO_HOST"
	mongoDBname  = "GLRY_MONGO_DB_NAME"
	mongoUser    = "GLRY_MONGO_USER"
	mongoPass    = "GLRY_MONGO_PASS"
	sentryEndpoint    = "GLRY_SENTRY_ENDPOINT"
	jwtTokenTTLsecInt = "GLRY_JWT_TOKEN_TTL_SECS"
	awsSecrets        = "GLRY_AWS_SECRETS"
)

type GLRYconfig struct {
	EnvStr         string
	BaseURL        string
	Port           int
	PortMetrics    int
	MongoHostStr   string
	MongoDBnameStr string
	MongoUserStr   string
	MongoPassStr   string
	SentryEndpointStr string
	JWTtokenTTLsecInt int64

	AWSsecretsBool bool
}

//-------------------------------------------------------------
func ConfigLoad() *GLRYconfig {

	viper.SetDefault(env, "local")
	viper.SetDefault(baseURL, "http://localhost:4000")
	viper.SetDefault(port, 4000)
	viper.SetDefault(portMetrics, 4000)
	viper.SetDefault(mongoHost, "localhost")
	viper.SetDefault(mongoDBname, "glry")
	viper.SetDefault(mongoUser, "") // empty strings by default to turn off authenticated client
	viper.SetDefault(mongoPass, "")
	viper.SetDefault(sentryEndpoint, "")
	viper.SetDefault(jwtTokenTTLsecInt, 60*60*24*3)
	viper.SetDefault(awsSecrets, false)

	viper.SetConfigFile("./.env")

	if err := viper.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{"err": err,}).Fatal("Error reading in env file")
		panic(-1)
	}

	config := &GLRYconfig{
		EnvStr:         viper.GetString(env),
		BaseURL:        viper.GetString(baseURL),
		Port:           viper.GetInt(port),
		PortMetrics:    viper.GetInt(portMetrics),
		MongoHostStr:   viper.GetString(mongoHost),
		MongoDBnameStr: viper.GetString(mongoDBname),
		MongoUserStr:   viper.GetString(mongoUser),
		MongoPassStr:   viper.GetString(mongoPass),

		SentryEndpointStr: viper.GetString(sentryEndpoint),
		JWTtokenTTLsecInt: int64(viper.GetInt(jwtTokenTTLsecInt)),

		AWSsecretsBool: viper.GetBool(awsSecrets),
	}
	return config
}

//-------------------------------------------------------------
// GET_AWS_SECRETS
func ConfigGetAWSsecrets(pEnvStr string,
	pRuntimeSys *gf_core.Runtime_sys) (map[string]string, *gf_core.Gf_error) {

	secretsLst := []string{
		"glry_mongo_user",
		"glry_mongo_pass",
	}

	secretValuesMap := map[string]string{}
	for _, secretNameStr := range secretsLst {

		secretFullNameStr := fmt.Sprintf("%s__%s", secretNameStr, pEnvStr)

		secretValueStr, gErr := gf_aws.AWS_SECMNGR__get_secret(secretFullNameStr, pRuntimeSys)
		if gErr != nil {
			return nil, gErr
		}

		secretValuesMap[secretNameStr] = secretValueStr
	}

	return secretValuesMap, nil
}