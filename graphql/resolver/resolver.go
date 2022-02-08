package graphql

import (
	"context"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
)

// Add gqlgen to "go generate"
//go:generate go run github.com/99designs/gqlgen generate

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

const GinContextKey string = "GinContextKey"

type Resolver struct {
	Repos     *persist.Repositories
	EthClient *ethclient.Client
}

// GinContextFromContext retrieves a gin.Context previously stored in the request context via the GinContextToContext middleware,
// or panics if no gin.Context can be retrieved (since there's nothing left for the resolver to do if it can't obtain the context).
func GinContextFromContext(ctx context.Context) *gin.Context {
	ginContext := ctx.Value(GinContextKey)
	if ginContext == nil {
		panic("gin.Context not found in current context")
	}

	gc, ok := ginContext.(*gin.Context)
	if !ok {
		panic("gin.Context has wrong type")
	}

	return gc
}
