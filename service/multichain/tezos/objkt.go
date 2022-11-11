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
	maxPageSize   = 500
	objktEndpoint = "https://data.objkt.com/v3/graphql"
)

type inputArgs map[string]interface{}

type objktToken struct {
}

type TezosObjktProvider struct {
	gql *graphql.Client
}

type tokensByWalletQuery struct {
}

type tokensByContractQuery struct {
	Fa struct {
		Tokens []struct {
			ArtifactURI  string
			Decimals     int
			Description  string
			DisplayURI   string
			Metadata     string
			Name         string
			Symbol       string
			ThumbnailURI string
			TokenID      tokenID
			Level        int
			Fa           struct {
				Name      string
				Contract  persist.Address
				Type      tokenStandard
				ShortName string
				Creator   struct {
					Address persist.Address
					Alias   string
				}
			}
		} `graphql:"tokens(limit: $limit, offset: $offset)"`
	} `graphql:"fa(where: {contract: {_eq: $contractID}})"`
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

func (p *TezosObjktProvider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"ownerAddress": ownerAddress})
	tezosAddress, err := toTzAddress(ownerAddress)
	if err != nil {
		return nil, nil, err
	}
	var tokens tokensByWalletQuery

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

func (p *TezosObjktProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("not implemented")
}

func (p *TezosObjktProvider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"contractAddress": contractAddress})
	tezosAddress, err := toTzAddress(contractAddress)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	var tokens tokensByContractQuery

	pageSize := maxLimit
	if maxLimit > maxPageSize {
		pageSize = maxPageSize
	}

	// TODO: Handle pagination and rate-limiting?
	if err := p.gql.Query(ctx, &tokens, inputArgs{
		"contractID": tezosAddress,
		"limit":      pageSize,
		"offset":     startOffset,
	}); err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens.Fa.Tokens))
	for _, t := range tokens.Fa.Tokens {
		token := t
		if token.Fa.Type == tokenStandardFa12 {
			continue
		}

		agnosticToken := multichain.ChainAgnosticToken{
			TokenType:       persist.TokenTypeERC1155,
			Description:     token.Description,
			Name:            token.Name,
			TokenID:         persist.TokenID(token.TokenID.toBase16String()),
			Media:           persist.Media{}, // TODO
			ContractAddress: token.Fa.Contract,
			Quantity:        "",  // TODO
			TokenMetadata:   nil, // TODO
			OwnerAddress:    "",  // TODO
			BlockNumber:     persist.BlockNumber(token.Level),
		}
	}
}

type tokensByContractQuery struct {
	Fa struct {
		Tokens []struct {
			ArtifactURI  string
			Decimals     int
			Description  string
			DisplayURI   string
			Metadata     string
			Name         string
			Symbol       string
			ThumbnailURI string
			TokenID      tokenID
			Fa           struct {
				Name      string
				Contract  persist.Address
				Type      tokenStandard
				ShortName string
				Creator   struct {
					Address persist.Address
					Alias   string
				}
			}
		} `graphql:"tokens(limit: $limit, offset: $offset, distinct_on: token_id)"`
	} `graphql:"fa(where: {contract: {_eq: $contractID}})"`
}
