package glry_lib

import (
	"context"
	gf_core "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
	"time"
)

//-------------------------------------------------------------
// INPUT
type GLRYcollGetInput struct {
	UserIDstr glry_db.GLRYuserID `validate:"required,min=4,max=50"`
}

// OUTPUT
type GLRYcollGetOutput struct {
	CollsOutputsLst []map[string]interface{}
}

// INPUT
type GLRYcollCreateInput struct {
	NameStr        string `json:"name"        validate:"required,min=4,max=50"`
	DescriptionStr string `json:"description" validate:"required,min=0,max=500"`
}

// OUTPUT
type GLRYcollCreateOutput struct {
	IDstr   glry_db.GLRYcollID `json:"id"`
	NameStr string             `json:"name"`
}

// INPUT
// FIX!! - currently coll IDs are mongodb ID's,
//         have some mongodb agnostic ID format.
type GLRYcollDeleteInput struct {
	IDstr string `json:"id" validate:"required,len=24"`
}

// OUTPUT
type GLRYcollDeleteOutput struct {
}

//-------------------------------------------------------------
func CollGetPipeline(pInput *GLRYcollGetInput,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollGetOutput, *gf_core.Gf_error) {

	collsLst, gErr := glry_db.CollGetByUserID(pInput.UserIDstr,
		pCtx,
		pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	collsOutputsLst := []map[string]interface{}{}
	for _, coll := range collsLst {

		/*
			COLL_OUTPUT:
			{
				id: 1,
				isHidden: true,
				name: 'Cool Collection',
				description: 'my favorites',
				// ! note: we want the CREATOR opensea username, not OWNER username
				nfts: [ { id: 1, name: 'cool nft', creator_username_opensea: 'ColorGlyphs' }, {}, {}, ...]
			},
		*/
		collOutputMap := map[string]interface{}{
			"id":          coll.IDstr,
			"hidden":      coll.HiddeBool,
			"name":        coll.NameStr,
			"description": coll.DescriptionStr,
			"nfts":        []map[string]interface{}{},
		}

		collsOutputsLst = append(collsOutputsLst, collOutputMap)
	}

	output := &GLRYcollGetOutput{
		CollsOutputsLst: collsOutputsLst,
	}

	return output, nil
}

//-------------------------------------------------------------
// CREATE
func CollCreatePipeline(pInput *GLRYcollCreateInput,
	pUserIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollCreateOutput, *gf_core.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	creationTimeUNIXf := float64(time.Now().UnixNano()) / 1000000000.0
	nameStr := pInput.NameStr
	ownerUserIDstr := pUserIDstr
	IDstr := glry_db.CollCreateID(nameStr, ownerUserIDstr, creationTimeUNIXf)

	coll := &glry_db.GLRYcollection{
		VersionInt:    0,
		IDstr:         IDstr,
		CreationTimeF: creationTimeUNIXf,

		NameStr:        nameStr,
		DescriptionStr: pInput.DescriptionStr,
		OwnerUserIDstr: ownerUserIDstr,
		DeletedBool:    false,
		NFTsLst:        []string{},
	}

	// DB
	gErr = glry_db.CollCreate(coll, pCtx, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	output := &GLRYcollCreateOutput{
		IDstr:   coll.IDstr,
		NameStr: coll.NameStr,
	}

	return output, nil
}

//-------------------------------------------------------------
// DELETE
func CollDeletePipeline(pInput *GLRYcollDeleteInput,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollDeleteOutput, *gf_core.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	return nil, nil
}

//-------------------------------------------------------------
