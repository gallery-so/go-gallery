




package glry_lib

import (
	"time"
	"context"
	log "github.com/sirupsen/logrus"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/dgrijalva/jwt-go"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// JWT_CLAIMS
type GLRYjwtClaims struct {
	AddressStr glry_db.GLRYuserAddress `json:"address"`
	jwt.StandardClaims
}

//-------------------------------------------------------------
// VERIFY_PIPELINE
func AuthJWTverifyPipeline(pJWTtokenStr string,
	pUserAddressStr glry_db.GLRYuserAddress,
	pCtx            context.Context,
	pRuntime        *glry_core.Runtime) (bool, *gf_core.Gf_error) {

	//------------------
	// DB_GET_KEY
	JWTkey, gErr := glry_db.AuthUserJWTkeyGet(pUserAddressStr, pCtx, pRuntime)
	if gErr != nil {
		return false, gErr
	}

	//------------------
	// VERIFY
	JWTkeyValueStr := JWTkey.ValueStr
	tokenValidBool, gErr := AuthJWTverify(pJWTtokenStr,
		JWTkeyValueStr,
		pRuntime)
	if gErr != nil {
		return false, gErr
	}
	
	//------------------
	
	claimedAddressStr := JWTkey.AddressStr

	if pUserAddressStr != claimedAddressStr {
		return false, nil
	}
	
	return tokenValidBool, nil
}

//-------------------------------------------------------------
// VERIFY
func AuthJWTverify(pJWTtokenStr string,
	pJWTsecretKeyStr string,
	pRuntime         *glry_core.Runtime) (bool, *gf_core.Gf_error) {


	claims := jwt.MapClaims{}
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
		return false, gErr
	}


	tokenValidBool := JWTtoken.Valid


	log.WithFields(log.Fields{}).Debug("JWT CLAIMS --------------")
	spew.Dump(claims)


	return tokenValidBool, nil
}

//-------------------------------------------------------------
// GENERATE__PIPELINE

// ADD!! - mark all other JWT's for this address as deleted to exclude them from future use.

func AuthJWTgeneratePipeline(pAddressStr glry_db.GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (string, *gf_core.Gf_error) {


	JWTkeyStr := AuthGenerateRandom()
	
	JWTissuerStr := "gallery" // string(pAddressStr)
	JWTtokenStr, gErr := AuthJWTgenerate(JWTkeyStr,
		JWTissuerStr,
		pAddressStr,
		pRuntime)
	if gErr != nil {
		return "", gErr
	}

	//------------------
	// DB
	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0

	IDstr := glry_db.AuthUserJWTkeyCreateID(pAddressStr,
		JWTkeyStr,
		creationTimeUNIXf)

	jwtKey := &glry_db.GLRYuserJWTkey {
		VersionInt:    0,
		ID:            IDstr,
		CreationTimeF: creationTimeUNIXf,
		DeletedBool:   false,
	
		ValueStr:   JWTkeyStr,
		AddressStr: pAddressStr,
	}

	gErr = glry_db.AuthUserJWTkeyCreate(jwtKey, pCtx, pRuntime)
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
	pIssuerStr  string,
	pAddressStr glry_db.GLRYuserAddress,
	pRuntime    *glry_core.Runtime) (string, *gf_core.Gf_error) {
	
	signingKeyBytesLst := []byte(pSigningKeyStr)

	//------------------
	// CLAIMS

	// Create the Claims
	creationTimeUNIXint := time.Now().UnixNano()/1000000000
	expiresAtUNIXint    := creationTimeUNIXint + pRuntime.Config.JWTtokenTTLsecInt //60*60*24*2 // expire N number of secs from now
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