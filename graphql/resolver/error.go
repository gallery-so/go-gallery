package graphql

import (
	"context"
	"sync"

	"github.com/mikeydub/go-gallery/util"
)

const errorsKey = "gql.errors"

type GraphQLErrorContext struct {
	errors []*GQLError
	mu     sync.Mutex
}

type GQLError struct {
	Error error
	Model interface{}
}

func (g *GraphQLErrorContext) Error(err error, gqlModel interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.errors = append(g.errors, &GQLError{err, gqlModel})
}

func (g *GraphQLErrorContext) Errors() []*GQLError {
	g.mu.Lock()
	defer g.mu.Unlock()

	errs := make([]*GQLError, len(g.errors))
	for i, err := range g.errors {
		cpy := *err
		errs[i] = &cpy

	}

	return errs
}

func addError(ctx context.Context, err error, gqlModel interface{}) {
	if gqlErrCtx := GqlErrorContextFromContext(ctx); gqlErrCtx != nil {
		gqlErrCtx.Error(err, gqlModel)
	}
}

func AddGqlErrorContextToContext(ctx context.Context, errorContext *GraphQLErrorContext) {
	gc := util.GinContextFromContext(ctx)
	gc.Set(errorsKey, errorContext)
}

func GqlErrorContextFromContext(ctx context.Context) *GraphQLErrorContext {
	gc := util.GinContextFromContext(ctx)
	if gqlErrKey, ok := gc.Get(errorsKey); ok {
		if gqlErrCtx, ok := gqlErrKey.(*GraphQLErrorContext); ok {
			return gqlErrCtx
		}
	}
	return nil
}
