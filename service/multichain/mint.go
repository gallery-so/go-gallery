package multichain

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/persist"
)

type MintStatuser interface {
	GetMintingStatusByTokenIdentifiers(context.Context, ChainAgnosticIdentifiers) (bool, persist.Currency, float64, error)
}

func (p *Provider) GetMintingStatusByTokenIdentifiers(ctx context.Context, chain persist.Chain, contractAddress persist.Address, tokenID persist.DecimalTokenID) (bool, persist.Currency, float64, error) {
	f, ok := p.Chains[chain].(MintStatuser)
	if !ok {
		return false, "", 0, fmt.Errorf("no mint statuser for chain: %s", chain)
	}
	return f.GetMintingStatusByTokenIdentifiers(ctx, ChainAgnosticIdentifiers{contractAddress, tokenID.ToHexTokenID()})
}
