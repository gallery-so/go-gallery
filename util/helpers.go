package util

import (
	"bufio"
	"bytes"
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
	"github.com/jackc/pgtype"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/spf13/viper"
	"go.mozilla.org/sops/v3/decrypt"
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

type MultiErr []error

func (m MultiErr) Error() string {
	var errStr string
	for _, err := range m {
		if err != nil {
			errStr += "(" + err.Error() + "),"
		}
	}
	return fmt.Sprint("Multiple errors: [", errStr, "]")
}

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
func UnmarshallBody(pInput any, body io.Reader) error {
	return json.NewDecoder(body).Decode(pInput)
}

// GetValueFromMap is a function that returns the value at the first occurence of a given key in a map that potentially contains nested maps
func GetValueFromMap(m map[string]any, key string, searchDepth int) any {
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

		if nest, ok := v.(map[string]any); ok {
			if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
				return nestVal
			}
		}
		if array, ok := v.([]any); ok {
			for _, arrayVal := range array {
				if nest, ok := arrayVal.(map[string]any); ok {
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
func GetValueFromMapUnsafe(m map[string]any, key string, searchDepth int) any {
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

		if nest, ok := v.(map[string]any); ok {
			if nestVal := GetValueFromMap(nest, key, searchDepth-1); nestVal != nil {
				return nestVal
			}
		}
		if array, ok := v.([]any); ok {
			for _, arrayVal := range array {
				if nest, ok := arrayVal.(map[string]any); ok {
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

func MapKeys[T comparable, V any](m map[T]V) []T {
	result := make([]T, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func MapValues[T comparable, V any](m map[T]V) []V {
	result := make([]V, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func AllEqual[T comparable](xs []T) bool {
	if len(xs) == 0 {
		return true
	}
	for _, x := range xs {
		if x != xs[0] {
			return false
		}
	}
	return true
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

// Difference will take in 2 arrays and return the elements that exist in the second array but are not in the first
func Difference[T comparable](old []T, new []T) []T {
	var added []T
	for _, v := range new {
		if !Contains(old, v) {
			added = append(added, v)
		}
	}
	return added
}

func SetConditionalValue[T any](value *T, param *T, conditional *bool) {
	if value != nil {
		*param = *value
		*conditional = true
	} else {
		*conditional = false
	}
}

func FindFirst[T any](s []T, f func(T) bool) (T, bool) {
	for _, v := range s {
		if f(v) {
			return v, true
		}
	}
	return *new(T), false
}

func MapFindOrNil[K comparable, T any](s map[K]T, key K) *T {
	if val, ok := s[key]; ok {
		return &val
	}
	return nil
}

func Filter[T any](s []T, f func(T) bool, filterInPlace bool) []T {
	var r []T
	if filterInPlace {
		r = s[:0]
	} else {
		r = make([]T, 0, len(s))
	}
	for _, v := range s {
		if f(v) {
			r = append(r, v)
		}
	}
	return r
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

func IsEmpty[T any](s *T) bool {
	return s == nil || any(*s) == reflect.Zero(reflect.TypeOf(s).Elem()).Interface()
}

func ToPointer[T any](s T) *T {
	return &s
}

func ToPointerSlice[T any](s []T) []*T {
	result := make([]*T, len(s))
	for i, v := range s {
		c := v
		result[i] = &c
	}
	return result
}

func FromPointerSlice[T any](s []*T) []T {
	result := make([]T, len(s))
	for i, v := range s {
		c := v
		result[i] = *c
	}
	return result
}

func StringersToStrings[T fmt.Stringer](stringers []T) []string {
	strings := make([]string, len(stringers))
	for i, stringer := range stringers {
		strings[i] = stringer.String()
	}
	return strings
}

// MustGetGinContext retrieves a gin.Context previously stored in the request context via the GinContextToContext
// middleware, or panics if no gin.Context is found.
func MustGetGinContext(ctx context.Context) *gin.Context {
	gc := GetGinContext(ctx)
	if gc == nil {
		panic("gin.Context not found in specified context")
	}

	return gc
}

// GetGinContext retrieves a gin.Context previously stored in the request context via the GinContextToContext
// middleware, or nil if no gin.Context is found.
func GetGinContext(ctx context.Context) *gin.Context {
	// If the current context is already a gin context, return it
	if gc, ok := ctx.(*gin.Context); ok {
		return gc
	}

	// Otherwise, find the gin context that was stored via middleware
	ginContext := ctx.Value(GinContextKey)
	if ginContext == nil {
		return nil
	}

	gc, ok := ginContext.(*gin.Context)
	if !ok {
		logger.For(ctx).Error("gin.Context has wrong type")
		return nil
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

// MustFindFile panics if the file is not found up to the default search depth.
func MustFindFileOrError(f string) (string, error) {
	f, err := FindFile(f, 5)
	if err != nil {
		return "", err
	}
	return f, nil
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
func FindFirstFieldFromMap(it map[string]any, fields ...string) any {

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

// LoadEncryptedServiceKey loads an encrypted service key JSON file from disk
func LoadEncryptedServiceKey(filePath string) []byte {
	path := MustFindFile(filePath)

	serviceKey, err := decrypt.File(path, "json")
	if err != nil {
		panic(fmt.Sprintf("error decrypting service key: %s\n", err))
	}

	return serviceKey
}

// LoadEncryptedServiceKeyOrError loads an encrypted service key JSON file from disk or errors
func LoadEncryptedServiceKeyOrError(filePath string) ([]byte, error) {
	path, err := MustFindFileOrError(filePath)
	if err != nil {
		return nil, err
	}

	serviceKey, err := decrypt.File(path, "json")
	if err != nil {
		return nil, fmt.Errorf("error decrypting service key: %s\n", err)
	}

	return serviceKey, nil
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
	if env != "local" && env != "dev" && env != "prod" {
		env = "local"
	}

	secretsDir := filepath.Join("secrets", env, "local")

	format := "app-%s-%s.yaml"
	if InDocker() {
		return filepath.Join(secretsDir, fmt.Sprintf(format, "docker", service))
	}

	return filepath.Join(secretsDir, fmt.Sprintf(format, env, service))
}

// LoadEncryptedEnvFile configures the environment with the configured input file.
func LoadEncryptedEnvFile(filePath string) {
	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
		return
	}

	// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
	logger.For(nil).Infof("configuring environment with settings from %s", filePath)
	path := MustFindFile(filePath)

	config, err := decrypt.File(path, "yaml")
	if err != nil {
		panic(fmt.Sprintf("error decrypting config file: %s\n", err))
	}

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBuffer(config)); err != nil {
		panic(fmt.Sprintf("error reading viper config: %s\n", err))
	}
}

// LoadEnvFile configures the environment with the configured input file.
func LoadEnvFile(filePath string) {
	if viper.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
		return
	}

	// Tests can run from directories deeper in the source tree, so we need to search parent directories to find this config file
	logger.For(nil).Infof("configuring environment with settings from %s", filePath)
	path := MustFindFile(filePath)

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("error reading viper config: %s\n", err))
	}
}

func TruncateWithEllipsis(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

func SqlStringIsNullOrEmpty(s sql.NullString) bool {
	return !s.Valid || s.String == ""
}

func ToNullString(s string, emptyIsNull bool) sql.NullString {
	if emptyIsNull && s == "" {
		return sql.NullString{String: "", Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func ToPGJSONB[T any](v T) (pgtype.JSONB, error) {
	marshalled, err := json.Marshal(v)
	if err != nil {
		return pgtype.JSONB{}, err
	}
	return pgtype.JSONB{Bytes: marshalled, Status: pgtype.Present}, nil
}

func GetOptionalValue[T any](optional *T, fallback T) T {
	if optional != nil {
		return *optional
	}

	return fallback
}
