package glry_lib

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
	// gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//---------------------------------------------------
func TestAuthJWT(pTest *testing.T) {

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	log.Info(fmt.Sprint(cyan("TEST__AUTH_JWT"), yellow(" ==============================================")))
	ctx := context.Background()

	testAddressStr := glry_db.GLRYuserAddress("0xBA47Bef4ca9e8F86149D2f109478c6bd8A642C97")

	//--------------------
	// RUNTIME_SYS

	mongoURLstr := "mongodb://127.0.0.1:27017"
	mongoDBnameStr := "glry_test"
	config := &glry_core.GLRYconfig{
		// Env            string
		// BaseURL        string
		// WebBaseURL     string
		// Port              int
		MongoURLstr:       mongoURLstr,
		MongoDBnameStr:    mongoDBnameStr,
		JWTtokenTTLsecInt: 86400,
	}
	runtime, gErr := glry_core.RuntimeGet(config)
	if gErr != nil {
		pTest.Fail()
	}

	//--------------------
	// JWT_SIMPLE

	testSigningKeyStr := "test_jwt_signing_key"
	testIssuerStr := "test_issuer"

	// GENERATE
	JWTtokenStr, gErr := AuthJWTgenerate(testSigningKeyStr,
		testIssuerStr,
		testAddressStr,
		runtime)
	if gErr != nil {
		pTest.Fail()
	}

	// VERIFY
	validBool, gErr := AuthJWTverify(JWTtokenStr, testSigningKeyStr, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"valid": validBool}).Info("JWT validity")

	assert.True(pTest, validBool, "test JWT is not valid")

	//--------------------
	// JWT_PIPELINES

	newJWTtokenStr, gErr := AuthJWTgeneratePipeline(testAddressStr, ctx, runtime)
	if gErr != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"jwt_token": newJWTtokenStr}).Info("pipeline - JTW generated token")

	newValidBool, gErr := AuthJWTverifyPipeline(newJWTtokenStr,
		testAddressStr,
		ctx,
		runtime)
	if gErr != nil {
		pTest.Fail()
	}

	log.WithFields(log.Fields{"valid": newValidBool}).Info("pipeline - JWT validity")

	assert.True(pTest, newValidBool, "test JWT is not valid")

	//--------------------
}
