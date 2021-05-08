package glry_lib

import (
	"fmt"
	"net/http"
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	gfrpclib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"github.com/mikeydub/go-gallery/glry_extern_services"
)

//-------------------------------------------------------------
func HandlersInit(pRuntime *glry_core.Runtime) {


	//-------------------------------------------------------------
	// COLLECTION_CREATE

	gfrpclib.Create_handler__http("/glry/v1/auth/users",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {



			//------------------
			// INPUT

			qMap := pReq.URL.Query()
			userPublicAddrStr := qMap["pubaddr"]
			//------------------
			

			fmt.Println(userPublicAddrStr)

			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
			
			}

			//------------------

			return data_map, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// COLLECTION_CREATE

	gfrpclib.Create_handler__http("/glry/v1/collections/create",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {

			//------------------
			// INPUT

			var input GLRYcollInputCreate
			inputParsed, gErr := gfrpclib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			// FINISH!! - get user_id mechanism
			userIDstr := ""

			//------------------


			coll, gErr := CollPipelineCreate(inputParsed.(*GLRYcollInputCreate), userIDstr, pRuntime)
			if gErr != nil {
				return nil, gErr
			}
			
			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
				"coll_id": coll.IDstr,
			}

			//------------------

			return data_map, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// COLLECTION_DELETE
	gfrpclib.Create_handler__http("/glry/v1/collections/delete",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {

			//------------------
			// INPUT

			var input GLRYcollInputDelete
			inputParsed, gErr := gfrpclib.Get_http_input_to_struct(input, pResp, pReq, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}

			//------------------

			gErr = CollPipelineDelete(inputParsed.(*GLRYcollInputDelete), pRuntime)
			if gErr != nil {
				return nil, gErr
			}

			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
		
			}

			//------------------

			return data_map, nil
		},
		pRuntime.RuntimeSys)
		
	//-------------------------------------------------------------
	// NFTS_FOR_USER__GET
	gfrpclib.Create_handler__http("/glry/v1/nfts/user_get",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {

			if pReq.Method == "GET" {

				//------------------
				// INPUT

				//------------------

				userIDstr := "7bfaafcc-722e-4dce-986f-fe0d9bee2047"
				nfts, gErr := glry_db.NFTgetByUserID(userIDstr, pCtx, pRuntime.RuntimeSys)
				if gErr != nil {
					return nil, gErr
				}

				//------------------
				// OUTPUT
				data_map := map[string]interface{}{
					"nfts": nfts,
				}

				//------------------

				return data_map, nil
			}

			return nil, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
	// NFTS_FROM_OPENSEA__GET
	gfrpclib.Create_handler__http("/glry/v1/nfts/opensea_get",
		func(pCtx context.Context, pResp http.ResponseWriter,
			pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {

			//------------------
			// INPUT

			//------------------

			ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
			_, gErr := glry_extern_services.OpenSeaPipelineAssetsForAcc(ownerWalletAddressStr, pCtx, pRuntime.RuntimeSys)
			if gErr != nil {
				return nil, gErr
			}


			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
	
			}

			//------------------

			return data_map, nil
		},
		pRuntime.RuntimeSys)

	//-------------------------------------------------------------
}