package generator

import (
	"buf.build/gen/go/sqlc/sqlc/protocolbuffers/go/protos/plugin"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

type goType struct {
	Modifiers  string
	ImportPath string
	ImportName string
	Name       string
}

type templateData struct {
	Package       string
	ImportPaths   []string
	Definitions   []dataloaderDefinition
	Subscriptions []subscription
}

type dataloaderConfig struct {
	MaxBatchSize   int
	BatchTimeout   time.Duration
	PublishResults bool
	Skip           bool
}

type dataloaderDefinition struct {
	Name             string
	MaxBatchSize     int
	BatchTimeout     int64
	PublishResults   bool
	KeyType          *goType
	ResultType       *goType
	KeyIsComparable  bool
	CanAutoCacheDBID bool
	Embeds           []embedDefinition
	IsCustomBatch    bool
	CustomBatching   *customBatchingDefinition
}

type subscription struct {
	Subscriber    string
	Target        string
	Result        string
	Field         string
	ResultIsSlice bool
	SingleKey     bool
	ManyKeys      bool
}

type autoCacheEntry struct {
	LoaderName string
	ResultType *goType
	SingleKey  bool
	ManyKeys   bool
}

func (t *goType) HasSameBaseType(other *goType) bool {
	return t.ImportPath == other.ImportPath && t.Name == other.Name
}

func (t *goType) String() string {
	if t.ImportName != "" {
		return t.Modifiers + t.ImportName + "." + t.Name
	}

	return t.Modifiers + t.Name
}

func (t *goType) StringWithoutModifiers() string {
	if t.ImportName != "" {
		return t.ImportName + "." + t.Name
	}

	return t.Name
}

func (t *goType) FullyQualifiedStringWithoutModifiers() string {
	if t.ImportPath != "" {
		return t.ImportPath + "." + t.Name
	}

	return t.Name
}

func (t *goType) IsPtr() bool {
	return strings.HasPrefix(t.Modifiers, "*")
}

func (t *goType) IsSlice() bool {
	return strings.HasPrefix(t.Modifiers, "[]")
}

func (t *goType) IsSliceOfPtrs() bool {
	return strings.HasPrefix(t.Modifiers, "[]*")
}

var partsRe = regexp.MustCompile(`^([\[\]\*]*)(.*?)(\.\w*)?$`)

func typeToGoType(t types.Type) (*goType, error) {
	return parseType(t.String())
}

var goTypeImportNames = make(map[string]string)

func parseType(str string) (*goType, error) {
	parts := partsRe.FindStringSubmatch(str)
	if len(parts) != 4 {
		return nil, fmt.Errorf("type must be in the form []*github.com/import/path.Name")
	}

	t := &goType{
		Modifiers:  parts[1],
		ImportPath: parts[2],
		Name:       strings.TrimPrefix(parts[3], "."),
	}

	if t.Name == "" {
		t.Name = t.ImportPath
		t.ImportPath = ""
	}

	if t.ImportPath != "" {
		p, ok := goTypeImportNames[t.ImportPath]
		if !ok {
			pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName}, t.ImportPath)
			if err != nil {
				return nil, err
			}

			if len(pkgs) != 1 {
				return nil, fmt.Errorf("not found")
			}

			p = pkgs[0].Name
			goTypeImportNames[t.ImportPath] = p
		}

		t.ImportName = p
	}

	return t, nil
}

func getImportPaths(defs []dataloaderDefinition) []string {
	outputImports := make([]string, 0)
	seenImports := map[string]bool{}

	addImport := func(importPath string) {
		if importPath == "" {
			return
		}

		if _, ok := seenImports[importPath]; !ok {
			seenImports[importPath] = true
			outputImports = append(outputImports, importPath)
		}
	}

	for _, def := range defs {
		addImport(def.KeyType.ImportPath)
		addImport(def.ResultType.ImportPath)
	}

	return outputImports
}

func newSubscription(entry autoCacheEntry, target dataloaderDefinition, field string) subscription {
	return subscription{
		Subscriber:    entry.LoaderName,
		Target:        target.Name,
		Result:        target.ResultType.String(),
		ResultIsSlice: target.ResultType.IsSlice(),
		Field:         field,
		SingleKey:     entry.SingleKey,
		ManyKeys:      entry.ManyKeys,
	}
}

