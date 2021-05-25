package glry_lib

//-------------------------------------------------------------
import (
	"fmt"
	"math/rand"
	"time"
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"github.com/dgrijalva/jwt-go"
	log "github.com/sirupsen/logrus"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT
type GLRYauthUserLoginInput struct {
	SignatureStr string                  `json:"signature" validate:"required,min=4,max=50"`
	UsernameStr  string                  `json:"username"  validate:"required,min=2,max=20"`
	AddressStr   glry_db.GLRYuserAddress `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT
type GLRYauthUserLoginOutput struct {
	JWTtokenStr string `json:"nonce"`
}

// INPUT
type GLRYauthUserGetPublicInfoInput struct {
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT
type GLRYauthUserGetPublicInfoOutput struct {
	NonceStr string `json:"nonce"`
}

// INPUT - initial user creation is just an empty user, to store it in the DB.
//         this is to allow for users interupting the onboarding flow, and to be able to come back to it later
//         and the system recognize that their user already exists.
//         the users entering details on the user as they onboard are all user-update operations.
type GLRYauthUserCreateInput struct {
	// NameStr    string                  `json:"name"    validate:"required,min=2,max=20"`

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	SignatureStr string                  `json:"signature" validate:"required,min=4,max=50"`
	AddressStr   glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT
type GLRYauthUserCreateOutput struct {

	// JWT token is sent back to user to use to continue onboarding
	JWTtokenStr string `json:"nonce"`
}

// OUTPUT
type GLRYauthUserGetPublicInfoOutput struct {
	NonceStr string `json:"nonce"`
}

// JWT_CLAIMS
type GLRYjwtClaims struct {
	AddressStr glry_db.GLRYuserAddress `json:"address"`
	jwt.StandardClaims
}

//-------------------------------------------------------------
// JWT
//-------------------------------------------------------------
// GENERATE__PIPELINE

// ADD!! - mark all other JWT's for this address as deleted to exclude them from future use.

func AuthJWTgeneratePipeline(pAddressStr glry_db.GLRYuserAddress,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (string, *gfcore.Gf_error) {


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
	pRuntime    *glry_core.Runtime) (string, *gfcore.Gf_error) {
	
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
	pRuntime         *glry_core.Runtime) (bool, *gfcore.Gf_error) {


	claims := jwt.MapClaims{}
	JWTtoken, err := jwt.ParseWithClaims(pJWTtokenStr,
		&claims,
		func(pJWTtoken *jwt.Token) (interface{}, error) {
			return []byte(pJWTsecretKeyStr), nil
		})

	if err != nil {
		gErr := gfcore.Error__create("failed to verify JWT token for a user", 
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

