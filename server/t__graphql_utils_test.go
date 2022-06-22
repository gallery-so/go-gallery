package server

import (
	"fmt"
	"github.com/mikeydub/go-gallery/graphql/generated"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"os"
	"strings"
)

// readOperationsFromFile reads in a file of named GraphQL operations, validates them against a schema,
// and returns a mapping of operation names to operations. All GraphQL operations in the file must have names.
func readOperationsFromFile(schema *ast.Schema, filePath string) (map[string]string, error) {
	operations := map[string]string{}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	parsed, gqlErr := gqlparser.LoadQuery(schema, string(data))
	if gqlErr != nil {
		return nil, err
	}

	lastOpIndex := len(parsed.Operations) - 1

	for i, op := range parsed.Operations {
		if op.Name == "" {
			return nil, fmt.Errorf("error parsing file '%s': all GraphQL operations used in tests must have names", filePath)
		}

		position := op.Position
		opStart := op.Position.Start

		// A QueryDocument doesn't have an explicit way to get the entire source string for
		// a given operation, but we can assume that an operation extends from its own starting
		// position to the start of the next operation (or the end of the source if this is the
		// last operation)
		var opString string
		if i == lastOpIndex {
			opString = position.Src.Input[opStart:]
		} else {
			nextOp := parsed.Operations[i+1]
			opString = position.Src.Input[opStart:nextOp.Position.Start]
		}

		// The above method of getting a query string may include unnecessary leading/trailing
		// whitespace, so we'll get rid of it here to keep our operations consistent
		operations[op.Name] = strings.TrimSpace(opString)
	}

	return operations, nil
}

// loadGeneratedSchema loads the Gallery GraphQL schema via generated code
func loadGeneratedSchema() *ast.Schema {
	return generated.NewExecutableSchema(generated.Config{}).Schema()
}
