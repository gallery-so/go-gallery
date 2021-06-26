package glry_lib

import (
	"fmt"
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

			if pReq.Method == "GET" {

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
						}, nil, "glry_core", pRuntime.RuntimeSys)
				}

				nfts, gErr := glry_db.NFTgetByID(nftIDstr, pCtx, pRuntime)
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
