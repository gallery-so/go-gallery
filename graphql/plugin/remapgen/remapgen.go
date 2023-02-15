package remapgen

import (
	_ "embed"
	"path"
	"sort"
	"syscall"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"
	"github.com/99designs/gqlgen/plugin"
	"github.com/vektah/gqlparser/v2/ast"
)

//go:embed remapgen.gotpl
var template string

const outputFileName = "remapgen_gen.go"

func New(modelDir string, modelPackage string) plugin.Plugin {
	return &Plugin{
		outputFilePath: path.Join(modelDir, outputFileName),
		modelDir:       modelDir,
		modelPackage:   modelPackage,
	}
}

type Plugin struct {
	outputFilePath string
	modelDir       string
	modelPackage   string
}

var (
	_ plugin.ConfigMutator = &Plugin{}
)

func (m *Plugin) Name() string {
	return "remapgen"
}

func (m *Plugin) MutateConfig(cfg *config.Config) error {
	_ = syscall.Unlink(m.outputFilePath)

	types := make([]string, 0, len(cfg.Schema.Types))

	for typeName, def := range cfg.Schema.Types {
		// Unions and interfaces can have other types assigned to them. If there are 1 or more types in our
		// schema that can be assigned to this union or interface, create a mapping for it!
		if (def.Kind == ast.Union || def.Kind == ast.Interface) && len(cfg.Schema.GetPossibleTypes(def)) > 0 {
			types = append(types, typeName)
		}
	}

	// Make sure these are in a stable order so repeated calls to "go generate" will give the same output
	sort.Strings(types)

	return templates.Render(templates.Options{
		Template:    template,
		PackageName: m.modelPackage,
		Filename:    m.outputFilePath,
		Data: &ConfigMutateTemplateData{
			Types: types,
		},
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	})

	return nil
}

type ConfigMutateTemplateData struct {
	Types []string
}