const notFoundImplementationStr = `
type %s must implement getNotFoundError. Add this signature to notfound.go and have it return an appropriate error:

func (*%s) getNotFoundError(%s) error {
    return pgx.ErrNoRows
}
`

func generateFiles(defs []dataloaderDefinition, outputDir string) error {
	genPkg := getPackage(outputDir)
	if genPkg == nil {
		return fmt.Errorf("unable to find package info for " + outputDir)
	}

	importPaths := getImportPaths(defs)

	data := templateData{
		Package:     genPkg.Name,
		ImportPaths: importPaths,
		Definitions: defs,
	}

	dataloadersFile := filepath.Join(outputDir, "dataloaders_gen.go")
	if err := writeTemplate(dataloadersFile, dataloadersTemplate, data); err != nil {
		return err
	}

	generatedPkg := loadPackage(outputDir)

	// Check for required getNotFoundError implementations
	for _, def := range defs {
		if def.IsCustomBatch || !def.ResultType.IsSlice() {
			obj := generatedPkg.Types.Scope().Lookup(def.Name)
			if obj == nil {
				failWithErr(fmt.Errorf("%s not found in declared types of %s", def.Name, generatedPkg))
			}
			if namedType, ok := obj.Type().(*types.Named); ok {
				if !implementsGetNotFoundError(namedType, def.KeyType) {
					return fmt.Errorf(notFoundImplementationStr, def.Name, def.Name, def.KeyType.String())
				}
			} else {
				return fmt.Errorf("type %s must be a named type", def.Name)
			}
		}
	}

	// Delete the old api_gen.go file to ensure that any compiler errors present in the old file
	// won't stop us from parsing the package and writing a new one
	apiFile := filepath.Join(outputDir, "api_gen.go")
	if err := os.Remove(apiFile); err != nil && !os.IsNotExist(err) {
		failWithErr(fmt.Errorf("error deleting old api_gen.go file: %v", err))
	}

	// Map every dataloader to a list of its result types (both the top-level TResult and any sqlc.embed subfields)
	defsByResultType := make(map[string][]dataloaderDefinition)
	for _, def := range defs {
		defsByResultType[def.ResultType.FullyQualifiedStringWithoutModifiers()] = append(defsByResultType[def.ResultType.FullyQualifiedStringWithoutModifiers()], def)
		for _, embed := range def.Embeds {
			defsByResultType[embed.FieldType.FullyQualifiedStringWithoutModifiers()] = append(defsByResultType[embed.FieldType.FullyQualifiedStringWithoutModifiers()], def)
		}
	}

	data.Subscriptions = make([]subscription, 0)

	// Find all dataloaders that implement the getKeyForResult or getKeysForResult functions with
	// appropriate [TKey, TResult] values
	autoCacheEntries := getAutoCacheEntries(generatedPkg, defs)

	// For every dataloader that can cache published results, loop through all other dataloaders
	// to see which ones return results of the appropriate type. This includes checking for sqlc.embed
	// subfields that can be cached, too.
	for _, entry := range autoCacheEntries {
		targets, ok := defsByResultType[entry.ResultType.FullyQualifiedStringWithoutModifiers()]
		if !ok {
			continue
		}

		for _, target := range targets {
			// Don't subscribe loaders to their own results
			if entry.LoaderName == target.Name {
				continue
			}

			// If this entry can subscribe to the target's type, add it to the list of subscriptions
			if target.ResultType.HasSameBaseType(entry.ResultType) {
				data.Subscriptions = append(data.Subscriptions, newSubscription(entry, target, ""))
				continue
			}

			// If the entry couldn't subscribe to the target type, it must be able to subscribe to one of the target's fields
			for _, embed := range target.Embeds {
				if embed.FieldType.HasSameBaseType(entry.ResultType) {
					data.Subscriptions = append(data.Subscriptions, newSubscription(entry, target, "."+embed.FieldName))
				}
			}
		}
	}

	if err := writeTemplate(apiFile, apiTemplate, data); err != nil {
		return err
	}

	fmt.Printf("wrote %d dataloaders to:\n    %s\n    %s\n", len(defs), dataloadersFile, apiFile)

	return nil
}

