package glry_lib

import (
	"net/http"
	"context"
	"github.com/mitchellh/mapstructure"
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
	AddressStr   glry_db.GLRYuserAddress `json:"address" validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET
type GLRYauthUserGetOutput struct {
	UserNameStr    string   `mapstructure:"username"`
	DescriptionStr string   `mapstructure:"description"`
	AddressesLst   []string `mapstructure:"addresses`
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

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	SignatureStr  string                  `json:"signature" validate:"required,min=4,max=50"`
	AddressStr    glry_db.GLRYuserAddress `json:"address"   validate:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	NonceValueStr string                  `json:"nonce"     validate:"required,len=50"`
}

// OUTPUT - USER_CREATE
type GLRYauthUserCreateOutput struct {
	UserExistsBool     bool
	NonceValueStr      string
	SignatureValidBool bool
	JWTtokenStr        string // JWT token is sent back to user to use to continue onboarding
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
	// AUTHENTICATED/UN-AUTHENTICATED

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

			var output *GLRYauthUserGetOutput

			// AUTHENTICATED
			if validJWTbool {
				output, gErr = AuthUserGetPipeline(input,
					true, // pAuthenticatedBool
					pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}


			// UN_AUTHENTICATED - different set of results for user not-authenticated
			} else {
				output, gErr = AuthUserGetPipeline(input,
					false, // pAuthenticatedBool
					pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

			}


			


			//------------------
			// OUTPUT

			var dataMap map[string]interface{}
			err := mapstructure.Decode(output, &dataMap)
			if err != nil {
				gf_err := gf_core.Error__create("failed to load user_get pipeline output into a map",
					"mapstruct__decode",
					map[string]interface{}{},
					err, "glry_lib", pRuntime.RuntimeSys)
				return nil, gf_err
			}

			// dataMap := map[string]interface{}{
			// 	"username":    output.UserNameStr,
			// 	"description": output.DescriptionStr,
			// }

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/create",
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
				// USER_CREATE
				output, gErr := AuthUserCreatePipeline(inputParsed.(*GLRYauthUserCreateInput), pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT

				dataMap := map[string]interface{}{
					"user_exists": output.UserExistsBool,
					"nonce":       output.NonceValueStr,
					"sig_valid":   output.SignatureValidBool,
					"jwt_token":   output.JWTtokenStr,
				}

				//------------------

				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
}