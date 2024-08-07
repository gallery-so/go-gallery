package objkt

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/shurcooL/graphql"
	"github.com/sirupsen/logrus"
)

const (
	maxPageSize   = 500
	objktEndpoint = "https://data.objkt.com/v3/graphql"
)

type inputArgs map[string]any

type attribute struct {
	Name  string
	Value string
	Type  string
	ID    int
}

type contract struct {
	Name            string
	Contract        persist.Address
	Description     string
	Creator_Address persist.Address
	Level           int
	Type            tezos.TokenStandard
	Collection_ID   string
	Logo            string
}

type token struct {
	Artifact_URI      string
	Rights            string
	Decimals          int
	Description       string
	Display_URI       string
	Metadata          string
	Name              string
	Symbol            string
	Thumbnail_URI     string
	Token_ID          tezos.TokenStandard
	Level             int
	Is_Boolean_Amount bool
	Attributes        []struct {
		Attribute attribute
	}
	Holders []holder `graphql:"holders(where: {quantity: {_gt: 0}})"`
	Fa      contract
}

type holder struct {
	Quantity       int
	Holder_Address persist.Address
}

type tokenNode struct {
	Token token
}

type tokenHolder struct {
	Held_Tokens []tokenNode `graphql:"held_tokens(limit: $limit, offset: $offset, where: {quantity: {_gt: 0}})"`
}

type tokensByWalletQuery struct {
	Holder []tokenHolder `graphql:"holder(where: {address: {_eq: $ownerAddress}}, limit: 1)"`
}

type tokensByContractQuery struct {
	Fa []struct {
		Tokens []token `graphql:"tokens(limit: $limit, offset: $offset, distinct_on: token_id, where: {holders: holder_address: {_eq: $ownerAddress}})"`
	} `graphql:"fa(where: {contract: {_eq: $contractAddress}, type: {_eq: fa2}})"`
}

type tokensByIdentifiersOwnerQuery struct {
	Token []token `graphql:"token(where: {fa: {type: {_eq: fa2}}, fa_contract: {_eq: $contractAddress}, token_id: {_eq: $tokenID}, holders: {quantity: {_gt: 0}, holder: {address: {_eq: $ownerAddress}}}})"`
}

type tokensByIdentifiersQuery struct {
	Token []token `graphql:"token(where: {fa: {type: {_eq: fa2}}, fa_contract: {_eq: $contractAddress}, token_id: {_eq: $tokenID}})"`
}

type Provider struct {
	gql *graphql.Client
}

func NewProvider() *Provider {
	return &Provider{
		gql: graphql.NewClient(objktEndpoint, http.DefaultClient),
	}
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	t, _, err := p.GetTokensByTokenIdentifiers(ctx, ti)
	if err != nil {
		return persist.TokenMetadata{}, err
	}

	if len(t) == 0 {
		return persist.TokenMetadata{}, fmt.Errorf("token not found for %s", ti)
	}

	return t[0].TokenMetadata, nil
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"ownerAddress": ownerAddress})
	tzOwnerAddress, err := tezos.ToAddress(ownerAddress)
	if err != nil {
		return nil, nil, err
	}

	pageSize := maxPageSize

	// Paginate results
	var query tokensByWalletQuery
	tokens := make([]tokenNode, 0)
	offset := 0
	for {
		if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
			"ownerAddress": graphql.String(tzOwnerAddress),
			"limit":        graphql.Int(pageSize),
			"offset":       graphql.Int(offset),
		}); err != nil {
			return nil, nil, err
		}

		// No more results
		if len(query.Holder) < 1 || len(query.Holder[0].Held_Tokens) < 1 {
			break
		}

		// Exceeded fetch size
		tokens = append(tokens, query.Holder[0].Held_Tokens...)

		offset += len(query.Holder[0].Held_Tokens)
	}

	// FA1.2 is the equivalent of ERC-20 on Tezos
	returnTokens, returnContracts := objktTokensToChainAgnostic(tokens, tzOwnerAddress)

	return returnTokens, returnContracts, nil
}

