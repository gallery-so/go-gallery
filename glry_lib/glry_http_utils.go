package glry_lib

import (
	"github.com/mikeydub/go-gallery/glry_core"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
func HTTPvalidate(pInput interface{},
	pRuntime *glry_core.Runtime) *gfcore.Gf_error {

	err := pRuntime.Validator.Struct(pInput)
	if err != nil {
		gErr := gfcore.Error__create("failed to validate HTTP input", 
			"verify__invalid_input_struct_error",
			map[string]interface{}{"input": pInput,},
			err, "glry_lib", pRuntime.RuntimeSys)
		return gErr
	}
	
	return nil
}