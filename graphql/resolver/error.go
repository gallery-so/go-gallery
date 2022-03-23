package graphql

import (
	"context"
	"sync"

	"github.com/mikeydub/go-gallery/util"
)

const GraphQLErrorsKey = "gql.errors"

type GraphQLErrorContext struct {
	errors []MappedError
	mu     sync.Mutex
}

type MappedError struct {
	Error    error
	GqlModel interface{}
}

func (g *GraphQLErrorContext) Error(err error, gqlModel interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.errors = append(g.errors, MappedError{err, gqlModel})
}

func (g *GraphQLErrorContext) Errors() []MappedError {
	return g.errors
}

func addError(ctx context.Context, err error, gqlModel interface{}) {
	if gqlErrCtx := GqlErrorContextFromContext(ctx); gqlErrCtx != nil {
		gqlErrCtx.Error(err, gqlModel)
	}
}

func GqlErrorContextFromContext(ctx context.Context) *GraphQLErrorContext {
	gc := util.GinContextFromContext(ctx)
	if gqlErrKey, ok := gc.Get(GraphQLErrorsKey); ok {
		if gqlErrs, ok := gqlErrKey.(*GraphQLErrorContext); ok {
			return gqlErrs
		}
	}
	return nil
}
