package server

import (
	"net/http"
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	gfrpclib "github.com/gloflow/gloflow/go/gf_rpc_lib"
	"github.com/mikeydub/go-gallery/db"
	"github.com/mikeydub/go-gallery/extern_services"
)

//-------------------------------------------------------------
func initHandlers(pRuntimeSys *gfcore.Runtime_sys) {

	//-------------------------------------------------------------
	// NFTS_FOR_USER
	gfrpclib.Create_handler__http("/glry/v1/nfts",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {


			//------------------
			// INPUT

			//------------------

			userIDstr := "7bfaafcc-722e-4dce-986f-fe0d9bee2047"
			nfts, err := db.NFTgetByUserID(userIDstr, pCtx)
			if err != nil {
				

				return nil, err
			}




			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
				"nfts": nfts,
			}

			//------------------



			return data_map, nil
		},
		pRuntimeSys)

	//-------------------------------------------------------------
	// NFTS_FOR_USER
	gfrpclib.Create_handler__http("/glry/v1/nfts_opensea",
		func(pCtx context.Context, pResp http.ResponseWriter, pReq *http.Request) (map[string]interface{}, *gfcore.Gf_error) {


			//------------------
			// INPUT

			//------------------

			ownerWalletAddressStr := "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15"
			extern_services.OpenSeaGetAssetsForAccount(ownerWalletAddressStr, pCtx)



			//------------------
			// OUTPUT
			data_map := map[string]interface{}{
	
			}

			//------------------



			return data_map, nil
		},
		pRuntimeSys)

	//-------------------------------------------------------------






}