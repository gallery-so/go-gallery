package runtime

import (
	"bytes"
	"encoding/json"
	"io"
)

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
