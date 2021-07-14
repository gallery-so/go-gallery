package server

import (
	"context"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
// INPUT - USER_UPDATE
type authUserUpdateInput struct {
	UserId      persist.DbId `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	UserNameStr string       `json:"username"`
	BioStr      string       `json:"description"`
}

// INPUT - USER_GET
type authUserGetInput struct {
	UserId persist.DbId `json:"user_id" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET
type authUserGetOutput struct {
	UserNameStr string ` json:"username"`
	BioStr      string ` json:"bio"`
	Address     string ` json:"address"`
}

// INPUT - USER_LOGIN
type authUserLoginInput struct {
	SignatureStr string `json:"signature" validate:"required,min=4,max=50"`
	Address      string `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_LOGIN
type authUserLoginOutput struct {
	SignatureValidBool bool         `json:"signature_valid"`
	JWTtokenStr        string       `json:"jwt_token"`
	UserIDstr          persist.DbId `json:"user_id"`
	AddressStr         string       `json:"address"`
}

// INPUT - USER_GET_PREFLIGHT
type authUserGetPreflightInput struct {
	AddressStr string `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET_PREFLIGHT
type authUserGetPreflightOutput struct {
	NonceStr       string `json:"nonce"`
	UserExistsBool bool   `json:"user_exists"`
}

// INPUT - USER_CREATE - initial user creation is just an empty user, to store it in the DB.
//         this is to allow for users interupting the onboarding flow, and to be able to come back to it later
//         and the system recognize that their user already exists.
//         the users entering details on the user as they onboard are all user-update operations.
type authUserCreateInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	SignatureStr  string `json:"signature" validate:"required,min=80,max=200"`
	AddressStr    string `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	NonceValueStr string `json:"nonce"     validate:"required,min=10,max=150"`
}

// OUTPUT - USER_CREATE
type authUserCreateOutput struct {
	SignatureValidBool bool         `json:"signature_valid"`
	JWTtokenStr        string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserIDstr          persist.DbId `json:"user_id"`
}

//-------------------------------------------------------------
// USER_CREATE__PIPELINE
func userCreateDb(pInput *authUserCreateInput,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*authUserCreateOutput, error) {

	//------------------
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------
	output := &authUserCreateOutput{}

	//------------------
	// USER_CHECK
	// _, nonceValueStr, _, gErr := authUserCheck(pInput, pCtx, pRuntime)
	// if gErr != nil {
	// 	return nil, gErr
	// }

	//------------------
	// VERIFY_SIGNATURE

	dataStr := pInput.NonceValueStr
	sigValidBool, err := authVerifySignatureAllMethods(pInput.SignatureStr,
		dataStr,
		pInput.AddressStr,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.SignatureValidBool = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	//------------------

	user := &persist.User{
		AddressesLst: []string{pInput.AddressStr},
	}

	// DB
	id, err := persist.UserCreate(user, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserIDstr = id

	//------------------

	// JWT_GENERATION - signature is valid, so generate JWT key
	jwtTokenStr, gErr := jwtGeneratePipeline(id,
		pCtx,
		pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	output.JWTtokenStr = jwtTokenStr

	//------------------

	return output, nil
}

//-------------------------------------------------------------
// USER_GET__PIPELINE
func userGetDb(pInput *authUserGetInput,
	pAuthenticatedBool bool,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (*authUserGetOutput, error) {

	//------------------
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------

	user, err := persist.UserGetById(pInput.UserId, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	output := &authUserGetOutput{}
	if pAuthenticatedBool {
		output = &authUserGetOutput{
			UserNameStr: user.UserNameStr,
			BioStr:      user.BioStr,
		}
	} else {
		// TODO
	}

	return output, nil
}

//-------------------------------------------------------------
func userUpdateDb(pInput *authUserUpdateInput,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {

	//------------------
	// VALIDATE
	err := runtime.Validate(pInput, pRuntime)
	if err != nil {
		return err
	}

	//------------------

	return persist.UserUpdate(
		&persist.User{
			IDstr:       pInput.UserId,
			UserNameStr: pInput.UserNameStr,
			BioStr:      pInput.BioStr,
		},
		pCtx,
		pRuntime,
	)

}

//-------------------------------------------------------------
// USER_DELETE__PIPELINE
func userDeleteDb(pUserIDstr persist.DbId,
	pCtx context.Context,
	pRuntime *runtime.Runtime) error {
	return persist.UserDelete(pUserIDstr, pCtx, pRuntime)
}

//-------------------------------------------------------------
func userIsValid(pAddress string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) (bool, string, persist.DbId, error) {

	//------------------
	// CHECK_USER_EXISTS
	userExistsBool, err := persist.UserExistsByAddress(pAddress,
		pCtx,
		pRuntime)
	if err != nil {
		return false, "", "", err
	}

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, err := persist.AuthNonceGet(pAddress,
		pCtx,
		pRuntime)
	if err != nil {
		return false, "", "", err
	}

	// NONCE_NOT_FOUND - for this particular user
	var nonceValueStr string
	if nonce == nil {
		nonceValueStr = ""
	} else {
		nonceValueStr = nonce.ValueStr
	}

	//------------------
	// GET_ID

	var userIDstr persist.DbId
	if userExistsBool {

		user, err := persist.UserGetByAddress(pAddress, pCtx, pRuntime)
		if err != nil {
			return false, "", "", err
		}

		userIDstr = user.IDstr
	}

	//------------------

	return userExistsBool, nonceValueStr, userIDstr, nil
}
