package env

import (
	"context"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/spf13/viper"
)

var validators = map[string][]string{}

var v = validator.New()

var validatorsMu = &sync.Mutex{}

func init() {
	v.RegisterValidation("required_for_env", RequiredForEnv)
}

func RegisterValidation(name string, tags ...string) {
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	validators[name] = dedupe(append(validators[name], tags...))
}

func Get[T any](ctx context.Context, name string) T {
	func() {
		validatorsMu.Lock()
		defer validatorsMu.Unlock()
		for _, tag := range validators[name] {
			err := v.Var(name, tag)
			if err != nil {
				logger.For(ctx).Errorf("invalid env var: %s, tag: %s, err: %s", name, tag, err.Error())
			}
		}
	}()

	if !viper.IsSet(name) {
		return *new(T)
	}

	it, ok := viper.Get(name).(T)
	if !ok {
		logger.For(ctx).Errorf("invalid env var: %s, expected type: %T", name, it)
		return *new(T)
	}

	return it
}

func GetIfExists[T any](ctx context.Context, name string) (T, bool) {
	func() {
		validatorsMu.Lock()
		defer validatorsMu.Unlock()
		for _, tag := range validators[name] {
			err := v.Var(name, tag)
			if err != nil {
				logger.For(ctx).Errorf("invalid env var: %s, tag: %s, err: %s", name, tag, err.Error())
			}
		}
	}()

	if !viper.IsSet(name) {
		return *new(T), false
	}

	it, ok := viper.Get(name).(T)
	if !ok {
		logger.For(ctx).Errorf("invalid env var: %s, expected type: %T", name, it)
		return *new(T), false
	}

	return it, true
}

func GetString(ctx context.Context, name string) string {
	return Get[string](ctx, name)
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
