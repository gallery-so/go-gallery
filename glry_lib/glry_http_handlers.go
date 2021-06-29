package glry_lib

import (
	"fmt"
	"os"
	"strings"

	// "time"
	"context"
	"net/http"

	// log "github.com/sirupsen/logrus"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	gf_rpc_lib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mikeydub/go-gallery/glry_extern_services"
	"github.com/mitchellh/mapstructure"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *glry_core.Runtime) {

	// AUTH_HANDLERS
	AuthHandlersInit(pRuntime)

	//-------------------------------------------------------------
	// COLLECTION
	//-------------------------------------------------------------
	// COLLECTION_GET

	gf_rpc_lib.Create_handler__http("/glry/v1/collections/get",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			qMap := pReq.URL.Query()
			userIDstr := qMap["userid"][0]

			input := &GLRYcollGetInput{
				UserIDstr: glry_db.GLRYuserID(userIDstr),
			}

			//------------------
			// CREATE
			output, gErr := CollGetPipeline(input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"colls": output.CollsOutputsLst,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// COLLECTION_CREATE

	gf_rpc_lib.Create_handler__http("/glry/v1/collections/create",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			inputMap, gErr := gf_rpc_lib.Get_http_input(pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			// FINISH!! - get user_id mechanism
			userIDstr := ""

			var input GLRYcollCreateInput
			err := mapstructure.Decode(inputMap, &input)
			if err != nil {
				gf_err := gf_core.Error__create("failed to load input map into GLRYcollCreateInput struct",
					"mapstruct__decode",
					map[string]interface{}{},
					err, "glry_lib", pRuntime.RuntimeSys)
				return nil, gf_err
			}

			//------------------
			// CREATE
			output, gErr := CollCreatePipeline(&input, userIDstr, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			fmt.Println(output)

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// COLLECTION_DELETE
	gf_rpc_lib.Create_handler__http("/glry/v1/collections/delete",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			inputMap, gErr := gf_rpc_lib.Get_http_input(pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			var input GLRYcollDeleteInput
			err := mapstructure.Decode(inputMap, &input)
			if err != nil {
				gf_err := gf_core.Error__create("failed to load input map into GLRYcollDeleteInput struct",
					"mapstruct__decode",
					map[string]interface{}{},
					err, "glry_lib", pRuntime.RuntimeSys)
				return nil, gf_err
			}

			//------------------

			_, gErr = CollDeletePipeline(&input, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// NFTS
	//-------------------------------------------------------------
	// SINGLE GET
	gf_rpc_lib.Create_handler__http("/glry/v1/nfts/get",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			if pReq.Method == http.MethodGet {

				//------------------
				// INPUT

				//------------------

				qMap := pReq.URL.Query()

				nftIDstr := qMap.Get("id")

				if nftIDstr == "" {
					// is this the right way to create an error for gf_core?
					return nil, gf_core.Error__create("nft id not found in query values",
						"http_client_req_error",
						map[string]interface{}{
							"uri": "/glry/v1/nfts/get",
						}, nil, "glry_lib", pRuntime.RuntimeSys)
				}

				nfts, gErr := glry_db.NFTgetByID(nftIDstr, pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				if len(nfts) == 0 {
					return nil, gf_core.Error__create(fmt.Sprintf("no nfts found with id: %s", nftIDstr),
						"http_client_req_error",
						map[string]interface{}{
							"uri": "/glry/v1/nfts/get",
						}, nil, "glry_lib", pRuntime.RuntimeSys)
				}

				//------------------
				// OUTPUT
				dataMap := map[string]interface{}{
					"nfts": nfts,
				}

				//------------------
				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	// SINGLE UPDATE
	gf_rpc_lib.Create_handler__http("/glry/v1/nfts/update",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			if pReq.Method == http.MethodPut {

				//------------------
				// INPUT

				//------------------

				pReq.ParseForm()

				nftIDstr := pReq.FormValue("id")

				if nftIDstr == "" {
					return nil, gf_core.Error__create("no id found in form values",
						"http_client_req_error",
						map[string]interface{}{
							"uri": "/glry/v1/nfts/update",
						}, nil, "glry_lib", pRuntime.RuntimeSys)
				}

				nft := &glry_db.GLRYnft{}

				gErr := glry_core.UnmarshalBody(nft, pReq.Body, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				gErr = glry_db.NFTupdateById(nftIDstr, nft, pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT
				dataMap := map[string]interface{}{}

				//------------------
				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	// USER_GET
	gf_rpc_lib.Create_handler__http("/glry/v1/nfts/user_get",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			if pReq.Method == "GET" {

				//------------------
				// INPUT

				//------------------

				userIDstr := "7bfaafcc-722e-4dce-986f-fe0d9bee2047"
				nfts, gErr := glry_db.NFTgetByUserID(userIDstr, pCtx, pRuntime)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT
				dataMap := map[string]interface{}{
					"nfts": nfts,
				}

				//------------------
				return dataMap, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// OPENSEA_GET
	gf_rpc_lib.Create_handler__http("/glry/v1/nfts/opensea_get",
		func(pCtx context.Context, pResp http.ResponseWriter,
			pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			//------------------

			ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
			_, gErr := glry_extern_services.OpenSeaPipelineAssetsForAcc(ownerWalletAddressStr, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// VAR
	//-------------------------------------------------------------
	// HEALTH

	gf_rpc_lib.Create_handler__http("/glry/v1/health",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			// log.WithFields(log.Fields{}).Debug("/health")

			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"msg": "gallery operational",
				"env": pRuntime.Config.EnvStr,
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
}

// information to be added to context with middlewares
type contextKey string

type authContextValue struct {
	AuthenticatedBool bool
	UserAddressStr    string
}

// jwt middleware
// parameter hell because gf_core http_handler is private :(
// both funcs (param and return funcs) are of type gf_core.http_handler implicitly
func precheckJwt(midd func(pCtx context.Context, pResp http.ResponseWriter,
	pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error), pRuntime *glry_core.Runtime) func(context.Context, http.ResponseWriter,
	*http.Request) (map[string]interface{}, *gf_core.Gf_error) {
	return func(pCtx context.Context, pResp http.ResponseWriter,
		pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

		authHeaders := strings.Split(pReq.Header.Get("Authorization"), " ")
		if len(authHeaders) > 0 {
			// get string after "Bearer"
			jwt := authHeaders[1]
			// use an env variable as jwt secret as upposed to using a stateful secret stored in
			// database that is unique to every user and session
			valid, userAddr, gErr := AuthJWTverify(jwt, os.Getenv("JWT_SECRET"), pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			// using a struct for storing values with a kard
			pCtx = context.WithValue(pCtx, contextKey("auth"), authContextValue{
				AuthenticatedBool: valid,
				UserAddressStr:    userAddr,
			})
		}
		return midd(pCtx, pResp, pReq)
	}
}

func getAuthFromCtx(pCtx context.Context) bool {
	if value, ok := pCtx.Value("auth").(authContextValue); ok {
		return value.AuthenticatedBool
	} else {
		return false
	}
}