func newDataloaderDefinition(name string, keyType types.Type, resultType types.Type, maxBatchSize int, batchTimeout time.Duration, publishResults bool, genPkgPath string) dataloaderDefinition {
	data := dataloaderDefinition{
		Name:            name,
		MaxBatchSize:    maxBatchSize,
		BatchTimeout:    batchTimeout.Nanoseconds(),
		PublishResults:  publishResults,
		KeyIsComparable: types.Comparable(keyType),
	}

	var err error
	data.KeyType, err = typeToGoType(keyType)
	if err != nil {
		failWithErr(fmt.Errorf("key type: %s", err.Error()))
	}
	data.ResultType, err = typeToGoType(resultType)
	if err != nil {
		failWithErr(fmt.Errorf("result type: %s", err.Error()))
	}

	// if we are inside the same package as the type we don't need an import and can refer directly to the type
	if genPkgPath == data.ResultType.ImportPath {
		data.ResultType.ImportName = ""
		data.ResultType.ImportPath = ""
	}
	if genPkgPath == data.KeyType.ImportPath {
		data.KeyType.ImportName = ""
		data.KeyType.ImportPath = ""
	}

	return data
}

func getPackage(dir string) *packages.Package {
	p, _ := packages.Load(&packages.Config{
		Dir: dir,
	}, ".")

	if len(p) != 1 {
		return nil
	}

	return p[0]
}

func writeTemplate(filepath string, tpl *template.Template, data any) error {
	var buf bytes.Buffer

	if err := tpl.Execute(&buf, data); err != nil {
		return errors.Wrap(err, "generating code")
	}

	src, err := imports.Process(filepath, buf.Bytes(), nil)
	if err != nil {
		return errors.Wrap(err, "unable to gofmt")
	}

	if err := os.WriteFile(filepath, src, 0644); err != nil {
		return errors.Wrap(err, "writing output")
	}

	return nil
}

