package env

import (
	"context"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/spf13/viper"
)

// TODO cmd+f and replace "viper.GetString(" with "env.Get[string]("

var validators = map[string][]string{}

var v = validate.WithCustomValidators()

func init() {
	v.RegisterValidation("required_for_env", RequiredForEnv)
}

func RegisterEnvValidation(name string, tags []string) {
	validators[name] = util.Dedupe(append(validators[name], tags...), true)
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
