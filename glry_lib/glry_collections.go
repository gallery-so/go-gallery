package glry_lib

import (
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
)

//-------------------------------------------------------------
type GLRYcollInputCreate struct {
	NameStr string `json:"name" validate:"required, min=4, max=50"`
}

// FIX!! - currently coll IDs are mongodb ID's, 
//         have some mongodb agnostic ID format.
type GLRYcollInputDelete struct {
	IDstr string `json:"id" validate:"required, len=24"`
}

//-------------------------------------------------------------
func CollPipelineCreate(pInput *GLRYcollInputCreate,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	// VALIDATE
	gErr := HTTPvalidate(pInput, pRuntime)
	if gErr != nil {
		return gErr
	}




	return nil

}

//-------------------------------------------------------------
func CollPipelineDelete(pInput *GLRYcollInputDelete,
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	// VALIDATE
	gErr := HTTPvalidate(pInput, pRuntime)
	if gErr != nil {
		return gErr
	}



	return nil
}

//-------------------------------------------------------------