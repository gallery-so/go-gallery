package graphql

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
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

func GinContextFromContext(ctx context.Context) (*gin.Context, error) {
	ginContext := ctx.Value(GinContextKey)
	if ginContext == nil {
		err := fmt.Errorf("gin.Context not found in current context")
		logrus.Errorf("could not retrieve gin.Context: %v", err)
		return nil, err
	}

	gc, ok := ginContext.(*gin.Context)
	if !ok {
		err := fmt.Errorf("gin.Context has wrong type")
		logrus.Errorf("could not retrieve gin.Context: %v", err)
		return nil, err
	}

	return gc, nil
}