func lcFirst(s string) string {
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func parseDataloaderConfig(comments []string) (*dataloaderConfig, error) {
	const withDataloader = "dataloader-config:"
	for _, comment := range comments {
		comment = strings.TrimSpace(comment)
		if strings.HasPrefix(comment, withDataloader) {
			return parseConfigOptions(strings.TrimSpace(comment[len(withDataloader):]))
		}
	}

	return parseConfigOptions("")
}

func parseConfigOptions(optionStr string) (*dataloaderConfig, error) {
	config := &dataloaderConfig{}

	fs := flag.NewFlagSet("dataloader generator", flag.ContinueOnError)
	fs.IntVar(&config.MaxBatchSize, "maxBatchSize", 100, "max batch size")
	fs.DurationVar(&config.BatchTimeout, "batchTimeout", 2*time.Millisecond, "batch timeout")
	fs.BoolVar(&config.PublishResults, "publishResults", true, "publish results")
	fs.BoolVar(&config.Skip, "skip", false, "don't generate a dataloader")

	// Add -- prefix to each option so we can parse it with the flag package
	args := strings.Fields(optionStr)
	for i, arg := range args {
		args[i] = "--" + arg
	}

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func parseManifest(path string) *plugin.CodeGenRequest {
	var req plugin.CodeGenRequest

	manifestFile, err := os.Open(path)
	if err != nil {
		failWithErr(fmt.Errorf("error opening manifest file at \"%s\": %v", path, err))
	}

	jsonParser := json.NewDecoder(manifestFile)
	if err = jsonParser.Decode(&req); err != nil {
		failWithErr(fmt.Errorf("error decoding manifest file at \"%s\": %v", path, err))
	}

	return &req
}

func Generate(sqlcManifestPath string, outputDir string) {
	wd, err := os.Getwd()
	if err != nil {
		failWithErr(fmt.Errorf("error getting working directory: %v", err))
	}

	outputDir = filepath.Join(wd, outputDir)
	genPkg := getPackage(outputDir)
	if genPkg == nil {
		failWithErr(fmt.Errorf("unable to find package info for " + outputDir))
	}

	sqlcManifestPath = filepath.Join(wd, sqlcManifestPath)
	sqlcManifest := parseManifest(sqlcManifestPath)
	sqlcQueries := make(map[string]*plugin.Query)
	sqlcEmbeds := make(map[string][]string)

	for _, query := range sqlcManifest.GetQueries() {
		name := query.GetName()
		sqlcQueries[name] = query
		columns := query.GetColumns()

		// Look for columns that use sqlc.embed syntax
		for _, c := range columns {
			embedTable := c.GetEmbedTable()
			if embedTable != nil {
				sqlcEmbeds[name] = append(sqlcEmbeds[name], embedTable.GetName())
			}
		}
	}

	// Load the Queries type from the sqlc generated output directory
	sourceType := filepath.Join(wd, sqlcManifest.GetSettings().GetGo().GetOut()+".Queries")
	queriesType := loadQueriesType(sourceType)

	// Get the package of the Queries type so we can compare it to other structs and see if they are
	// also sqlc-generated types
	dbTypesPkg := queriesType.Obj().Pkg()

	dataloaderDefs := make([]dataloaderDefinition, 0)

	// Loop through all methods on the Queries type
	for i := 0; i < queriesType.NumMethods(); i++ {
		method := queriesType.Method(i)
		methodName := method.Name()

		// Some methods on the Queries object (like WithTx) are not actually queries.
		// We only want to examine queries, so skip any method that doesn't have a corresponding
		// query in the sqlc manifest.
		query, ok := sqlcQueries[methodName]
		if !ok {
			continue
		}

		config, err := parseDataloaderConfig(query.GetComments())
		if err != nil {
			failWithErr(fmt.Errorf("error parsing dataloader config: %s", err.Error()))
		}

		if config != nil && config.Skip {
			continue
		}

		// Automatically generate dataloaders for all sqlc batch queries
		sqlcCmd := query.GetCmd()
		if strings.Contains(sqlcCmd, ":batch") {
			var batchInputType types.Type
			var batchOutputType types.Type

			if strings.Contains(sqlcCmd, ":batchone") {
				batchInputType, batchOutputType = getBatchOneTypes(method)
			} else {
				batchInputType, batchOutputType = getBatchManyTypes(method)
			}

			def := newDataloaderDefinition(methodName, batchInputType, batchOutputType, config.MaxBatchSize, config.BatchTimeout, config.PublishResults, genPkg.PkgPath)
			if embeds, ok := sqlcEmbeds[methodName]; ok {
				def.Embeds = getEmbedsForType(batchOutputType, embeds, dbTypesPkg)
			}

			// See whether this dataloader fits the "look up object by its DBID" pattern. If so, we can automatically
			// cache results for it.
			def.CanAutoCacheDBID = canAutoCacheDBID(batchInputType, batchOutputType, dbTypesPkg)

			dataloaderDefs = append(dataloaderDefs, def)
			continue
		}

		// We can also generate dataloaders for sqlc :many queries that include a batch_key_index column in their results
		if strings.Contains(sqlcCmd, ":many") {
			for _, col := range query.GetColumns() {
				name := col.GetName()
				if name == "batch_key_index" {
					inputType, outputType, customBatchingDef := getCustomBatchTypes(method)

					def := newDataloaderDefinition(methodName, inputType, outputType, config.MaxBatchSize, config.BatchTimeout, config.PublishResults, genPkg.PkgPath)

					// If the return type consists only of batch_key_index and one other field, we hide the batch_key_index
					// field from callers and just return the other field. When this happens, there are no sqlc.embed fields
					// to publish, so we don't need to check this type for embeds.
					if !customBatchingDef.UseOtherFieldName {
						if embeds, ok := sqlcEmbeds[methodName]; ok {
							def.Embeds = getEmbedsForType(outputType, embeds, dbTypesPkg)
						}
					}

					def.IsCustomBatch = true
					def.CustomBatching = customBatchingDef

					dataloaderDefs = append(dataloaderDefs, def)
					continue
				}
			}
		}
	}

	err = generateFiles(dataloaderDefs, outputDir)
	if err != nil {
		failWithErr(fmt.Errorf("error generating dataloaders: %v", err))
	}
}

type customBatchingDefinition struct {
	BatchKeyIndexFieldName string
	ResultFieldName        string
	QueryResultType        *goType
	LoaderResultType       *goType
	UseOtherFieldName      bool
}

type embedDefinition struct {
	FieldName string
	FieldType *goType
}

// getEmbedsForType figures out which fields of the given type use sqlc.embed.
// It currently assumes that any subfield whose type is a sqlc generated struct
// (i.e. it's from the same package as the Queries type) is an embed.
func getEmbedsForType(t types.Type, columnNames []string, dbTypesPkg *types.Package) []embedDefinition {
	embeds := make([]embedDefinition, 0)

	// If t is a slice, get its underlying type
	if sliceType, ok := t.(*types.Slice); ok {
		t = sliceType.Elem()
	}

	// Check if it's a named type
	namedType, ok := t.(*types.Named)
	if !ok {
		failWithErr(fmt.Errorf("expected named type, but got: %v", t))
	}

	// Check if the underlying type of the named type is a struct
	structType, ok := namedType.Underlying().(*types.Struct)
	if !ok {
		failWithErr(fmt.Errorf("expected named type to be a struct, but got: %v", namedType))
	}

	// Loop over the struct's fields and see whether any of them belong to the dbTypesPkg package
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if namedFieldType, ok := field.Type().(*types.Named); ok {
			if namedFieldType.Obj().Pkg().Path() == dbTypesPkg.Path() {
				if gt, err := parseType(namedFieldType.String()); err == nil {
					embeds = append(embeds, embedDefinition{
						FieldName: field.Name(),
						FieldType: gt,
					})
				} else {
					failWithErr(fmt.Errorf("error parsing type: %v", err))
				}
			}
		}
	}

	return embeds
}

// canAutoCacheDBID looks at a dataloader's input and output types to determine whether it fits the
// "look up object by its DBID" pattern. If so, we can automatically cache results for it. This function
// returns true if:
// - the inputType is a persist.DBID
// - the outputType contains a field named "ID" whose type is also a persist.DBID
func canAutoCacheDBID(inputType types.Type, outputType types.Type, dbTypesPkg *types.Package) bool {
	// Check if it's a named type
	keyType, ok := inputType.(*types.Named)
	if !ok {
		return false
	}

	// The key type must be a DBID
	if keyType.Obj().Name() != "DBID" {
		return false
	}

	resultType, ok := outputType.(*types.Named)
	if !ok {
		return false
	}

	// If the result isn't from the same package as the Queries type, we can't auto-cache it
	if resultType.Obj().Pkg().Path() != dbTypesPkg.Path() {
		return false
	}

	// Check if the underlying type of the named type is a struct
	structResultType, ok := resultType.Underlying().(*types.Struct)
	if !ok {
		return false
	}

	// Loop over the struct's fields to find an "ID" field whose type is a persist.DBID
	for i := 0; i < structResultType.NumFields(); i++ {
		field := structResultType.Field(i)
		if field.Name() != "ID" {
			continue
		}
		if namedFieldType, ok := field.Type().(*types.Named); ok {
			if namedFieldType.Obj().Name() == "DBID" {
				return true
			}
		}
	}

	return false
}

func getCustomBatchTypes(batchFunc *types.Func) (types.Type, types.Type, *customBatchingDefinition) {
	// A custom batched sqlc query should have a function with a signature like this:
	//     func (q *Queries) GetContractsByIDs(ctx context.Context, params []InputType) ([]OutputType, error)

	signature := batchFunc.Type().(*types.Signature)
	inputType := getBatchInputType(signature)

	results := signature.Results()
	if results.Len() != 2 {
		failWithErr(fmt.Errorf("expected batch query to return 2 results like ([]TResult, error), but got: %v", results))
	}

	// The output type should be a slice
	outputSlice, ok := results.At(0).Type().(*types.Slice)
	if !ok {
		failWithErr(fmt.Errorf("expected batch query to return a slice, but got: %v", results))
	}

	// Get the element type of the slice
	elemType := outputSlice.Elem()

	// The element type should be a named type
	namedType, ok := elemType.(*types.Named)
	if !ok {
		failWithErr(fmt.Errorf("expected batch query to return a slice of a named type, but got: %v", elemType))
	}

	structType, ok := namedType.Underlying().(*types.Struct)
	if !ok {
		failWithErr(fmt.Errorf("expected batch query to return a slice of a struct type, but got: %v", elemType))
	}

	// If there are only two fields in the struct, we can hide the batch_key_index field from callers and
	// just return the other field.
	numFields := structType.NumFields()
	batchKeyIndexFieldName := ""
	var otherField *types.Var
	for i := 0; i < numFields; i++ {
		field := structType.Field(i)

		// Use struct tags to match columns from the sqlc manifest to fields in the generated structs
		tag := reflect.StructTag(structType.Tag(i))
		if tag.Get("db") == "batch_key_index" {
			batchKeyIndexFieldName = field.Name()
		} else if numFields == 2 {
			otherField = field
		}
	}

	queryResultType, err := typeToGoType(namedType)
	if err != nil {
		failWithErr(fmt.Errorf("error parsing type: %v", err))
	}

	useOtherField := otherField != nil
	var loaderResultType *goType

	if useOtherField {
		loaderResultType, err = typeToGoType(otherField.Type())
		if err != nil {
			failWithErr(fmt.Errorf("error parsing type: %v", err))
		}
	} else {
		loaderResultType = queryResultType
	}

	resultFieldName := ""
	if useOtherField {
		resultFieldName = "." + otherField.Name()
	}

	var outputType types.Type
	if useOtherField {
		outputType = otherField.Type()
	} else {
		outputType = namedType
	}

	return inputType, outputType, &customBatchingDefinition{
		BatchKeyIndexFieldName: batchKeyIndexFieldName,
		ResultFieldName:        resultFieldName,
		QueryResultType:        queryResultType,
		LoaderResultType:       loaderResultType,
		UseOtherFieldName:      useOtherField,
	}
}

func getBatchInputType(signature *types.Signature) types.Type {
	// A sqlc batch query generates a function with a signature like this:
	//     func (q *Queries) GetThing(ctx context.Context, arg []InputType) *GetThingResults

	var input types.Type
	found := false
	// Iterate over input parameters and find the first one that's not context.Context
	params := signature.Params()
	for i := 0; i < params.Len(); i++ {
		v := params.At(i)
		if v.Type().String() != "context.Context" {
			input = v.Type()
			found = true
			break
		}
	}

	if !found {
		failWithErr(fmt.Errorf("couldn't find input arg for batch signature: %v", signature))
	}

	// The input type should be a slice of something
	sliceType, ok := input.(*types.Slice)
	if !ok {
		failWithErr(fmt.Errorf("expected batch input arg to be a slice. got: %v", input))
	}

	// Return the underlying type of the slice
	return sliceType.Elem()
}

func getBatchOneTypes(batchFunc *types.Func) (input types.Type, output types.Type) {
	// A :batchone query generates a function with a signature like this:
	//    - func (q *Queries) GetThing(ctx context.Context, arg []InputType) *GetThingResults
	signature := batchFunc.Type().(*types.Signature)

	return getBatchInputType(signature), getBatchOneOutputType(signature)
}

func getBatchManyTypes(batchFunc *types.Func) (input types.Type, output types.Type) {
	// A :batchone query generates a function with a signature like this:
	//    - func (q *Queries) GetThing(ctx context.Context, arg []InputType) *GetThingResults
	signature := batchFunc.Type().(*types.Signature)

	return getBatchInputType(signature), getBatchManyOutputType(signature)
}

func getBatchOneOutputType(signature *types.Signature) types.Type {
	// A :batchone query generates a BatchResults type that has a function
	// called QueryRow with a signature like this:
	//    - func (b *BatchResults) QueryRow(f func(int, OutputType, error))

	// This will return a non-slice type
	return getBatchOutputType(signature, "QueryRow")
}

func getBatchManyOutputType(signature *types.Signature) types.Type {
	// A :batchmany query generates a BatchResults type that has a function
	// called QueryRow with a signature like this:
	//    - func (b *BatchResults) Query(f func(int, []OutputType, error))

	// This will return a slice type
	return getBatchOutputType(signature, "Query")
}

func getBatchOutputType(signature *types.Signature, queryFuncName string) types.Type {
	results := signature.Results()
	if results.Len() != 1 {
		failWithErr(fmt.Errorf("expected batch query to return a single result type, but got: %v", results))
	}

	// The output type should be a pointer
	outputPtr, ok := results.At(0).Type().(*types.Pointer)
	if !ok {
		failWithErr(fmt.Errorf("expected batch query to return a pointer, but got: %v", results))
	}

	// Get the element type of the pointer
	elemType := outputPtr.Elem()

	// The element type should be a named type
	namedType, ok := elemType.(*types.Named)
	if !ok {
		failWithErr(fmt.Errorf("expected batch query to return a pointer to a named type, but got: %v", elemType))
	}

	// Get the queryFunc method on the named type
	for i := 0; i < namedType.NumMethods(); i++ {
		if namedType.Method(i).Name() != queryFuncName {
			continue
		}

		querySig, ok := namedType.Method(i).Type().(*types.Signature)
		if !ok {
			failWithErr(fmt.Errorf("query method %s on %v has no signature", queryFuncName, namedType))
		}

		// The method should have one parameter: the function f
		if querySig.Params().Len() != 1 {
			failWithErr(fmt.Errorf("query method %s on %v has %d parameters (expected 1)", queryFuncName, namedType, querySig.Params().Len()))
		}

		// Get the type of the function f
		funcType, ok := querySig.Params().At(0).Type().(*types.Signature)
		if !ok {
			failWithErr(fmt.Errorf("function parameter f of signature type %v has no signature", querySig))
		}

		// The function f should have three parameters (int, ?, error)
		if funcType.Params().Len() != 3 {
			failWithErr(fmt.Errorf("function parameter f of signature type %v has %d parameters (expected 3)", querySig, funcType.Params().Len()))
		}

		// Return the type of the second parameter of the function f
		return funcType.Params().At(1).Type()
	}

	failWithErr(fmt.Errorf("couldn't find query method %s on %v", queryFuncName, namedType))
	return nil
}

func loadQueriesType(sourceType string) *types.Named {
	sourceTypePackage, sourceTypeName := splitSourceType(sourceType)

	// Load the package so we can get type info
	pkg := loadPackage(sourceTypePackage)

	// Lookup the given source type name in the package declarations
	obj := pkg.Types.Scope().Lookup(sourceTypeName)
	if obj == nil {
		failWithErr(fmt.Errorf("%s not found in declared types of %s", sourceTypeName, pkg))
	}

	objType := obj.Type()
	if objType == nil {
		failWithErr(fmt.Errorf("%s has no type", obj))
	}

	queriesType, ok := objType.(*types.Named)
	if !ok {
		failWithErr(fmt.Errorf("%s is not a named type (expected: types.Named)", obj))
	}

	return queriesType
}

func getAutoCacheEntries(generatedPkg *packages.Package, defs []dataloaderDefinition) []autoCacheEntry {
	entries := make([]autoCacheEntry, 0)

	for _, def := range defs {
		// Lookup the given source type name in the package declarations
		obj := generatedPkg.Types.Scope().Lookup(def.Name)
		if obj == nil {
			failWithErr(fmt.Errorf("%s not found in declared types of %s", def.Name, generatedPkg))
		}

		objType := obj.Type()
		if objType == nil {
			failWithErr(fmt.Errorf("%s has no type", obj))
		}

		loader, ok := objType.(*types.Named)
		if !ok {
			failWithErr(fmt.Errorf("%s is not a named type (expected: types.Named)", obj))
		}

		cacheOne := false
		if resultType, keyType, ok := getAutoCacheTypes(loader, "getKeyForResult"); ok {
			if !resultType.HasSameBaseType(def.ResultType) {
				failWithErr(fmt.Errorf("resultType %s does not match expected: %s", resultType, def.ResultType))
			}
			if !keyType.HasSameBaseType(def.KeyType) {
				failWithErr(fmt.Errorf("keyType %s does not match expected: %s", keyType, def.KeyType))
			}
			cacheOne = true
		}

		cacheMany := false
		if resultType, keyType, ok := getAutoCacheTypes(loader, "getKeysForResult"); ok {
			if !resultType.HasSameBaseType(def.ResultType) {
				failWithErr(fmt.Errorf("resultType %s does not match expected: %s", resultType, def.ResultType))
			}
			if !keyType.HasSameBaseType(def.KeyType) {
				failWithErr(fmt.Errorf("keyType %s does not match expected: %s", keyType, def.KeyType))
			}
			cacheMany = true
		}

		if cacheOne || cacheMany {
			entries = append(entries, autoCacheEntry{
				LoaderName: def.Name,
				ResultType: def.ResultType,
				SingleKey:  cacheOne,
				ManyKeys:   cacheMany,
			})
		}
	}

	return entries
}

func getAutoCacheTypes(namedType *types.Named, funcName string) (*goType, *goType, bool) {
	for i := 0; i < namedType.NumMethods(); i++ {
		if namedType.Method(i).Name() != funcName {
			continue
		}

		autoCacheSig, ok := namedType.Method(i).Type().(*types.Signature)
		if !ok {
			failWithErr(fmt.Errorf("method %s on %v has no signature", funcName, namedType))
		}

		// The method should have one parameter: the value type for the dataloader
		if autoCacheSig.Params().Len() != 1 {
			failWithErr(fmt.Errorf("method %s on %v has %d parameters (expected 1)", funcName, namedType, autoCacheSig.Params().Len()))
		}

		// Get the type of the parameter
		paramType, ok := autoCacheSig.Params().At(0).Type().(*types.Named)
		if !ok {
			failWithErr(fmt.Errorf("input parameter of signature type %v must be a named type", autoCacheSig))
		}

		// The method should have one result: the key type for the dataloader (or a slice of the key type)
		if autoCacheSig.Results().Len() != 1 {
			failWithErr(fmt.Errorf("method %s on %v has %d results (expected 1)", funcName, namedType, autoCacheSig.Results().Len()))
		}

		resultType := autoCacheSig.Results().At(0).Type()

		paramGoType, err := typeToGoType(paramType)
		if err != nil {
			failWithErr(fmt.Errorf("error converting type to go type: %v", err))
		}

		resultGoType, err := typeToGoType(resultType)
		if err != nil {
			failWithErr(fmt.Errorf("error converting type to go type: %v", err))
		}

		return paramGoType, resultGoType, true
	}

	return nil, nil, false
}

var errorType = types.Universe.Lookup("error").Type()

func implementsGetNotFoundError(namedType *types.Named, expectedParamType *goType) bool {
	const funcName = "getNotFoundError"
	for i := 0; i < namedType.NumMethods(); i++ {
		if namedType.Method(i).Name() != funcName {
			continue
		}

		sig, ok := namedType.Method(i).Type().(*types.Signature)
		if !ok {
			failWithErr(fmt.Errorf("method %s on %v has no signature", funcName, namedType))
		}

		// The method should have one parameter: the key type for the dataloader
		if sig.Params().Len() != 1 {
			failWithErr(fmt.Errorf("method %s on %v has %d parameters (expected 1)", funcName, namedType, sig.Params().Len()))
		}

		// Get the type of the parameter
		paramType := sig.Params().At(0).Type()

		// The method should have one result: an error type
		if sig.Results().Len() != 1 {
			failWithErr(fmt.Errorf("method %s on %v has %d results (expected 1)", funcName, namedType, sig.Results().Len()))
		}

		resultType := sig.Results().At(0).Type()

		paramGoType, err := typeToGoType(paramType)
		if err != nil {
			failWithErr(fmt.Errorf("error converting type to go type: %v", err))
		}

		if !types.Identical(resultType, errorType) || *paramGoType != *expectedParamType {
			return false
		}

		return true
	}

	return false
}

func loadPackage(path string) *packages.Package {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedImports | packages.NeedSyntax | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, path)
	if err != nil {
		failWithErr(fmt.Errorf("couldn't load package for inspection: %v", err))
	}
	if packages.PrintErrors(pkgs) > 0 {
		failWithErr(fmt.Errorf("couldn't load package for inspection"))
	}

	return pkgs[0]
}

func splitSourceType(sourceType string) (string, string) {
	idx := strings.LastIndexByte(sourceType, '.')
	if idx == -1 {
		failWithErr(fmt.Errorf(`couldn't find type: "%s". expected qualified type as "pkg/path.Type"`, sourceType))
	}
	sourceTypePackage := sourceType[0:idx]
	sourceTypeName := sourceType[idx+1:]
	return sourceTypePackage, sourceTypeName
}

func failWithErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "generating dataloaders failed: %v\n", err)
		os.Exit(1)
	}
}
