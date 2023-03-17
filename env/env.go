package env

import (
	"context"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/spf13/viper"
)

var validators = map[string][]string{}

var v = validator.New()

func init() {
	v.RegisterValidation("required_for_env", RequiredForEnv)
}

func RegisterEnvValidation(name string, tags []string) {
	validators[name] = dedupe(append(validators[name], tags...))
}

func Get[T any](ctx context.Context, name string) T {
	for _, tag := range validators[name] {
		err := v.Var(name, tag)
		if err != nil {
			logger.For(ctx).Errorf("invalid env var: %s, tag: %s, err: %s", name, tag, err.Error())
		}
	}

	it, ok := viper.Get(name).(T)
	if !ok {
		if reflect.ValueOf(it).IsZero() {
			return *new(T)
		}
		logger.For(ctx).Errorf("invalid env var: %s, expected type: %T", name, it)
		return *new(T)
	}

	return it
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

	return spl[1] == Get[string](context.Background(), "ENV")
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
