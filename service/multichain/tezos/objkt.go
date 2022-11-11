package tezos

import (
	"context"
	"net/http"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/shurcooL/graphql"
	"github.com/sirupsen/logrus"
)

const (
	maxResultSize = 500
	objktEndpoint = "https://data.objkt.com/v3/graphql"
)

type objktToken struct {
}

type TezosObjktProvider struct {
	gql *graphql.Client
}

type tokensByAddressQuery struct {
}

// Objkt's API has pretty strict usage limits (120 requests/minute, and 500 results per page)
// so its best used as a fallback.
func NewObjktProvider() *TezosObjktProvider {
	return &TezosObjktProvider{
		gql: graphql.NewClient(objktEndpoint, http.DefaultClient),
	}
}

func (d *TezosObjktProvider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

func (p *TezosObjktProvider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"address": address})
	tezosAddress, err := toTzAddress(address)
	if err != nil {
		return nil, nil, err
	}
	var tokens tokensByAddressQuery

	// TODO: Test the rate-limit response, add backoff.
	if err := p.gql.Query(ctx, &tokens, map[string]interface{}{
		"limit":       limit,
		"distinct_on": "token_pk",
		"where": map[string]interface{}{
			"holder_address": map[string]interface{}{
				"_eq": tezosAddress,
			},
		},
	}); err != nil {
		logger.For(ctx).WithError(err).Error("failed to fetch tokens")
	}

	// XXX: resultTokens := []objktToken{}
	return nil, nil, nil
	// XXX: return p.tokensToAgnosticTokensAndContracts(ctx, resultTokens)
}

func (d *TezosObjktProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (d *TezosObjktProvider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}
