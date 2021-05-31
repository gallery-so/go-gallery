package glry_lib

import (
	"net/http"
	"context"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	gf_rpc_lib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
// INPUT - USER_UPDATE
type GLRYauthUserUpdateInput struct {
	AddressStr        glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	UserNameNewStr    string
	DescriptionNewStr string
}

// INPUT - USER_GET
type GLRYauthUserGetInput struct {
	AddressStr   glry_db.GLRYuserAddress `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET
type GLRYauthUserGetOutput struct {
	UserNameStr    string 
	DescriptionStr string
}

// INPUT - USER_LOGIN
type GLRYauthUserLoginInput struct {
	SignatureStr string                  `json:"signature" validate:"required,min=4,max=50"`
	UsernameStr  string                  `json:"username"  validate:"required,min=2,max=20"`
	AddressStr   glry_db.GLRYuserAddress `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_LOGIN
type GLRYauthUserLoginOutput struct {
	UserExistsBool     bool
	SignatureValidBool bool
	JWTtokenStr        string
	NonceValueStr      string
}

// INPUT - USER_GET_PREFLIGHT
type GLRYauthUserGetPreflightInput struct {
	AddressStr glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET_PREFLIGHT
type GLRYauthUserGetPriflightOutput struct {
	NonceStr       string
	UserExistsBool bool
}

// INPUT - USER_CREATE - initial user creation is just an empty user, to store it in the DB.
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

// OUTPUT - USER_CREATE
type GLRYauthUserCreateOutput struct {

	// JWT token is sent back to user to use to continue onboarding
	JWTtokenStr string
}

//-------------------------------------------------------------
func AuthHandlersInit(pRuntime *glry_core.Runtime) {
	
	//-------------------------------------------------------------
	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?addr=:walletAddress
	gf_rpc_lib.Create_handler__http("/glry/v1/auth/get_preflight",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			qMap        := pReq.URL.Query()
			userAddrStr := qMap["addr"][0]

			input := &GLRYauthUserGetPreflightInput{
				AddressStr: glry_db.GLRYuserAddress(userAddrStr),
			}

			//------------------
			
			// GET_PUBLIC_INFO
			output, gErr := AuthUserGetPreflightPipeline(input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"nonce":       output.NonceStr,
				"user_exists": output.UserExistsBool,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// AUTH_USER_LOGIN
	// UN-AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/auth/login",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			var input GLRYauthUserLoginInput
			inputParsed, gErr := gf_rpc_lib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			
			// USER_LOGIN__PIPELINE
			output, gErr := AuthUserLoginAndMemorizeAttemptPipeline(inputParsed.(*GLRYauthUserLoginInput),
				pReq,
				pCtx,
				pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			// FAILED - NO_USER
			if !output.UserExistsBool {
				dataMap := map[string]interface{}{
					"user_exists": false,
				}
				return dataMap, nil
			}

			// FAILED - INVALID_SIGNATURE
			if !output.SignatureValidBool {
				dataMap := map[string]interface{}{
					"sig_valid": false,
				}
				return dataMap, nil
			}

			// FAILED - NO_NONCE_FOUND
			if output.NonceValueStr == "" {
				dataMap := map[string]interface{}{
					"nonce_found": false,
				}
				return dataMap, nil
			}

			/*
			// ADD!! - going forward we should follow this approach, after v1
			// SET_JWT_COOKIE
			expirationTime := time.Now().Add(time.Duration(pRuntime.Config.JWTtokenTTLsecInt/60) * time.Minute)
			http.SetCookie(pResp, &http.Cookie{
				Name:    "glry_token",
				Value:   userJWTtokenStr,
				Expires: expirationTime,
			})*/

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"jwt_token": output.JWTtokenStr,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/update",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT
			qMap        := pReq.URL.Query()
			userAddrStr := qMap["addr"][0]

			var input GLRYauthUserUpdateInput
			_, gErr := gf_rpc_lib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}
			input.AddressStr = glry_db.GLRYuserAddress(userAddrStr)

			//------------------
			// JWT_VERIFY
			validJWTbool, dataJWTmap, gErr := AuthJWTverifyHTTP(input.AddressStr,
				pReq,
				pCtx,
				pRuntime)
			if gErr != nil {
				return nil, gErr
			}
			if !validJWTbool {
				return dataJWTmap, nil
			}
			
			//------------------
			// UPDATE
			gErr = AuthUserUpdatePipeline(&input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}
			
			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/get",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			qMap        := pReq.URL.Query()
			userAddrStr := qMap["addr"][0]

			input := &GLRYauthUserGetInput{
				AddressStr: glry_db.GLRYuserAddress(userAddrStr),
			}

			//------------------
			// JWT_VERIFY
			validJWTbool, _, gErr := AuthJWTverifyHTTP(input.AddressStr,
				pReq,
				pCtx,
				pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// USER_GET


			if validJWTbool {
				// return one set of results for user authenticated
			} else {
				// different set of results for user not-authenticated
			}

			output, gErr := AuthUserGetPipeline(input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"username":    output.UserNameStr,
				"description": output.DescriptionStr,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)
	
	//-------------------------------------------------------------
	// AUTH_SIGNUP
	// UN-AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/auth/signup",
	func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

		//------------------
		// INPUT


		//------------------
		
		//------------------
		// OUTPUT
		dataMap := map[string]interface{}{
			
		}

		//------------------

		return dataMap, nil
	},
	pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// AUTH_SIGNUP

	gf_rpc_lib.Create_handler__http("/glry/v1/auth/signup",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {
			
			if pReq.Method == "POST" {
				//------------------
				// INPUT

				var input GLRYauthUserCreateInput
				inputParsed, gErr := gf_rpc_lib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// GET_PUBLIC_INFO
				user, gErr := AuthUserCreatePipeline(inputParsed.(*GLRYauthUserCreateInput), pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT
				dataMap := map[string]interface{}{
					"id": user.IDstr,
					// "nonce": user.NonceInt,
				}

				//------------------

				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)
}