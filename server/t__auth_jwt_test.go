package server

import (
	"context"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TODO change tests to reflect changes to jwt
//---------------------------------------------------
func TestAuthJWT(pTest *testing.T) {

	ctx := context.Background()

	testUserId := persist.DbId("testid")

	//--------------------
	// RUNTIME_SYS

	mongoURLstr := "mongodb://127.0.0.1:27017"
	mongoDBnameStr := "glry_test"
	config := &runtime.Config{
		// Env            string
		// BaseURL        string
		// WebBaseURL     string
		// Port              int
		MongoURLstr:       mongoURLstr,
		MongoDBnameStr:    mongoDBnameStr,
		JWTtokenTTLsecInt: 86400,
	}
	runtime, gErr := runtime.RuntimeGet(config)
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------
	// JWT_SIMPLE

	testSigningKeyStr := "test_jwt_signing_key"
	testIssuerStr := "test_issuer"

	// GENERATE
	JWTtokenStr, err := jwtGenerate(testSigningKeyStr,
		testIssuerStr,
		testUserId,
		runtime)
	if err != nil {
		pTest.Fail()
	}

	// VERIFY
	validBool, _, err := authJwtParse(JWTtokenStr, testSigningKeyStr, runtime)
	if err != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"valid": validBool}).Info("JWT validity")

	assert.True(pTest, validBool, "test JWT is not valid")

	//--------------------
	// JWT_PIPELINES

	newJWTtokenStr, err := jwtGeneratePipeline(testUserId, ctx, runtime)
	if err != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"jwt_token": newJWTtokenStr}).Info("pipeline - JTW generated token")

	newValidBool, _, err := authJwtParse(newJWTtokenStr,
		testSigningKeyStr,
		runtime)
	if err != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"valid": newValidBool}).Info("pipeline - JWT validity")

	assert.True(pTest, newValidBool, "test JWT is not valid")

}
