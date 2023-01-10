package util

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/spf13/viper"
)

// DefaultSearchDepth represents the maximum amount of nested maps (aka recursions) that can be searched
const DefaultSearchDepth = 5
const GinContextKey string = "GinContextKey"

const (
	// KB is the number of bytes in a kilobyte
	KB = 1024
	// MB is the number of bytes in a megabyte
	MB = 1024 * KB
	// GB is the number of bytes in a gigabyte
	GB = 1024 * MB
	// TB is the number of bytes in a terabyte
	TB = 1024 * GB
	// PB is the number of bytes in a petabyte
	PB = 1024 * TB
	// EB is the number of bytes in an exabyte
	EB = 1024 * PB
)

// FileHeaderReader is a struct that wraps an io.Reader and pre-reads the first 512 bytes of the reader
// When the reader is read, the first 512 bytes are returned first, then the rest of the reader is read,
// so that the first 512 bytes are not lost
type FileHeaderReader struct {
	*bufio.Reader
	headers   []byte
	subreader io.Reader
}

// NewFileHeaderReader returns a new FileHeaderReader
func NewFileHeaderReader(reader io.Reader) *FileHeaderReader {
	return &FileHeaderReader{bufio.NewReader(reader), nil, reader}
}

// Close closes the given io.Reader if it is also a closer
func (f FileHeaderReader) Close() error {
	if closer, ok := f.subreader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Headers returns the first 512 bytes of the reader
func (f FileHeaderReader) Headers() ([]byte, error) {
	if f.headers != nil {
		return f.headers, nil
	}

	byt, err := f.Peek(512)
	if err != nil && err != io.EOF {
		return nil, err
	}

	f.headers = byt
	return f.headers, nil
}

// RemoveBOM removes the byte order mark from a byte array
func RemoveBOM(bs []byte) []byte {
	if len(bs) > 3 && bs[0] == 0xEF && bs[1] == 0xBB && bs[2] == 0xBF {
		return bs[3:]
	}
	return bs
}

// ContainsString checks whether an item exists in a slice
func ContainsString(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// ContainsAnyString checks whether a string contains any of the given substrings
func ContainsAnyString(s string, strs ...string) bool {
	for _, v := range strs {
		if strings.Contains(s, v) {
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
	if _, ok := m[key]; ok {
		return m[key]
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
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
	if _, ok := m[key]; ok {
		return m[key]
	}
	for k, v := range m {

		if strings.Contains(strings.ToLower(k), strings.ToLower(key)) {
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

// Map applies a function to each element of a slice, returning a new slice of the same length.
func Map[T, U any](xs []T, f func(T) (U, error)) ([]U, error) {
	result := make([]U, len(xs))
	for i, x := range xs {
		it, err := f(x)
		if err != nil {
			return nil, err
		}
		result[i] = it
	}
	return result, nil
}

// Dedupe removes duplicate elements from a slice, preserving the order of the remaining elements.
func Dedupe[T comparable](src []T, filterInPlace bool) []T {
	var result []T
	if filterInPlace {
		result = src[:0]
	} else {
		result = make([]T, 0, len(src))
	}
	seen := make(map[T]bool)
	for _, x := range src {
		if !seen[x] {
			result = append(result, x)
			seen[x] = true
		}
	}
	return result
}

func Contains[T comparable](s []T, str T) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func SetConditionalValue[T any](value *T, param *T, conditional *bool) {
	if value != nil {
		*param = *value
		*conditional = true
	} else {
		*conditional = false
	}
}

// StringToPointer simply returns a pointer to the parameter string. It's useful for taking the address of a string concatenation,
// a function that returns a string, or any other string that would otherwise need to be assigned to a variable before becoming addressable.
func StringToPointer(str string) *string {
	return &str
}

// StringToPointerIfNotEmpty returns a pointer to the string if it is a non-empty string
func StringToPointerIfNotEmpty(str string) *string {
	if str == "" {
		return nil
	}
	return &str
}

// FromPointer returns the value of a pointer, or the zero value of the pointer's type if the pointer is nil.
func FromPointer[T comparable](s *T) T {
	if s == nil {
		return reflect.Zero(reflect.TypeOf(s).Elem()).Interface().(T)
	}
	return *s
}

func StringersToStrings[T fmt.Stringer](stringers []T) []string {
	strings := make([]string, len(stringers))
	for i, stringer := range stringers {
		strings[i] = stringer.String()
	}
	return strings
}

// BoolToPointer returns a pointer to the parameter boolean. Useful for a boolean that would need to be assigned to a variable
// before becoming addressable.
func BoolToPointer(b bool) *bool {
	return &b
}

// IntToPointer returns a pointer to the parameter integer. Useful for an integer that would need to be assigned to a variable
// before becoming addressable.
func IntToPointer(i int) *int {
	return &i
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

	return "", fmt.Errorf("could not find file '%s' in path", f)
}

// MustFindFile panics if the file is not found up to the default search depth.
func MustFindFile(f string) string {
	f, err := FindFile(f, 5)
	if err != nil {
		panic(err)
	}
	return f
}

// InByteSizeFormat converts a number of bytes to a human-readable string
// using SI units (kB, MB, GB, TB, PB, EB, ZB, YB)
func InByteSizeFormat(bytes uint64) string {
	unit := ""
	value := float64(bytes)

	if bytes >= EB {
		unit = "EB"
		value = value / EB
	} else if bytes >= PB {
		unit = "PB"
		value = value / PB
	} else if bytes >= TB {
		unit = "TB"
		value = value / TB
	} else if bytes >= GB {
		unit = "GB"
		value = value / GB
	} else if bytes >= MB {
		unit = "MB"
		value = value / MB
	} else if bytes >= KB {
		unit = "KB"
		value = value / KB
	} else {
		unit = "B"
	}

	return fmt.Sprintf("%.2f %s", value, unit)
}

// IntToPointerSlice returns a slice to pointers of integer values.
func IntToPointerSlice(s []int) []*int {
	ret := make([]*int, len(s))
	for idx, it := range s {
		ret[idx] = IntToPointer(it)
	}
	return ret
}

// GetURIPath takes a uri in any form and returns just the path
func GetURIPath(initial string, withoutQuery bool) string {

	var path string

	path = strings.TrimSpace(initial)
	if strings.HasPrefix(initial, "http") {
		path = strings.TrimPrefix(path, "https://")
		path = strings.TrimPrefix(path, "http://")
		indexOfPath := strings.Index(path, "/")
		if indexOfPath > 0 {
			path = path[indexOfPath:]
		}
	} else if strings.HasPrefix(initial, "ipfs://") {
		path = strings.ReplaceAll(initial, "ipfs://", "")
	} else if strings.HasPrefix(initial, "arweave://") || strings.HasPrefix(initial, "ar://") {
		path = strings.ReplaceAll(initial, "arweave://", "")
		path = strings.ReplaceAll(path, "ar://", "")
	}
	path = strings.ReplaceAll(path, "ipfs/", "")
	path = strings.TrimPrefix(path, "/")
	if withoutQuery {
		path = strings.Split(path, "?")[0]
		path = strings.TrimSuffix(path, "/")
	}

	return path
}

// FindFirstFieldFromMap finds the first field in the map that contains the given field
func FindFirstFieldFromMap(it map[string]interface{}, fields ...string) interface{} {

	for _, field := range fields {
		if val := GetValueFromMapUnsafe(it, field, DefaultSearchDepth); val != nil {
			return val
		}
	}
	return nil
}

// VarNotSetTo panics if an environment variable is not set or set to `emptyVal`.
func VarNotSetTo(envVar, emptyVal string) {
	setTo := viper.GetString(envVar)
	if setTo == emptyVal || setTo == "" {
		panic(fmt.Sprintf("%s must be set", envVar))
	}
}

// InDocker returns true if the service is running as a container.
func InDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

// ResolveEnvFile finds the appropriate env file to use for the service.
func ResolveEnvFile(service string, env string) string {
	format := "app-%s-%s.yaml"
	if InDocker() {
		return fmt.Sprintf(format, "docker", service)
	}

	switch env {
	case "local":
		return fmt.Sprintf(format, "local", service)
	case "dev":
		return fmt.Sprintf(format, "dev", service)
	case "prod":
		return fmt.Sprintf(format, "prod", service)
	}

	return fmt.Sprintf("app-local-%s.yaml", service)
}

// LoadEnvFile configures the environment with the configured input file.
func LoadEnvFile(fileName string) {
	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
		return
	}

	// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
	filePath := filepath.Join("_local", fileName)
	logger.For(nil).Infof("configuring environment with settings from %s", filePath)
	path := MustFindFile(filePath)

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("error reading viper config: %s\nmake sure your _local directory is decrypted and up-to-date", err))
	}
}

func TruncateWithEllipsis(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

func IsNullOrEmpty(s sql.NullString) bool {
	return !s.Valid || s.String == ""
}
