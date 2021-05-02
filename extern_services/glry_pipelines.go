package extern_services

import (
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
func OpenSeaPipelineAssetsForAcc(pOwnerWalletAddressStr string,
	pCtx context.Context,
	pRuntimeSys *gfcore.Runtime_sys) *gfcore.Gf_error {




	openSeaAssetsForAccLst, gErr := OpenSeaFetchAssetsForAcc(pOwnerWalletAddressStr,
		pCtx,
		pRuntimeSys)
	if gErr != nil {
		return gErr
	}




	for _, openSeaAsset := range openSeaAssetsForAccLst {




		nft := &GLRYnft{
			VersionInt: 0
			IDstr
			ImageURL       
			Description     
			NameStr:           openSeaAsset.NameStr,
			CollectionNameStr: 


			
			ExternalURL    
			CreatedDateF   
			CreatorAddress   
			ContractAddress  
			OpenSeaTokenID          

			
			ImageThumbnailURL
			ImagePreviewURL   


			Position          int64
			Hidden            bool
		}

		NFTcreate


	}










}