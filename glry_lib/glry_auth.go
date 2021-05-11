package glry_lib

//-------------------------------------------------------------
import (
	// "fmt"
	"math/rand"
	"time"
	"context"
	"go.mongodb.org/mongo-driver/bson/primitive"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
// INPUT
type GLRYauthUserVerifySignatureInput struct {
	NameStr      string                     `json:"name"    validate:"required,min=4,max=50"`
	AddressStr   glry_db.GLRYuserAddressStr `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	SignatureStr string                     `json:"name"    validate:"required,min=4, max=50"`
}

// INPUT
type GLRYauthUserGetPublicInfoInput struct {
	AddressStr glry_db.GLRYuserAddressStr `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// INPUT
type GLRYauthUserCreateInput struct {
	NameStr    string                     `json:"name"    validate:"required,min=4,max=50"`
	AddressStr glry_db.GLRYuserAddressStr `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

//-------------------------------------------------------------
// USER
//-------------------------------------------------------------
// USER_VERIFY_SIGNATURE__PIPELINE
func AuthUserUserVerifySignaturePipeline(pInput *GLRYauthUserVerifySignatureInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return gErr
	}

	//------------------


	return nil
}

//-------------------------------------------------------------
// USER_GET_PUBLIC_INFO__PIPELINE
func AuthUserGetPublicInfoPipeline(pInput *GLRYauthUserGetPublicInfoInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (int, *gfcore.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return 0, gErr
	}

	//------------------
	

	// DB_GET_USER_BY_ADDRESS
	user, gErr := glry_db.AuthUserGetByAddress(pInput.AddressStr, pCtx, pRuntime)
	if gErr != nil {
		return 0, gErr
	}

	// NO_USER_FOUND - user doesnt exist in the system, and so return an empty response
	//                 to the front-end. subsequently the client has to create a new user.
	if user == nil {



		// NONCE_CREATE
		nonce, gErr := AuthNonceCreatePipeline(glry_db.GLRYuserID(""), pInput.AddressStr, pCtx, pRuntime)
		if gErr != nil {
			return 0, gErr
		}


		return nonce.NonceInt, nil
	}

	// NONCE_GET
	nonce, gErr := glry_db.AuthNonceGet(pInput.AddressStr, pCtx, pRuntime)
	if gErr != nil {
		return 0, gErr
	}
	
	return nonce.NonceInt, nil
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
		AddressesLst:  []glry_db.GLRYuserAddressStr{addressStr, },
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
// NONCE
//-------------------------------------------------------------
// NONCE_CREATE__PIPELINE
func AuthNonceCreatePipeline(pUserIDstr glry_db.GLRYuserID,
	pUserAddressStr glry_db.GLRYuserAddressStr,
	pCtx            context.Context,
	pRuntime        *glry_core.Runtime) (*glry_db.GLRYuserNonce, *gfcore.Gf_error) {
	
	// NONCE
	nonceInt := AuthNonceGenerate()

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	nonce := &glry_db.GLRYuserNonce{
		VersionInt:    0,
		ID:            primitive.NewObjectID(),
		CreationTimeF: creationTimeUNIXf,
		DeletedBool:   false,

		NonceInt:   nonceInt,
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
func AuthNonceGenerate() int {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt   := seededRand.Int()
	return nonceInt	  
}