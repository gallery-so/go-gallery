package glry_lib

import (
	// "fmt"
	// "time"
	"net/http"
	"context"
	log "github.com/sirupsen/logrus"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	gf_rpc_lib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mikeydub/go-gallery/glry_extern_services"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *glry_core.Runtime) {



	log.WithFields(log.Fields{}).Debug("initializing HTTP handlers")


	// AUTH_HANDLERS
	AuthHandlersInit(pRuntime)

	//-------------------------------------------------------------
	// HEALTH

	gf_rpc_lib.Create_handler__http("/glry/v1/health",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			log.WithFields(log.Fields{}).Debug("/health")

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
	// COLLECTION_CREATE

	gf_rpc_lib.Create_handler__http("/glry/v1/collections/create",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gf_core.Gf_error) {

			//------------------
			// INPUT

			var input GLRYcollCreateInput
			inputParsed, gErr := gf_rpc_lib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			// FINISH!! - get user_id mechanism
			userIDstr := ""

			//------------------


			coll, gErr := CollCreatePipeline(inputParsed.(*GLRYcollCreateInput), userIDstr, pCtx, pRuntime)
			if gErr != nil {
				return nil, gErr
			}
			
			//------------------
			// OUTPUT
			dataMap := map[string]interface{}{
				"coll_id": coll.IDstr,
			}

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

			var input GLRYcollDeleteInput
			inputParsed, gErr := gf_rpc_lib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			//------------------

			_, gErr = CollDeletePipeline(inputParsed.(*GLRYcollDeleteInput), pCtx, pRuntime)
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
	// NFTS_FOR_USER__GET
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
	// NFTS_FROM_OPENSEA__GET
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
			dataMap := map[string]interface{}{
	
			}

			//------------------

			return dataMap, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
}