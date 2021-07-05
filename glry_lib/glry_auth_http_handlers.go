package glry_lib

import (
	// "fmt"
	"context"
	"net/http"

	gf_core "github.com/gloflow/gloflow/go/gf_core"
	gf_rpc_lib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
func AuthHandlersInit(pRuntime *glry_core.Runtime) {

	//-------------------------------------------------------------
	// AUTH_GET_PREFLIGHT
	// UN-AUTHENTICATED

	// called before login/sugnup calls, mostly to get nonce and also discover if user exists.

	// [GET] /glry/v1/auth/get_preflight?addr=:walletAddress
	gf_rpc_lib.Create_handler__http("/glry/v1/auth/get_preflight",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			log.WithFields(log.Fields{}).Debug("/glry/v1/auth/get_preflight")

			//------------------
			// INPUT

			qMap := pReq.URL.Query()
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

	gf_rpc_lib.Create_handler__http("/glry/v1/users/login",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			inputMap, gErr := gf_rpc_lib.Get_http_input(pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			var input GLRYauthUserLoginInput
			err := mapstructure.Decode(inputMap, &input)
			if err != nil {
				gf_err := gf_core.Error__create("failed to load input map into GLRYauthUserLoginInput struct",
					"mapstruct__decode",
					map[string]interface{}{},
					err, "glry_lib", pRuntime.RuntimeSys)
				return nil, gf_err
			}

			//------------------

			// USER_LOGIN__PIPELINE
			output, gErr := AuthUserLoginAndMemorizeAttemptPipeline(&input,
				pReq,
				pCtx,
				pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			// FAILED - INVALID_SIGNATURE
			if !output.SignatureValidBool {
				dataMap := map[string]interface{}{
					"sig_valid": false,
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
				"user_id":   output.UserIDstr,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_UPDATE
	// AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/update",
		precheckJwt(func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			if !getAuthFromCtx(pCtx) {
				return nil, gf_core.Error__create("jwt authentication required",
					"http_client_req_error",
					map[string]interface{}{}, nil,
					"glry_lib", pRuntime.RuntimeSys)
			}
			//------------------
			// INPUT
			qMap := pReq.URL.Query()
			userAddrStr := qMap["addr"][0]

			inputMap, gErr := gf_rpc_lib.Get_http_input(pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			var input GLRYauthUserUpdateInput
			err := mapstructure.Decode(inputMap, &input)
			if err != nil {
				gf_err := gf_core.Error__create("failed to load input map into GLRYauthUserUpdateInput struct",
					"mapstruct__decode",
					map[string]interface{}{},
					err, "glry_lib", pRuntime.RuntimeSys)
				return nil, gf_err
			}

			input.AddressStr = glry_db.GLRYuserAddress(userAddrStr)

			//------------------
			// UPDATE
			gErr = AuthUserUpdatePipeline(&input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{}

			//------------------

			return dataMap, nil
		}, pRuntime),
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_GET
	// AUTHENTICATED/UN-AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/get",
		precheckJwt(func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			authenticated := getAuthFromCtx(pCtx)

			//------------------
			// INPUT

			qMap := pReq.URL.Query()
			userAddrStr := qMap["addr"][0]

			input := &GLRYauthUserGetInput{
				AddressStr: glry_db.GLRYuserAddress(userAddrStr),
			}

			//------------------
			// USER_GET

			var output *GLRYauthUserGetOutput

			// AUTHENTICATED
			if authenticated {
				o, gErr := AuthUserGetPipeline(input,
					authenticated,
					pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}
				output = o
			} else {
				// UN_AUTHENTICATED - different set of results for user not-authenticated
				o, gErr := AuthUserGetPipeline(input,
					authenticated, // pAuthenticatedBool
					pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}
				output = o

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
		}, pRuntime),
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// USER_CREATE
	// UN-AUTHENTICATED

	gf_rpc_lib.Create_handler__http("/glry/v1/users/create",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			if pReq.Method == "POST" {

				log.WithFields(log.Fields{}).Debug("/glry/v1/users/create")

				//------------------
				// INPUT

				inputMap, gErr := gf_rpc_lib.Get_http_input(pResp, pReq, pRuntime.RuntimeSys)
				if gErr != nil {
					return nil, gErr
				}

				var input GLRYauthUserCreateInput
				err := mapstructure.Decode(inputMap, &input)
				if err != nil {
					gf_err := gf_core.Error__create("failed to load input map into GLRYauthUserCreateInput struct",
						"mapstruct__decode",
						map[string]interface{}{},
						err, "glry_lib", pRuntime.RuntimeSys)
					return nil, gf_err
				}

				//------------------
				// USER_CREATE
				output, gErr := AuthUserCreatePipeline(&input, pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT

				dataMap := map[string]interface{}{
					"sig_valid": output.SignatureValidBool,
					"jwt_token": output.JWTtokenStr,
					"user_id":   output.UserIDstr,
				}

				//------------------

				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
}
