package glry_lib

//-------------------------------------------------------------
import (
	"fmt"
	"math/rand"
	"time"
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	
	"github.com/dgrijalva/jwt-go"
	
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT
type GLRYauthUserLoginInput struct {
	SignatureStr string                  `json:"signature" validate:"required,min=4,max=50"`
	UsernameStr  string                  `json:"username"  validate:"required,min=2,max=20"`
	AddressStr   glry_db.GLRYuserAddress `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// INPUT
type GLRYauthUserGetPublicInfoInput struct {
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT
type GLRYauthUserGetPublicInfoOutput struct {
	NonceStr string `json:"nonce"`
}

// INPUT
type GLRYauthUserCreateInput struct {
	NameStr    string                  `json:"name"    validate:"required,min=2,max=20"`
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

//-------------------------------------------------------------
// JWT
//-------------------------------------------------------------
// GENERATE__PIPELINE
func AuthJWTgeneratePipeline(pAddressStr glry_db.GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (string, *gfcore.Gf_error) {


	JWTkeyStr := AuthGenerateRandom()
	
	JWTissuerStr := string(pAddressStr)
	JWTtokenStr, gErr := AuthJWTgenerate(JWTkeyStr, JWTissuerStr, pRuntime)
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
	pIssuerStr string,
	pRuntime   *glry_core.Runtime) (string, *gfcore.Gf_error) {
	
	signingKeyBytesLst := []byte(pSigningKeyStr)

	// CLAIMS
	JWTclaims := &jwt.StandardClaims{
		ExpiresAt: 15000,
		Issuer:    pIssuerStr,
	}

	// SYMETRIC_SIGNING - same secret is used to both sign and validate tokens
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTclaims)

	// SIGN
	JWTtokenStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {

		gErr := gfcore.Error__create("failed to sign an Auth JWT token for a user", 
			"crypto_jwt_sign_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return "", gErr
	}
	
	return JWTtokenStr, nil
}

//-------------------------------------------------------------
func AuthJWTverifyPipeline(pJWTtokenStr string,
	pUserAddressStr glry_db.GLRYuserAddress,
	pCtx            context.Context,
	pRuntime        *glry_core.Runtime) (bool, *gfcore.Gf_error) {

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
		pCtx,
		pRuntime)
	if gErr != nil {
		return false, gErr
	}

	//------------------
	return tokenValidBool, nil
}

//-------------------------------------------------------------
// VERIFY
func AuthJWTverify(pJWTtokenStr string,
	pJWTkeyStr string,
	pCtx       context.Context, 
	pRuntime   *glry_core.Runtime) (bool, *gfcore.Gf_error) {



	token, err := jwt.ParseWithClaims(pJWTtokenStr,
		&CustomClaimsExample{},
		func(pToken *jwt.Token) (interface{}, error) {
			return pJWTkeyStr, nil
		})

	if err != nil {
		gErr := gfcore.Error__create("failed to verify JWT token for a user", 
			"crypto_jwt_verify_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return false, gErr
	}



	tokenValidBool := token.Valid
	return tokenValidBool, nil
}

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------
// NONCE_CREATE__PIPELINE
func AuthNonceCreatePipeline(pUserIDstr glry_db.GLRYuserID,
	pUserAddressStr glry_db.GLRYuserAddress,
	pCtx            context.Context,
	pRuntime        *glry_core.Runtime) (*glry_db.GLRYuserNonce, *gfcore.Gf_error) {
	
	// NONCE
	nonceStr := AuthGenerateRandom()

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	nonce := &glry_db.GLRYuserNonce{
		VersionInt:    0,
		ID:            primitive.NewObjectID(),
		CreationTimeF: creationTimeUNIXf,
		DeletedBool:   false,

		ValueStr:   nonceStr,
		UserIDstr:  pUserIDstr,
		AddressStr: pUserAddressStr,
	}

	// DB_CREATE
	gErr := glry_db.AuthNonceCreate(nonce, pCtx, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	return nonce, nil
}

//-------------------------------------------------------------
// NONCE_GENERATE
func AuthGenerateRandom() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt   := seededRand.Int()
	nonceStr   := fmt.Sprintf("%d", nonceInt)
	return nonceStr	  
}

