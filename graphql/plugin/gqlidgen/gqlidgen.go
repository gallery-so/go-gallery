package gqlidgen

import (
	"fmt"
	"path"
	"strings"
	"syscall"

	"github.com/99designs/gqlgen/codegen"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"
	"github.com/99designs/gqlgen/plugin"
)

const filename = "gqlidgen_gen.go"

func New(modelDir string, modelPackage string) plugin.Plugin {
	return &Plugin{
		filePath:     path.Join(modelDir, filename),
		modelDir:     modelDir,
		modelPackage: modelPackage,
	}
}

type Plugin struct {
	filePath     string
	modelDir     string
	modelPackage string
}

var (
	_ plugin.CodeGenerator = &Plugin{}
	_ plugin.ConfigMutator = &Plugin{}
)

func (m *Plugin) Name() string {
	return "gqlidgen"
}

func (m *Plugin) MutateConfig(cfg *config.Config) error {
	_ = syscall.Unlink(m.filePath)
	return nil
}

type nodeImplementor struct {
	Parameters     string
	Packages       []string
	Name           string
	Implementation string
}

func (m *Plugin) getNodeImplementors(objects []*codegen.Object) []nodeImplementor {
	nodeImplementors := make([]nodeImplementor, 0)

	for _, object := range objects {
		for _, implements := range object.Implements {
			if implements.Name == "Node" {
				implementor, err := m.createNodeImplementor(object)
				if err == nil {
					nodeImplementors = append(nodeImplementors, implementor)
				} else {
					fmt.Printf("error: failed to generate gqlId bindings for Node implementor '%s': %s\n", object.Name, err)
				}
			}
		}
	}

	return nodeImplementors
}

func (m *Plugin) getGqlIdArgs(object *codegen.Object, directive *codegen.Directive) ([]string, error) {
	var fieldsArg *codegen.FieldArgument

	for _, arg := range directive.Args {
		if arg.Name == "fields" {
			fieldsArg = arg
			continue
		}

		fmt.Printf("warning: generating gqlId bindings for Node implementor '%s': @gqlId parameter '%s' not recognized\n", object.Name, arg.Name)
	}

	if fieldsArg == nil {
		return nil, fmt.Errorf("@gqlId must include 'fields' argument")
	}

	var fieldNames []string

	if argsArray, ok := fieldsArg.Value.([]interface{}); ok {
		for _, arg := range argsArray {
			if argStr, ok := arg.(string); ok {
				fieldNames = append(fieldNames, argStr)
			} else {
				return nil, fmt.Errorf("@gqlId parameter 'fields' must be an array of strings")
			}
		}
	} else {
		return nil, fmt.Errorf("@gqlId parameter 'fields' must be an array of strings")
	}

	return fieldNames, nil
}

func (m *Plugin) getFieldForArg(object *codegen.Object, arg string) *codegen.Field {
	for _, field := range object.Fields {
		if arg == field.Name {
			return field
		}
	}

	return nil
}

func (m *Plugin) createNodeImplementor(object *codegen.Object) (nodeImplementor, error) {
	var args []string
	var err error
	foundDirective := false

	for _, directive := range object.Directives {
		if directive.Name == "gqlId" {
			args, err = m.getGqlIdArgs(object, directive)
			if err != nil {
				return nodeImplementor{}, err
			}
			foundDirective = true
			break
		}
	}

	if !foundDirective {
		// No explicit @gqlId directive -- see if the object has a dbid field
		field := m.getFieldForArg(object, "dbid")
		if field == nil {
			err := fmt.Errorf("could not generate default implementation (no 'dbid' field found) -- use @gqlId directive to bind fields explicitly")
			return nodeImplementor{}, err
		}

		args = []string{"dbid"}
	}

	var packages []string
	var params []string
	var typedParams []string

	for _, arg := range args {
		var packageName string
		var typeName string
		var param string

		field := m.getFieldForArg(object, arg)
		if field == nil {
			packageName, typeName = "", "string"
			param = arg
		} else {
			packageName, typeName = m.getTypeInfo(field)
			param = field.Name
		}

		if packageName != "" {
			packages = append(packages, packageName)
		}

		params = append(params, param)
		typedParams = append(typedParams, fmt.Sprintf("%s %s", param, typeName))
	}

	placeholders := "%s"
	if len(params) > 1 {
		placeholders += strings.Repeat(":%s", len(params)-1)
	}

	parameters := "(" + strings.Join(typedParams, ", ") + ")"
	implementation := "return GqlID(fmt.Sprintf(\"" + object.Name + ":" + placeholders + "\", " + strings.Join(params, ", ") + "))"

	ni := nodeImplementor{
		Name:           object.Name,
		Packages:       packages,
		Parameters:     parameters,
		Implementation: implementation,
	}

	return ni, nil
}

func isStringType(field *codegen.Field) bool {
	if field.TypeReference.IsPtr() {
		return field.TypeReference.Target.Underlying().String() == "string"
	}

	return field.TypeReference.GO.Underlying().String() == "string"
}

func (m *Plugin) getTypeInfo(field *codegen.Field) (packageName string, typeName string) {
	goType := field.TypeReference.GO.String()

	isPointer := field.TypeReference.IsPtr()
	if isPointer {
		goType = goType[1:]
	}

	// If the underlying type is a string, we can do easy conversions and add some
	// convenient type safety in our generated code. If the underlying type isn't a
	// string, the caller will have to do their own conversions.
	if !isStringType(field) {
		goType = "string"
		if isPointer {
			goType = "*string"
		}
		return "", goType
	}

	if dotIndex := strings.LastIndex(goType, "."); dotIndex == -1 {
		packageName = ""
	} else {
		packageName = goType[:dotIndex]
	}

	if slashIndex := strings.LastIndex(goType, "/"); slashIndex == -1 {
		typeName = goType
	} else {
		typeName = goType[slashIndex+1:]
	}

	if dotIndex := strings.Index(typeName, "."); dotIndex != -1 {
		if typeName[:dotIndex] == m.modelPackage {
			typeName = typeName[dotIndex+1:]
			packageName = ""
		}
	}

	if isPointer {
		typeName = "*" + typeName
	}

	return packageName, typeName
}

func (m *Plugin) GenerateCode(data *codegen.Data) error {
	pkgName := "model"

	implementors := m.getNodeImplementors(data.Objects)
	var addedImports []string
	seenImports := make(map[string]bool)

	for _, i := range implementors {
		for _, p := range i.Packages {
			if p != "" && !seenImports[p] {
				seenImports[p] = true
				addedImports = append(addedImports, p)
			}
		}

		//fmt.Printf("Name: %s\nParameters: %s\nImplementation: %s\nPackages: %v\n\n", i.Name, i.Parameters, i.Implementation, i.Packages)
	}

	return templates.Render(templates.Options{
		PackageName: pkgName,
		Filename:    m.filePath,
		Data: &TemplateData{
			Data:             data,
			AddedImports:     addedImports,
			NodeImplementors: implementors,
		},
		GeneratedHeader: true,
		Packages:        data.Config.Packages,
	})
}

type TemplateData struct {
	*codegen.Data
	AddedImports     []string
	NodeImplementors []nodeImplementor
}
