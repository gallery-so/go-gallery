package gqlidgen

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/99designs/gqlgen/codegen"
	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"
	"github.com/99designs/gqlgen/plugin"
)

const outputFileName = "gqlidgen_gen.go"
const configMutatorTemplate = "config_mutator.gotpl"
const codeGeneratorTemplate = "code_generator.gotpl"
const bindingMethodPrefix = "GetGqlIDField_"

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
	_ plugin.CodeGenerator = &Plugin{}
	_ plugin.ConfigMutator = &Plugin{}
)

func (m *Plugin) Name() string {
	return "gqlidgen"
}

// MutateConfig will generate stub ID() implementations for any type in the schema that
// implements the Node interface. This is necessary because gqlgen needs to see that
// an ID() method exists in order to bind to it, but that binding step happens before
// gqlgen invokes code generation callbacks, so the real ID() implementations we
// generate in GenerateCode() below won't exist yet.
func (m *Plugin) MutateConfig(cfg *config.Config) error {
	_ = syscall.Unlink(m.outputFilePath)

	implementors := make([]string, 0)

	for typeName, interfaces := range cfg.Schema.Implements {
		for _, i := range interfaces {
			if i.Name == "Node" {
				implementors = append(implementors, typeName)
				break
			}
		}
	}

	if len(implementors) == 0 {
		return nil
	}

	templateStr, err := readTemplateFile(configMutatorTemplate)
	if err != nil {
		return err
	}

	return templates.Render(templates.Options{
		PackageName: m.modelPackage,
		Template:    templateStr,
		Filename:    m.outputFilePath,
		Data: &ConfigMutateTemplateData{
			NodeImplementors: implementors,
		},
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	})
}

type nodeImplementor struct {
	Types                   []string
	TypeIsPointer           []bool
	Args                    []string
	Packages                []string
	Name                    string
	Implementation          string
	HasBindingMethods       bool
	BindingMethodSignatures []string
}

func getNodeImplementors(objects []*codegen.Object, modelPackage string) []nodeImplementor {
	nodeImplementors := make([]nodeImplementor, 0)

	for _, object := range objects {
		for _, implements := range object.Implements {
			if implements.Name == "Node" {
				implementor, err := createNodeImplementor(object, modelPackage)
				if err == nil {
					nodeImplementors = append(nodeImplementors, implementor)
				} else {
					fmt.Printf("error: failed to generate goGqlId bindings for Node implementor '%s': %s\n", object.Name, err)
				}
			}
		}
	}

	return nodeImplementors
}

func getGqlIdArgs(object *codegen.Object, directive *codegen.Directive) ([]string, error) {
	var fieldsArg *codegen.FieldArgument

	for _, arg := range directive.Args {
		if arg.Name == "fields" {
			fieldsArg = arg
			continue
		}

		fmt.Printf("warning: generating goGqlId bindings for Node implementor '%s': @goGqlId parameter '%s' not recognized\n", object.Name, arg.Name)
	}

	if fieldsArg == nil {
		return nil, fmt.Errorf("@goGqlId must include 'fields' argument")
	}

	var fieldNames []string

	if argsArray, ok := fieldsArg.Value.([]interface{}); ok {
		for _, arg := range argsArray {
			if argStr, ok := arg.(string); ok {
				fieldNames = append(fieldNames, argStr)
			} else {
				return nil, fmt.Errorf("@goGqlId parameter 'fields' must be an array of strings")
			}
		}
	} else {
		return nil, fmt.Errorf("@goGqlId parameter 'fields' must be an array of strings")
	}

	uniqueMap := make(map[string]bool)

	for _, fieldName := range fieldNames {
		lower := strings.ToLower(fieldName)
		if uniqueMap[lower] {
			return nil, fmt.Errorf("'fields' arguments must be unique (duplicate entry: '%s')", fieldName)
		}
		uniqueMap[lower] = true
	}

	return fieldNames, nil
}

func getFieldForArg(object *codegen.Object, arg string) *codegen.Field {
	lowerArg := strings.ToLower(arg)
	for _, field := range object.Fields {
		if lowerArg == strings.ToLower(field.Name) {
			return field
		}
	}

	return nil
}

