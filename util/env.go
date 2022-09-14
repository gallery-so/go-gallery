package util

import (
	"fmt"

	"github.com/spf13/viper"
)

// MustExist panics if an environment variable is not set.
func MustExist(envVar, emptyVal string) {
	if viper.GetString(envVar) == emptyVal {
		panic(fmt.Sprintf("%s must be set", envVar))
	}
}
