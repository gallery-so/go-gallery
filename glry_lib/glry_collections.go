package glry_lib

import (
	"time"
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

// INPUT
// FIX!! - currently coll IDs are mongodb ID's, 
//         have some mongodb agnostic ID format.
type GLRYcollDeleteInput struct {
	IDstr string `json:"id" validate:"required,len=24"`
}

//-------------------------------------------------------------
// CREATE
func CollCreatePipeline(pInput *GLRYcollCreateInput,
	pUserIDstr string,
	pRuntime   *glry_core.Runtime) (*glry_db.GLRYcollection, *gfcore.Gf_error) {

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

	return coll, nil
}

//-------------------------------------------------------------
// DELETE
func CollDeletePipeline(pInput *GLRYcollDeleteInput,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {
	
	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return gErr
	}

	//------------------



	return nil
}

//-------------------------------------------------------------