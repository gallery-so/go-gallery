package cursorgen

import (
	"path"
	"syscall"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin"
)

const outputFileName = "cursor_gen.go"
const configMutatorTemplate = "config_mutator.gotpl"
const codeGeneratorTemplate = "code_generator.gotpl"
const bindingMethodPrefix = "GetCursorField_"

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

func (m *Plugin) Name() string {
	return "cursorgen"
}

func (m *Plugin) MutateConfig(cfg *config.Config) error {
	_ = syscall.Unlink(m.outputFilePath)

	implementors := make([]string, 0)

	for typeName, interfaces := range cfg.Schema.Implements {
		for _, i := range interfaces {
			if i.Name == "Edge" {
				implementors = append(implementors, typeName)
				break
			}
		}
	}

	if len(implementors) == 0 {
		return nil
	}

	return nil

	templateStr, err := readTemplateFile(configMutatorTemplate)
	if err != nil {
		return err
	}
}
