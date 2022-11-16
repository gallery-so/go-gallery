package tezos

import (
	"context"
	"fmt"
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

type attribute struct {
	Name  string
	Value string
	Type  string
	ID    string
}

type contract struct {
	Name           string
	Contract       persist.Address
	Description    string
	CreatorAddress persist.Address
	Level          int
	Type           tokenStandard
}

type token struct {
	ArtifactURI     string
	Rights          string
	Decimals        int
	Description     string
	DisplayURI      string
	Metadata        string
	Name            string
	Symbol          string
	ThumbnailURI    string
	TokenID         tokenID
	Level           int
	IsBooleanAmount bool
	Attributes      []struct {
		Attribute attribute
	}
}

type holder struct {
	Quantity      int
	HolderAddress persist.Address
}

type heldToken struct {
	token
	Holders []holder `graphql:"holders(where: {quantity: {_gt: '0'}})"`
	Fa      contract
}

type tokensByWalletQuery struct {
	Result []struct {
		HeldTokens []heldToken `graphql:"held_tokens(limit: $limit, offset: $offset, where: {quantity: {_gt: '0'}})"`
	} `graphql:"holder(where: {address: {_eq: $ownerAddress}}, limit: 1)"`
}

type tokensByContractQuery struct {
	Result []struct {
		Tokens []heldToken `graphql:"tokens(limit: $limit, offset: $offset, distinct_on: token_id)"`
	} `graphql:"fa(where: {contract: {_eq: $contractAddress}, type: {_eq: 'fa2'}})"`
}

type tokensByIdentifiersQuery struct {
	Tokens []heldToken `graphql:"token(where: {fa_contract: {_eq: $contractAddress}, fa_type: {_eq: 'fa2'}, token_id: {_eq: $tokenID}, holders: {holder: {address: {_eq: $ownerAddress}}}, quantity: {_gt: '0'}}, distinct_on: token_id)"`
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

func (d *TezosObjktProvider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

func (p *TezosObjktProvider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address, maxLimit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"ownerAddress": ownerAddress})
	tzOwnerAddress, err := toTzAddress(ownerAddress)
	if err != nil {
		return nil, nil, err
	}

	pageSize := maxLimit
	if maxLimit > maxPageSize {
		pageSize = maxPageSize
	}

	var query tokensByWalletQuery
	tokens := make([]heldToken, 0, maxLimit)

	// Paginate results
	for len(tokens) < maxLimit {
		if err := p.gql.Query(ctx, &query, inputArgs{
			"limit":        pageSize,
			"offset":       offset,
			"ownerAddress": tzOwnerAddress,
		}); err != nil {
			return nil, nil, err
		}

		if len(query.Result) < 1 || len(query.Result[0].HeldTokens) < 1 {
			break
		}

		tokens = append(tokens, query.Result[0].HeldTokens...)
		offset += len(query.Result[0].HeldTokens)
	}

	tokens = tokens[:maxLimit]
	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	returnContracts := make([]multichain.ChainAgnosticContract, 0)
	dedupeContracts := make(map[persist.Address]multichain.ChainAgnosticContract)

	for _, token := range tokens {
		// FA1.2 is the equivalent of ERC-20 on Tezos
		if token.Fa.Type == tokenStandardFa12 {
			continue
		}

		// Create the metadata
		metadata := createMetadata(token.token)

		if _, ok := dedupeContracts[token.Fa.Contract]; !ok {
			dedupeContracts[token.Fa.Contract] = multichain.ChainAgnosticContract{
				Address:        token.Fa.Contract,
				Symbol:         token.Symbol,
				Name:           token.Fa.Name,
				Description:    token.Fa.Description,
				CreatorAddress: token.Fa.CreatorAddress,
				LatestBlock:    persist.BlockNumber(token.Fa.Level),
			}
			returnContracts = append(returnContracts, dedupeContracts[token.Fa.Contract])
		}

		// Create the media
		tokenID := persist.TokenID(token.TokenID.toBase16String())
		media := makeTempMedia(ctx, tokenID, dedupeContracts[token.Fa.Contract].Address, metadata, p.ipfsGatewayURL)
		agnosticToken := multichain.ChainAgnosticToken{
			TokenType:       persist.TokenTypeERC1155,
			Description:     token.Description,
			Name:            token.Name,
			TokenID:         tokenID,
			Media:           media,
			ContractAddress: dedupeContracts[token.Fa.Contract].Address,
			Quantity:        persist.HexString(fmt.Sprintf("%x", token.Holders[0].Quantity)),
			TokenMetadata:   metadata,
			OwnerAddress:    tzOwnerAddress,
			BlockNumber:     persist.BlockNumber(token.Level),
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

	var query tokensByIdentifiersQuery

	if err := p.gql.Query(ctx, &query, inputArgs{}); err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(query.Tokens) < 1 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrNoTokensFoundByIdentifiers{tokenIdentifiers}
	}

	token := query.Tokens[0]

	// Create the metadata
	metadata := createMetadata(token.token)

	agnosticContract := multichain.ChainAgnosticContract{
		Address:        token.Fa.Contract,
		Symbol:         token.Symbol,
		Name:           token.Fa.Name,
		Description:    token.Fa.Description,
		CreatorAddress: token.Fa.CreatorAddress,
		LatestBlock:    persist.BlockNumber(token.Fa.Level),
	}

	// Create the media
	tokenID := persist.TokenID(token.TokenID.toBase16String())
	media := makeTempMedia(ctx, tokenID, agnosticContract.Address, metadata, p.ipfsGatewayURL)

	agnosticToken := multichain.ChainAgnosticToken{
		TokenType:       persist.TokenTypeERC1155,
		Description:     token.Description,
		Name:            token.Name,
		TokenID:         tokenID,
		Media:           media,
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
	tzContractAddress, err := toTzAddress(contractAddress)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	pageSize := maxLimit
	if maxLimit > maxPageSize {
		pageSize = maxPageSize
	}

	var query tokensByContractQuery
	tokens := make([]heldToken, 0, maxLimit)

	// Paginate results
	for len(tokens) < maxLimit {
		if err := p.gql.Query(ctx, &query, inputArgs{
			"contractAddress": tzContractAddress,
			"limit":           pageSize,
			"offset":          offset,
		}); err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}

		if len(query.Result) < 1 || len(query.Result[0].Tokens) < 1 {
			break
		}

		tokens = append(tokens, query.Result[0].Tokens...)
		offset += len(query.Result[0].Tokens)
	}

	// No matching query results
	if len(tokens) < 1 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens found for contract")
	}

	tokens = tokens[:maxLimit]

	agnosticContract := multichain.ChainAgnosticContract{
		Address:        tokens[0].Fa.Contract,
		Symbol:         tokens[0].Symbol,
		Name:           tokens[0].Fa.Name,
		Description:    tokens[0].Fa.Description,
		CreatorAddress: tokens[0].Fa.CreatorAddress,
		LatestBlock:    persist.BlockNumber(tokens[0].Fa.Level),
	}

	returnTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	for _, token := range tokens {
		tokenID := persist.TokenID(token.TokenID.toBase16String())
		metadata := createMetadata(token.token)
		media := makeTempMedia(ctx, tokenID, agnosticContract.Address, metadata, p.ipfsGatewayURL)

		// Create token per holder
		for _, holder := range token.Holders {
			agnosticToken := multichain.ChainAgnosticToken{
				TokenType:       persist.TokenTypeERC1155,
				Description:     token.Description,
				Name:            token.Name,
				TokenID:         tokenID,
				Media:           media,
				ContractAddress: agnosticContract.Address,
				Quantity:        persist.HexString(fmt.Sprintf("%x", holder.Quantity)),
				TokenMetadata:   metadata,
				OwnerAddress:    holder.HolderAddress,
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
	metadata["displayUri"] = t.DisplayURI
	metadata["artifactUri"] = t.ArtifactURI
	metadata["description"] = t.Description
	metadata["thumbnailUri"] = t.ThumbnailURI
	metadata["isBooleanAmount"] = t.IsBooleanAmount
	return metadata
}
