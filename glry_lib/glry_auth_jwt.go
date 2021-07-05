package glry_lib

import (
	"context"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	log "github.com/sirupsen/logrus"
)

// USER_JWT_KEY - is unique per user, and stored in the DB for now.
// type GLRYuserJWTkey struct {
// 	VersionInt    int64            `bson:"version"       mapstructure:"version"`
// 	ID            GLRYuserJWTkeyID `bson:"_id"           mapstructure:"_id"`
// 	CreationTimeF float64          `bson:"creation_time" mapstructure:"creation_time"`
// 	BlackListed   bool             `bson:"blacklisted"       mapstructure:"blacklisted"`

// 	ValueStr   string          `bson:"value"   mapstructure:"value"`
// 	AddressStr GLRYuserAddress `bson:"address" mapstructure:"address"`
// }

//-------------------------------------------------------------
// JWT_CLAIMS
type GLRYjwtClaims struct {
	AddressStr glry_db.GLRYuserAddress `json:"address"`
	jwt.StandardClaims
}

//-------------------------------------------------------------
// VERIFY
func AuthJWTverify(pJWTtokenStr string,
	pJWTsecretKeyStr string,
	pRuntime *glry_core.Runtime) (bool, string, *gf_core.Gf_error) {

	claims := GLRYjwtClaims{}
	JWTtoken, err := jwt.ParseWithClaims(pJWTtokenStr,
		&claims,
		func(pJWTtoken *jwt.Token) (interface{}, error) {
			return []byte(pJWTsecretKeyStr), nil
		})

	if err != nil {
		gErr := gf_core.Error__create("failed to verify JWT token for a user",
			"crypto_jwt_verify_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return false, "", gErr
	}

	log.WithFields(log.Fields{}).Debug("JWT CLAIMS --------------")
	spew.Dump(claims)

	if claims, ok := JWTtoken.Claims.(GLRYjwtClaims); ok && JWTtoken.Valid {
		return JWTtoken.Valid, string(claims.AddressStr), nil
	} else {
		gErr := gf_core.Error__create("failed to verify JWT token for a user",
			"crypto_jwt_verify_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return false, "", gErr
	}
}

//-------------------------------------------------------------
// GENERATE__PIPELINE

// ADD!! - mark all other JWT's for this address as deleted to exclude them from future use.

func AuthJWTgeneratePipeline(pAddressStr glry_db.GLRYuserAddress,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (string, *gf_core.Gf_error) {

	// previously we would generate a random string and use that as jwt secret and store
	// the string in the db with the jwt for verifcation. with stateless auth, we might
	// use an environment variable like so as the secret. worth considering other options
	// to increase security
	JWTissuerStr := "gallery" // string(pAddressStr)
	JWTtokenStr, gErr := AuthJWTgenerate(os.Getenv("JWT_SECRET"),
		JWTissuerStr,
		pAddressStr,
		pRuntime)
	if gErr != nil {
		return "", gErr
	}

	//------------------

	return JWTtokenStr, nil
}

//-------------------------------------------------------------
// GENERATE
// ADD!! - make sure when creating new JWT tokens for user that the old ones are marked as deleted

func AuthJWTgenerate(pSigningKeyStr string,
	pIssuerStr string,
	pAddressStr glry_db.GLRYuserAddress,
	pRuntime *glry_core.Runtime) (string, *gf_core.Gf_error) {

	signingKeyBytesLst := []byte(pSigningKeyStr)

	//------------------
	// CLAIMS

	// Create the Claims
	creationTimeUNIXint := time.Now().UnixNano() / 1000000000
	expiresAtUNIXint := creationTimeUNIXint + pRuntime.Config.JWTtokenTTLsecInt //60*60*24*2 // expire N number of secs from now
	JWTclaims := GLRYjwtClaims{
		pAddressStr,
		jwt.StandardClaims{
			ExpiresAt: expiresAtUNIXint,
			Issuer:    pIssuerStr,
		},
	}

	//------------------

	// SYMETRIC_SIGNING - same secret is used to both sign and validate tokens
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTclaims)

	// SIGN
	JWTtokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {

		gErr := gf_core.Error__create("failed to sign an Auth JWT token for a user",
			"crypto_jwt_sign_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return "", gErr
	}

	return JWTtokenStr, nil
}
