package graphql

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/persist"
)

// Add gqlgen to "go generate"
//go:generate go run generate.go

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	Repos     *persist.Repositories
	EthClient *ethclient.Client
}
