//go:build tools
// +build tools

package util

// Add gqlgen as a tool dependency. See: https://gqlgen.com/getting-started/#set-up-project
// and https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/99designs/gqlgen"
)
