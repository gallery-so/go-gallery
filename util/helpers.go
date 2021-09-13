package util

import (
	"bytes"
	"encoding/json"
	"io"
)

// Contains checks whether an item exists in a slice
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// UnmarshallBody takes a request body and unmarshals it into the given struct
// input must be a pointer to a struct with json tags
func UnmarshallBody(pInput interface{}, body io.Reader) error {
	buf := &bytes.Buffer{}

	if _, err := io.Copy(buf, body); err != nil {
		return err
	}

	if err := json.Unmarshal(buf.Bytes(), pInput); err != nil {
		return err
	}
	return nil
}