func createNodeImplementor(object *codegen.Object, modelPackage string) (nodeImplementor, error) {
	var args []string
	var err error
	foundDirective := false

	for _, directive := range object.Directives {
		if directive.Name == "goGqlId" {
			args, err = getGqlIdArgs(object, directive)
			if err != nil {
				return nodeImplementor{}, err
			}
			foundDirective = true
			break
		}
	}

	if !foundDirective {
		// No explicit @goGqlId directive -- see if the object has a dbid field
		field := getFieldForArg(object, "dbid")
		if field == nil {
			err := fmt.Errorf("could not generate default implementation (no 'dbid' field found) -- use @goGqlId directive to bind fields explicitly")
			return nodeImplementor{}, err
		}

		args = []string{"dbid"}
	}

	var packages []string
	var types []string
	var typeIsPointer []bool
	var requiresMethod []bool

	for _, arg := range args {
		if strings.ToLower(arg) == "id" {
			err := fmt.Errorf("@goGqlId directive may not reference 'id' field because this would cause an infite loop -- @goGqlId's purpose is to generate an implementation for 'id'")
			return nodeImplementor{}, err
		}

		var packageName string
		var typeName string
		var isPointer bool
		var argRequiresMethod bool

		field := getFieldForArg(object, arg)

		if field == nil || !isStringType(field) {
			packageName = ""
			typeName = "string"
			isPointer = false
			argRequiresMethod = true
		} else {
			packageName, typeName, isPointer = getTypeInfo(field, modelPackage)
			argRequiresMethod = false
		}

		if packageName != "" {
			packages = append(packages, packageName)
		}

		types = append(types, typeName)
		typeIsPointer = append(typeIsPointer, isPointer)
		requiresMethod = append(requiresMethod, argRequiresMethod)
	}

	var bindingMethodSignatures []string

	for i, b := range requiresMethod {
		if !b {
			continue
		}

		signature := fmt.Sprintf("func (r *%s) %s%s() string", object.Name, bindingMethodPrefix, templates.ToGo(args[i]))
		bindingMethodSignatures = append(bindingMethodSignatures, signature)
	}

	ni := nodeImplementor{
		Name:                    object.Name,
		Packages:                packages,
		Types:                   types,
		TypeIsPointer:           typeIsPointer,
		Args:                    args,
		Implementation:          getIdImplementation(object.Name, args, typeIsPointer, requiresMethod),
		HasBindingMethods:       len(bindingMethodSignatures) > 0,
		BindingMethodSignatures: bindingMethodSignatures,
	}

	return ni, nil
}

func getIdImplementation(objectName string, args []string, typeIsPointer []bool, requiresMethod []bool) string {
	placeholders := "%s"
	if len(args) > 1 {
		placeholders += strings.Repeat(":%s", len(args)-1)
	}

	implementationArgs := make([]string, len(args))

	for i, arg := range args {
		goArg := templates.ToGo(arg)
		if requiresMethod[i] {
			implementationArgs[i] = "r." + bindingMethodPrefix + goArg + "()"
		} else {
			implementationArgs[i] = "r." + goArg
		}

		if typeIsPointer[i] {
			implementationArgs[i] = "*" + implementationArgs[i]
		}
	}

	return "fmt.Sprintf(\"" + objectName + ":" + placeholders + "\", " + strings.Join(implementationArgs, ", ") + ")"
}

func isStringType(field *codegen.Field) bool {
	if field.TypeReference.IsPtr() {
		return field.TypeReference.Target.Underlying().String() == "string"
	}

	return field.TypeReference.GO.Underlying().String() == "string"
}

func getTypeInfo(field *codegen.Field, modelPackage string) (packageName string, typeName string, isPointer bool) {
	goType := field.TypeReference.GO.String()
	isPointer = field.TypeReference.IsPtr()

	if isPointer {
		goType = goType[1:]
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
		if typeName[:dotIndex] == modelPackage {
			typeName = typeName[dotIndex+1:]
			packageName = ""
		}
	}

	return packageName, typeName, isPointer
}

func (m *Plugin) GenerateCode(data *codegen.Data) error {
	implementors := getNodeImplementors(data.Objects, m.modelPackage)

	if len(implementors) == 0 {
		return nil
	}

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

	templateStr, err := readTemplateFile(codeGeneratorTemplate)
	if err != nil {
		return err
	}

	return templates.Render(templates.Options{
		PackageName: m.modelPackage,
		Template:    templateStr,
		Filename:    m.outputFilePath,
		Data: &CodegenTemplateData{
			Data:             data,
			AddedImports:     addedImports,
			NodeImplementors: implementors,
		},
		GeneratedHeader: true,
		Packages:        data.Config.Packages,
	})
}

type ConfigMutateTemplateData struct {
	NodeImplementors []string
}

type CodegenTemplateData struct {
	*codegen.Data
	AddedImports     []string
	NodeImplementors []nodeImplementor
}

func readTemplateFile(filename string) (string, error) {
	_, callerFile, _, _ := runtime.Caller(1)
	rootDir := filepath.Dir(callerFile)
	templatePath := filepath.Join(rootDir, filename)

	data, err := ioutil.ReadFile(templatePath)
	return string(data), err
}
