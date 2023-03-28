package tezos

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/shurcooL/graphql"
	"github.com/sirupsen/logrus"
)

const (
	maxPageSize   = 500
	objktEndpoint = "https://data.objkt.com/v3/graphql"
)

type inputArgs map[string]interface{}

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
	Type            tokenStandard
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
	Token_ID          tokenID
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

type tokensByIdentifiersQuery struct {
	Token []token `graphql:"token(where: {fa: {type: {_eq: fa2}}, fa_contract: {_eq: $contractAddress}, token_id: {_eq: $tokenID}, holders: {quantity: {_gt: 0}, holder: {address: {_eq: $ownerAddress}}}})"`
}

// Objkt's API has pretty strict usage limits (120 requests/minute, and 500 results per page)
// so its best used as a fallback.
type TezosObjktProvider struct {
	gql            *graphql.Client
	ipfsGatewayURL string
}

func NewObjktProvider(ipfsGatewayURL string) *TezosObjktProvider {
	return &TezosObjktProvider{
		gql:            graphql.NewClient(objktEndpoint, http.DefaultClient),
		ipfsGatewayURL: ipfsGatewayURL,
	}
}

func (p *TezosObjktProvider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

func (p *TezosObjktProvider) RefreshToken(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, owner persist.Address) error {
	return nil
}

func (p *TezosObjktProvider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address, maxLimit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"ownerAddress": ownerAddress})
	tzOwnerAddress, err := toTzAddress(ownerAddress)
	if err != nil {
		return nil, nil, err
	}

	pageSize := maxPageSize
	if maxLimit > 0 && maxLimit < maxPageSize {
		pageSize = maxLimit
	}

	// Paginate results
	var query tokensByWalletQuery
	tokens := make([]tokenNode, 0)
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
		if maxLimit > 0 && len(tokens) >= maxLimit {
			break
		}

		offset += len(query.Holder[0].Held_Tokens)
	}

	// Truncate tokens if there is a max limit
	if maxLimit > 0 && len(tokens) > maxLimit {
		tokens = tokens[:maxLimit]
	}

	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	returnContracts := make([]multichain.ChainAgnosticContract, 0)
	dedupeContracts := make(map[persist.Address]multichain.ChainAgnosticContract)

	for _, node := range tokens {
		// FA1.2 is the equivalent of ERC-20 on Tezos
		if node.Token.Fa.Type == tokenStandardFa12 {
			continue
		}

		metadata := createMetadata(node.Token)

		if _, ok := dedupeContracts[node.Token.Fa.Contract]; !ok {
			dedupeContracts[node.Token.Fa.Contract] = multichain.ChainAgnosticContract{
				Address:        node.Token.Fa.Contract,
				Symbol:         node.Token.Symbol,
				Name:           node.Token.Fa.Name,
				Description:    node.Token.Fa.Description,
				CreatorAddress: node.Token.Fa.Creator_Address,
				LatestBlock:    persist.BlockNumber(node.Token.Fa.Level),
			}
			returnContracts = append(returnContracts, dedupeContracts[node.Token.Fa.Contract])
		}

		tokenID := persist.TokenID(node.Token.Token_ID.toBase16String())

		agnosticToken := multichain.ChainAgnosticToken{
			TokenType:   persist.TokenTypeERC1155,
			Description: node.Token.Description,
			Name:        node.Token.Name,
			TokenID:     tokenID,

			ContractAddress: dedupeContracts[node.Token.Fa.Contract].Address,
			Quantity:        persist.HexString(fmt.Sprintf("%x", node.Token.Holders[0].Quantity)),
			TokenMetadata:   metadata,
			OwnerAddress:    tzOwnerAddress,
			BlockNumber:     persist.BlockNumber(node.Token.Level),
		}
		returnTokens = append(returnTokens, agnosticToken)
	}

	return returnTokens, returnContracts, nil
}

