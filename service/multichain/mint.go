package multichain

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

type MintStatuser interface {
	GetIsMintingByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) (bool, error)
}

func (p *Provider) IsTokenMinting(ctx context.Context, chain persist.Chain, contractAddress persist.Address, tokenID persist.DecimalTokenID) (bool, error) {
	_, ok := p.Chains[chain].(MintStatuser)
	if !ok {
		return false, fmt.Errorf("no mint statuser for chain: %s", chain)
	}
	return false, nil
}
