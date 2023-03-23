package env

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

var validators = map[string][]string{}

var v = validator.New()

var mu = &sync.Mutex{}

func init() {
	v.RegisterValidation("required_for_env", RequiredForEnv)
}

func RegisterValidation(name string, tags ...string) {
	mu.Lock()
	defer mu.Unlock()
	validators[name] = dedupe(append(validators[name], tags...))
}

func GetBool(name string) bool                         { return get(name, viper.GetBool) }
func GetDuration(name string) time.Duration            { return get(name, viper.GetDuration) }
func GetFloat64(name string) float64                   { return get(name, viper.GetFloat64) }
func GetInt(name string) int                           { return get(name, viper.GetInt) }
func GetInt64(name string) int64                       { return get(name, viper.GetInt64) }
func GetSizeInBytes(name string) uint                  { return get(name, viper.GetSizeInBytes) }
func GetString(name string) string                     { return get(name, viper.GetString) }
func GetStringMap(name string) map[string]interface{}  { return get(name, viper.GetStringMap) }
func GetStringMapString(name string) map[string]string { return get(name, viper.GetStringMapString) }
func GetStringSlice(name string) []string              { return get(name, viper.GetStringSlice) }
func GetTime(name string) time.Time                    { return get(name, viper.GetTime) }

func GetStringMapStringSlice(name string) map[string][]string {
	return get(name, viper.GetStringMapStringSlice)
}

func get[T any](name string, fetchFunc func(string) T) T {
	mu.Lock()
	defer mu.Unlock()
	val := fetchFunc(name)

	for _, tag := range validators[name] {
		if err := v.Var(val, tag); err != nil {
			panic(fmt.Errorf("invalid env var: %s failed when validating tag %s", name, tag))
		}
	}

	return val
}

var RequiredForEnv validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if s == "" {
		return false
	}

	spl := strings.Split(s, "=")
	if len(spl) != 2 {
		return false
	}

	return spl[1] == GetString("ENV")
}

func dedupe(src []string) []string {
	result := src[:0]

	seen := make(map[string]bool)
	for _, x := range src {
		if !seen[x] {
			result = append(result, x)
			seen[x] = true
		}
	}
	return result
}