func objktTokensToChainAgnostic(tokens []tokenNode, tzOwnerAddress persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract) {
	returnTokens := make([]common.ChainAgnosticToken, 0, len(tokens))
	returnContracts := make([]common.ChainAgnosticContract, 0)
	dedupeContracts := make(map[persist.Address]common.ChainAgnosticContract)

	for _, node := range tokens {

		if node.Token.Fa.Type == tezos.TokenStandardFa12 {
			continue
		}

		metadata := createMetadata(node.Token)

		if _, ok := dedupeContracts[node.Token.Fa.Contract]; !ok {
			dedupeContracts[node.Token.Fa.Contract] = common.ChainAgnosticContract{
				Address: node.Token.Fa.Contract,
				Descriptors: common.ChainAgnosticContractDescriptors{
					Symbol:          node.Token.Symbol,
					Name:            node.Token.Fa.Name,
					Description:     node.Token.Fa.Description,
					OwnerAddress:    node.Token.Fa.Creator_Address,
					ProfileImageURL: node.Token.Fa.Logo,
				},

				LatestBlock: persist.BlockNumber(node.Token.Fa.Level),
			}
			returnContracts = append(returnContracts, dedupeContracts[node.Token.Fa.Contract])
		}

		tokenID := persist.MustTokenID(string(node.Token.Token_ID))

		agnosticToken := common.ChainAgnosticToken{
			TokenType: persist.TokenTypeERC1155,
			Descriptors: common.ChainAgnosticTokenDescriptors{
				Description: node.Token.Description,
				Name:        node.Token.Name,
			},
			TokenID: tokenID,

			ContractAddress: dedupeContracts[node.Token.Fa.Contract].Address,
			Quantity:        persist.HexString(fmt.Sprintf("%x", node.Token.Holders[0].Quantity)),
			TokenMetadata:   metadata,
			OwnerAddress:    tzOwnerAddress,
			BlockNumber:     persist.BlockNumber(node.Token.Level),
		}
		returnTokens = append(returnTokens, agnosticToken)
	}
	return returnTokens, returnContracts
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, ownerAddress persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	go func() {
		defer close(recCh)

		ctx = logger.NewContextWithFields(ctx, logrus.Fields{"ownerAddress": ownerAddress})
		tzOwnerAddress, err := tezos.ToAddress(ownerAddress)
		if err != nil {
			errCh <- err
			return
		}

		pageSize := maxPageSize

		// Paginate results
		var query tokensByWalletQuery

		offset := 0
		for {
			if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
				"ownerAddress": graphql.String(tzOwnerAddress),
				"limit":        graphql.Int(pageSize),
				"offset":       graphql.Int(offset),
			}); err != nil {
				errCh <- err
				return
			}

			// No more results
			if len(query.Holder) < 1 || len(query.Holder[0].Held_Tokens) < 1 {
				break
			}

			returnTokens, returnContracts := objktTokensToChainAgnostic(query.Holder[0].Held_Tokens, tzOwnerAddress)

			recCh <- common.ChainAgnosticTokensAndContracts{
				Tokens:    returnTokens,
				Contracts: returnContracts,
			}

			offset += len(query.Holder[0].Held_Tokens)
		}
	}()
	return recCh, errCh
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers common.ChainAgnosticIdentifiers, ownerAddress persist.Address) (common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"contractAddress": tokenIdentifiers.ContractAddress,
		"tokenID":         tokenIdentifiers.TokenID,
		"ownerAddress":    ownerAddress,
	})

	tzOwnerAddress, err := tezos.ToAddress(ownerAddress)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	tokenInDecimal, err := strconv.ParseInt(tokenIdentifiers.TokenID.String(), 16, 64)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	var query tokensByIdentifiersOwnerQuery

	if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
		"contractAddress": graphql.String(tokenIdentifiers.ContractAddress),
		"ownerAddress":    graphql.String(tzOwnerAddress),
		"tokenID":         graphql.String(strconv.Itoa(int(tokenInDecimal))),
	}); err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	if len(query.Token) < 1 {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, fmt.Errorf("no token found for token identifiers: %s", tokenIdentifiers.String())
	}

	token := query.Token[0]

	metadata := createMetadata(token)

	agnosticContract := common.ChainAgnosticContract{
		Address: token.Fa.Contract,
		Descriptors: common.ChainAgnosticContractDescriptors{
			Symbol:       token.Symbol,
			Name:         token.Fa.Name,
			Description:  token.Fa.Description,
			OwnerAddress: token.Fa.Creator_Address,
		},

		LatestBlock: persist.BlockNumber(token.Fa.Level),
	}

	tokenID := persist.MustTokenID(string(token.Token_ID))

	agnosticToken := common.ChainAgnosticToken{
		TokenType: persist.TokenTypeERC1155,
		Descriptors: common.ChainAgnosticTokenDescriptors{
			Description: token.Description,
			Name:        token.Name,
		},
		TokenID:         tokenID,
		ContractAddress: agnosticContract.Address,
		Quantity:        persist.HexString(fmt.Sprintf("%x", token.Holders[0].Quantity)),
		TokenMetadata:   metadata,
		OwnerAddress:    ownerAddress,
		BlockNumber:     persist.BlockNumber(token.Level),
	}

	return agnosticToken, agnosticContract, nil
}

