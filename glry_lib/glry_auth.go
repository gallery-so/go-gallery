package glry_lib

//-------------------------------------------------------------
import (
	"fmt"
	"math/rand"
	"time"
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/dgrijalva/jwt-go"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT
type GLRYauthUserVerifySignatureInput struct {
	SignatureStr string                  `json:"name"     validate:"required,min=4, max=50"`
	PubKeyStr    string                  `json:"pubkey"   validate:"required,len=42"`
	UserNameStr  string                  `json:"username" validate:"required,min=4,max=50"`
	AddressStr   glry_db.GLRYuserAddress `json:"address"  validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// INPUT
type GLRYauthUserGetPublicInfoInput struct {
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// INPUT
type GLRYauthUserCreateInput struct {
	NameStr    string                  `json:"name"    validate:"required,min=4,max=50"`
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

//-------------------------------------------------------------
// USER
//-------------------------------------------------------------
// USER_LOGIN__PIPELINE
func AuthUserLoginPipeline(pInput *GLRYauthUserVerifySignatureInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (bool, string, *gfcore.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return false, "", gErr
	}

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, gErr := glry_db.AuthNonceGet(pInput.AddressStr,
		pCtx,
		pRuntime)
	if gErr != nil {
		return false, "", gErr
	}

	//------------------
	// VERIFY_SIGNATURE

	validBool, gErr := AuthUserVerifySignature(pInput.SignatureStr,
		nonce.ValueStr,
		pInput.PubKeyStr,
		pRuntime) 
	if gErr != nil {
		return false, "", gErr
	}

	//------------------
	// JWT_GENERATION



	signingKeyStr := ""
	JWTsignedStr, gErr := AuthJWTgenerate(signingKeyStr, pRuntime)
	if gErr != nil {


		return false, JWTsignedStr, gErr
	}


	//------------------


	return validBool, "", nil
}

//-------------------------------------------------------------
// USER_VERIFY_SIGNATURE
func AuthUserVerifySignature(pSignatureStr string,
	pNonceStr  string,
	pPubKeyStr string,
	pRuntime   *glry_core.Runtime) (bool, *gfcore.Gf_error) {

	// https://goethereumbook.org/signature-verify/




	// eth_sign
	// http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))
	
	/*publicKeyBytes := crypto.FromECDSAPub(publicKeyECDSA)
	
	hash := crypto.Keccak256Hash(data)
	fmt.Println(hash.Hex()) // 0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8

	// elliptic curve signature recover
	sigPublicKey, err := crypto.Ecrecover(hash.Bytes(), signature)
	if err != nil {
		
	// log.Fatal(err)

	}*/


	// DATA
	dataStr      := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(fmt.Sprintf(pNonceStr)), pNonceStr)
	dataBytesLst := []byte(dataStr)
	dataHash     := crypto.Keccak256Hash(dataBytesLst)

	// SIGNATURE
	signatureNoRecoverID := []byte(pSignatureStr[:len(pSignatureStr)-1]) // remove recovery id
	

	

	fmt.Println(pSignatureStr)

	pubKey, err := crypto.SigToPub(dataHash.Bytes(), []byte(pSignatureStr))
	if err != nil {
		
		fmt.Println(err)
		panic("aaaaaaa")
	}

	fmt.Println("=============== PUBKEY")
	spew.Dump(pubKey)


	
	// PUBLIC_KEY
	publicKeyBytesLst := []byte(pPubKeyStr)


	verifiedBool := crypto.VerifySignature(publicKeyBytesLst, dataHash.Bytes(), signatureNoRecoverID)


	return verifiedBool, nil
}

//-------------------------------------------------------------
// USER_GET_PUBLIC_INFO__PIPELINE
func AuthUserGetPublicInfoPipeline(pInput *GLRYauthUserGetPublicInfoInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (string, *gfcore.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return "", gErr
	}

	//------------------
	

	// DB_GET_USER_BY_ADDRESS
	user, gErr := glry_db.AuthUserGetByAddress(pInput.AddressStr, pCtx, pRuntime)
	if gErr != nil {
		return "", gErr
	}

	// NO_USER_FOUND - user doesnt exist in the system, and so return an empty response
	//                 to the front-end. subsequently the client has to create a new user.
	if user == nil {



		// NONCE_CREATE
		nonce, gErr := AuthNonceCreatePipeline(glry_db.GLRYuserID(""), pInput.AddressStr, pCtx, pRuntime)
		if gErr != nil {
			return "", gErr
		}


		return nonce.ValueStr, nil
	}

	// NONCE_GET
	nonce, gErr := glry_db.AuthNonceGet(pInput.AddressStr, pCtx, pRuntime)
	if gErr != nil {
		return "", gErr
	}
	
	return nonce.ValueStr, nil
}

//-------------------------------------------------------------
// USER_CREATE__PIPELINE
func AuthUserCreatePipeline(pInput *GLRYauthUserCreateInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*glry_db.GLRYuser, *gfcore.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	nameStr           := pInput.NameStr
	addressStr        := pInput.AddressStr
	IDstr := glry_db.AuthUserCreateID(nameStr,
		addressStr,
		creationTimeUNIXf)

	

	user := &glry_db.GLRYuser{
		VersionInt:    0,
		IDstr:         IDstr,
		CreationTimeF: creationTimeUNIXf,
		NameStr:       nameStr,
		AddressesLst:  []glry_db.GLRYuserAddress{addressStr, },
		// NonceInt:      nonceInt,
	}

	gErr = glry_db.AuthUserCreate(user, pCtx, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	return user, nil
}

//-------------------------------------------------------------
// USER_DELETE__PIPELINE
func AuthUserDeletePipeline(pUserIDstr glry_db.GLRYuserID,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {


	
	gErr := glry_db.AuthUserDelete(pUserIDstr, pCtx, pRuntime)
	if gErr != nil {
		return gErr
	}


	

	return nil
}

//-------------------------------------------------------------
// JWT
//-------------------------------------------------------------

func AuthJWTgenerate(pSigningKeyStr string,
	pRuntime *glry_core.Runtime) (string, *gfcore.Gf_error) {

	
	signingKeyBytesLst := []byte(pSigningKeyStr)

	// CLAIMS
	JWTclaims := &jwt.StandardClaims{
		ExpiresAt: 15000,
		Issuer:    "test", // FIX!! - use a proper "issuer" value here.
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTclaims)

	// SIGN
	JWTsignedStr, err := token.SignedString(signingKeyBytesLst)
	if err != nil {

		gErr := gfcore.Error__create("failed to sign an Auth JWT token for a user", 
			"jwt_sign_token_error",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return "", gErr
	}
	





	return JWTsignedStr, nil
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
	nonceStr := AuthNonceGenerate()

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
func AuthNonceGenerate() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt   := seededRand.Int()
	nonceStr   := fmt.Sprintf("%d", nonceInt)
	return nonceStr	  
}

