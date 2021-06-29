package glry_core

import (
	"bytes"
	"encoding/json"
	"io"

	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
// VALIDATE
func Validate(pInput interface{},
	pRuntime *Runtime) *gfcore.Gf_error {

	err := pRuntime.Validator.Struct(pInput)
	if err != nil {
		gErr := gfcore.Error__create("failed to validate HTTP input",
			"verify__invalid_input_struct_error",
			map[string]interface{}{"input": pInput},
			err, "glry_core", pRuntime.RuntimeSys)
		return gErr
	}

	return nil
}

//-------------------------------------------------------------
// UNMARSHALL BODY
// input must be a pointer to a struct with json tags
func UnmarshalBody(pInput interface{}, body io.Reader, pRuntime *Runtime) *gfcore.Gf_error {
	buf := &bytes.Buffer{}

	_, err := io.Copy(buf, body)
	if err != nil {
		return gfcore.Error__create("unable to read bytes of body",
			"io_reader_error",
			map[string]interface{}{}, err, "glry_core", pRuntime.RuntimeSys)
	}

	err = json.Unmarshal(buf.Bytes(), pInput)
	if err != nil {
		return gfcore.Error__create("unable to unmarshal body into struct",
			"io_reader_error",
			map[string]interface{}{}, err, "glry_core", pRuntime.RuntimeSys)
	}
	return nil
}
