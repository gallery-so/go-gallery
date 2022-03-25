//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/mikeydub/go-gallery/graphql/plugin/gqlidgen"
	"github.com/mikeydub/go-gallery/graphql/plugin/modelgen_custom"
	"os"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}

	err = api.Generate(cfg,
		api.ReplacePlugin(modelgen_custom.New()),
		api.AddPlugin(gqlidgen.New(cfg.Model.Dir(), cfg.Model.Package)),
	)

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}