func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers common.ChainAgnosticIdentifiers) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"contractAddress": tokenIdentifiers.ContractAddress,
		"tokenID":         tokenIdentifiers.TokenID,
	})

	tokenInDecimal, err := strconv.ParseInt(tokenIdentifiers.TokenID.String(), 16, 64)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}

	var query tokensByIdentifiersQuery

	if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
		"contractAddress": graphql.String(tokenIdentifiers.ContractAddress),
		"tokenID":         graphql.String(strconv.Itoa(int(tokenInDecimal))),
	}); err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}

	if len(query.Token) < 1 {
		return nil, common.ChainAgnosticContract{}, fmt.Errorf("no token found for token identifiers: %s", tokenIdentifiers.String())
	}

	firstToken := query.Token[0]

	agnosticContract := common.ChainAgnosticContract{
		Address: firstToken.Fa.Contract,
		Descriptors: common.ChainAgnosticContractDescriptors{
			Symbol:       firstToken.Symbol,
			Name:         firstToken.Name,
			Description:  firstToken.Description,
			OwnerAddress: firstToken.Fa.Creator_Address,
		},

		LatestBlock: persist.BlockNumber(firstToken.Fa.Level),
	}

	return objktHolderTokensToChainAgnostic(query.Token), agnosticContract, nil
}

func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"contractAddress": contractAddress})

	pageSize := maxPageSize
	if maxLimit > 0 && maxLimit < maxPageSize {
		pageSize = maxLimit
	}

	// Paginate results
	var query tokensByContractQuery
	tokens := make([]token, 0, maxLimit)
	for {
		if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
			"contractAddress": graphql.String(contractAddress),
			"limit":           graphql.Int(pageSize),
			"offset":          graphql.Int(offset),
		}); err != nil {
			return nil, common.ChainAgnosticContract{}, err
		}

		// No more results
		if len(query.Fa) < 1 || len(query.Fa[0].Tokens) < 1 {
			break
		}

		// Exceeded fetch size
		tokens = append(tokens, query.Fa[0].Tokens...)
		if maxLimit > 0 && len(tokens) >= maxLimit {
			break
		}

		offset += len(query.Fa[0].Tokens)
	}

	// No matching query results
	if len(tokens) < 1 {
		return nil, common.ChainAgnosticContract{}, fmt.Errorf("no tokens found for contract")
	}

	// Truncate tokens if there is a max limit
	if maxLimit > 0 && len(tokens) > maxLimit {
		tokens = tokens[:maxLimit]
	}

	agnosticContract := common.ChainAgnosticContract{
		Address: tokens[0].Fa.Contract,
		Descriptors: common.ChainAgnosticContractDescriptors{
			Symbol:       tokens[0].Symbol,
			Name:         tokens[0].Fa.Name,
			Description:  tokens[0].Fa.Description,
			OwnerAddress: tokens[0].Fa.Creator_Address,
		},

		LatestBlock: persist.BlockNumber(tokens[0].Fa.Level),
	}

	return objktHolderTokensToChainAgnostic(tokens), agnosticContract, nil
}

func objktHolderTokensToChainAgnostic(tokens []token) []common.ChainAgnosticToken {
	result := make([]common.ChainAgnosticToken, 0, len(tokens))
	for _, token := range tokens {
		tokenID := persist.MustTokenID(string(token.Token_ID))
		metadata := createMetadata(token)

		firstValidThumbnail, _ := util.FindFirst([]string{token.Thumbnail_URI, token.Display_URI, token.Artifact_URI}, func(s string) bool {
			return persist.TokenURI(s).IsRenderable()
		})

		for _, holder := range token.Holders {
			agnosticToken := common.ChainAgnosticToken{
				TokenType: persist.TokenTypeERC1155,
				Descriptors: common.ChainAgnosticTokenDescriptors{
					Description: token.Description,
					Name:        token.Name,
				},
				TokenID: tokenID,
				FallbackMedia: persist.FallbackMedia{
					ImageURL: persist.NullString(firstValidThumbnail),
				},
				ContractAddress: token.Fa.Contract,
				Quantity:        persist.HexString(fmt.Sprintf("%x", holder.Quantity)),
				TokenMetadata:   metadata,
				OwnerAddress:    holder.Holder_Address,
				BlockNumber:     persist.BlockNumber(token.Level),
			}
			result = append(result, agnosticToken)
		}
	}
	return result
}

func createMetadata(t token) persist.TokenMetadata {
	metadata := persist.TokenMetadata{}
	metadata["name"] = t.Name
	metadata["rights"] = t.Rights
	metadata["symbol"] = t.Symbol
	metadata["decimals"] = t.Decimals
	metadata["attributes"] = t.Attributes
	metadata["displayUri"] = t.Display_URI
	metadata["artifactUri"] = t.Artifact_URI
	metadata["description"] = t.Description
	metadata["thumbnailUri"] = t.Thumbnail_URI
	metadata["isBooleanAmount"] = t.Is_Boolean_Amount
	return metadata
}
