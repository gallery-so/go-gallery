package util

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// DefaultSearchDepth represents the maximum amount of nested maps (aka recursions) that can be searched
const DefaultSearchDepth = 5
const GinContextKey string = "GinContextKey"

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
	return json.NewDecoder(body).Decode(pInput)
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

var errDataTooLarge = errors.New("data too large")

// CopyMax will copy until a certain point and error after that point
func CopyMax(writer io.Writer, it io.Reader, max int64) error {
	if _, err := io.CopyN(writer, it, max); err != nil {
		if err != io.EOF {
			return err
		}
		return nil
	}
	extra := make([]byte, 1)
	if n, _ := io.ReadFull(it, extra); n > 0 {
		return errDataTooLarge
	}
	return nil
}

// StringToPointer simply returns a pointer to the parameter string. It's useful for taking the address of a string concatenation,
// a function that returns a string, or any other string that would otherwise need to be assigned to a variable before becoming addressable.
func StringToPointer(str string) *string {
	return &str
}

// GinContextFromContext retrieves a gin.Context previously stored in the request context via the GinContextToContext middleware,
// or panics if no gin.Context can be retrieved (since there's nothing left for the resolver to do if it can't obtain the context).
func GinContextFromContext(ctx context.Context) *gin.Context {
	// If the current context is already a gin context, return it
	if gc, ok := ctx.(*gin.Context); ok {
		return gc
	}

	// Otherwise, find the gin context that was stored via middleware
	ginContext := ctx.Value(GinContextKey)
	if ginContext == nil {
		panic("gin.Context not found in current context")
	}

	gc, ok := ginContext.(*gin.Context)
	if !ok {
		panic("gin.Context has wrong type")
	}

	return gc
}

// FindFile finds a file relative to the working directory
// by searching outer directories up to the search depth.
// Mostly for testing purposes.
func FindFile(f string, searchDepth int) (string, error) {
	if _, err := os.Stat(f); err == nil {
		return f, nil
	}

	for i := 0; i < searchDepth; i++ {
		f = filepath.Join("..", f)
		if _, err := os.Stat(f); err == nil {
			return f, nil
		}
	}

	return "", errors.New("could not find file in path")
}
