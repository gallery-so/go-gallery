package tezos

import (
	"context"

	"github.com/machinebox/graphql"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

const maxResultSize = 500

type objktToken struct {
}

type TezosObjktProvider struct {
	gql *graphql.Client
}

func NewObjktProvider() *TezosObjktProvider {
	return &TezosObjktProvider{}
}

func (p *TezosObjktProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tezosAddress, err := toTzAddress(address)
	if err != nil {
		return nil, nil, err
	}

	resultTokens := []objktToken{}
}
