package glry_core

import (
	"bytes"
	"encoding/json"
	"io"
)

//-------------------------------------------------------------
// VALIDATE
func Validate(pInput interface{},
	pRuntime *Runtime) error {

	err := pRuntime.Validator.Struct(pInput)
	if err != nil {
		return err
	}

	return nil
}

//-------------------------------------------------------------
// UNMARSHALL BODY
// input must be a pointer to a struct with json tags
func UnmarshalBody(pInput interface{}, body io.Reader, pRuntime *Runtime) error {
	buf := &bytes.Buffer{}

	if _, err := io.Copy(buf, body); err != nil {
		return err
	}

	if err := json.Unmarshal(buf.Bytes(), pInput); err != nil {
		return err
	}
	return nil
}
