package util

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// MaxSearchDepth represents the maximum amount of nested maps (aka recursions) that can be searched
const MaxSearchDepth = 5

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

// GetValueFromMap is a function that returns the value at the first occurence of a given key in a map that potentially contains nested maps
func GetValueFromMap(m map[string]interface{}, key string, searchDepth int) interface{} {
	if searchDepth == 0 {
		return nil
	}
	for k, v := range m {
		if k == key {
			return v
		}

		if nest, ok := v.(map[string]interface{}); ok {
			if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
				return nestVal
			}
		}
		if array, ok := v.([]interface{}); ok {
			for _, arrayVal := range array {
				if nest, ok := arrayVal.(map[string]interface{}); ok {
					if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
						return nestVal
					}
				}
			}
		}
	}
	return nil
}

// GetValueFromMapUnsafe is a function that returns the value at the first occurence of a given key in a map that potentially contains nested maps
// This function is unsafe because it will also return if the specified key is a substring of any key found in the map
func GetValueFromMapUnsafe(m map[string]interface{}, key string, searchDepth int) interface{} {
	if searchDepth == 0 {
		return nil
	}
	for k, v := range m {
		if strings.Contains(k, key) {
			return v
		}

		if nest, ok := v.(map[string]interface{}); ok {
			if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
				return nestVal
			}
		}
		if array, ok := v.([]interface{}); ok {
			for _, arrayVal := range array {
				if nest, ok := arrayVal.(map[string]interface{}); ok {
					if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
						return nestVal
					}
				}
			}
		}
	}
	return nil
}
