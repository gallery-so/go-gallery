package glry_lib

import (
	"time"
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
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

type GLRYcollDeleteOutput struct {

}


//-------------------------------------------------------------
// CREATE
func CollCreatePipeline(pInput *GLRYcollCreateInput,
	pUserIDstr string,
	pCtx       context.Context,
	pRuntime   *glry_core.Runtime) (*GLRYcollCreateOutput, *gfcore.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	nameStr        := pInput.NameStr
	ownerUserIDstr := pUserIDstr
	IDstr          := glry_db.CollCreateID(nameStr, ownerUserIDstr, creationTimeUNIXf)

	coll := &glry_db.GLRYcollection {
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
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollDeleteOutput, *gfcore.Gf_error) {
	
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