func (p *TezosObjktProvider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"contractAddress": tokenIdentifiers.ContractAddress,
		"tokenID":         tokenIdentifiers.TokenID,
		"ownerAddress":    ownerAddress,
	})

	tzOwnerAddress, err := toTzAddress(ownerAddress)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	tokenInDecimal, err := strconv.ParseInt(tokenIdentifiers.TokenID.String(), 16, 64)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	var query tokensByIdentifiersQuery

	if err := retry.RetryQuery(ctx, p.gql, &query, inputArgs{
		"contractAddress": graphql.String(tokenIdentifiers.ContractAddress),
		"ownerAddress":    graphql.String(tzOwnerAddress),
		"tokenID":         graphql.String(strconv.Itoa(int(tokenInDecimal))),
	}); err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(query.Token) < 1 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrNoTokensFoundByIdentifiers{tokenIdentifiers}
	}

	token := query.Token[0]

	metadata := createMetadata(token)

	agnosticContract := multichain.ChainAgnosticContract{
		Address:        token.Fa.Contract,
		Symbol:         token.Symbol,
		Name:           token.Fa.Name,
		Description:    token.Fa.Description,
		CreatorAddress: token.Fa.Creator_Address,
		LatestBlock:    persist.BlockNumber(token.Fa.Level),
	}

	tokenID := persist.TokenID(token.Token_ID.toBase16String())

	agnosticToken := multichain.ChainAgnosticToken{
		TokenType:       persist.TokenTypeERC1155,
		Description:     token.Description,
		Name:            token.Name,
		TokenID:         tokenID,
		ContractAddress: agnosticContract.Address,
		Quantity:        persist.HexString(fmt.Sprintf("%x", token.Holders[0].Quantity)),
		TokenMetadata:   metadata,
		OwnerAddress:    ownerAddress,
		BlockNumber:     persist.BlockNumber(token.Level),
	}

	return agnosticToken, agnosticContract, nil
}

func (p *TezosObjktProvider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
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
			return nil, multichain.ChainAgnosticContract{}, err
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
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens found for contract")
	}

	// Truncate tokens if there is a max limit
	if maxLimit > 0 && len(tokens) > maxLimit {
		tokens = tokens[:maxLimit]
	}

	agnosticContract := multichain.ChainAgnosticContract{
		Address:        tokens[0].Fa.Contract,
		Symbol:         tokens[0].Symbol,
		Name:           tokens[0].Fa.Name,
		Description:    tokens[0].Fa.Description,
		CreatorAddress: tokens[0].Fa.Creator_Address,
		LatestBlock:    persist.BlockNumber(tokens[0].Fa.Level),
	}

	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	for _, token := range tokens {
		tokenID := persist.TokenID(token.Token_ID.toBase16String())
		metadata := createMetadata(token)
		// Create token per holder
		for _, holder := range token.Holders {
			agnosticToken := multichain.ChainAgnosticToken{
				TokenType:       persist.TokenTypeERC1155,
				Description:     token.Description,
				Name:            token.Name,
				TokenID:         tokenID,
				ContractAddress: agnosticContract.Address,
				Quantity:        persist.HexString(fmt.Sprintf("%x", holder.Quantity)),
				TokenMetadata:   metadata,
				OwnerAddress:    holder.Holder_Address,
				BlockNumber:     persist.BlockNumber(token.Level),
			}
			returnTokens = append(returnTokens, agnosticToken)
		}
	}

	return returnTokens, agnosticContract, nil
}

func (p *TezosObjktProvider) GetTokensByContractAddressAndOwner(ctx context.Context, owner, contractAddress persist.Address, maxLimit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"contractAddress": contractAddress, "owner": owner})

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
			"ownerAddress":    graphql.String(owner),
			"limit":           graphql.Int(pageSize),
			"offset":          graphql.Int(offset),
		}); err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
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
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens found for contract")
	}

	// Truncate tokens if there is a max limit
	if maxLimit > 0 && len(tokens) > maxLimit {
		tokens = tokens[:maxLimit]
	}

	agnosticContract := multichain.ChainAgnosticContract{
		Address:        tokens[0].Fa.Contract,
		Symbol:         tokens[0].Symbol,
		Name:           tokens[0].Fa.Name,
		Description:    tokens[0].Fa.Description,
		CreatorAddress: tokens[0].Fa.Creator_Address,
		LatestBlock:    persist.BlockNumber(tokens[0].Fa.Level),
	}

	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	for _, token := range tokens {
		tokenID := persist.TokenID(token.Token_ID.toBase16String())
		metadata := createMetadata(token)
		// Create token per holder
		for _, holder := range token.Holders {
			agnosticToken := multichain.ChainAgnosticToken{
				TokenType:       persist.TokenTypeERC1155,
				Description:     token.Description,
				Name:            token.Name,
				TokenID:         tokenID,
				ContractAddress: agnosticContract.Address,
				Quantity:        persist.HexString(fmt.Sprintf("%x", holder.Quantity)),
				TokenMetadata:   metadata,
				OwnerAddress:    holder.Holder_Address,
				BlockNumber:     persist.BlockNumber(token.Level),
			}
			returnTokens = append(returnTokens, agnosticToken)
		}
	}

	return returnTokens, agnosticContract, nil
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
