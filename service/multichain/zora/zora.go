package zora

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/machinebox/graphql"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var zoraURL = "https://api.zora.co/graphql"
var goldskyURL = "https://api.goldsky.com/api/public/project_clhk16b61ay9t49vm6ntn4mkz/subgraphs/zora-create-zora-mainnet/stable/gn"

// Provider is an the struct for retrieving data from the zora blockchain
type Provider struct {
	zoraAPIKey string
	httpClient *http.Client
	zgql       *graphql.Client
	ggql       *graphql.Client
}

type tokenID string

func (t tokenID) toBase16String() string {
	big, ok := new(big.Int).SetString(string(t), 10)
	if !ok {
		panic("invalid token ID")
	}
	return big.Text(16)
}

type token struct {
	TokenID       tokenID                `json:"tokenId"`
	TokenURL      string                 `json:"tokenUrl"`
	TokenStandard string                 `json:"tokenStandard"`
	TokenContract tokenContract          `json:"tokenContract"`
	Metadata      map[string]interface{} `json:"metadata"`
	Description   string                 `json:"description"`
	Name          string                 `json:"name"`
	Owner         string                 `json:"owner"`
	Image         struct {
		URL string `json:"url"`
	} `json:"image"`
}

type tokenContract struct {
	CollectionAddress string `json:"collectionAddress"`
	Name              string `json:"name"`
	Symbol            string `json:"symbol"`
	Description       string `json:"description"`
}

type pageInfo struct {
	hasNextPage bool   `json:"hasNextPage"`
	endCursor   string `json:"endCursor"`
	limit       int    `json:"limit"`
}

type tokenNode struct {
	Token token `json:"token"`
}
type getTokensResponse struct {
	Tokens struct {
		Nodes    []tokenNode `json:"nodes"`
		PageInfo pageInfo    `json:"pageInfo"`
	} `json:"tokens"`
}

type getContractCreatorResponse struct {
	ZoraCreateContracts []struct {
		Creator string `json:"creator"`
	} `json:"zoraCreateContracts"`
}

type getContractsResponse struct {
	Collections struct {
		Nodes []tokenContract `json:"nodes"`
	} `json:"collections"`
}

type getZoraCreateContractsResponse struct {
	ZoraCreateContracts []struct {
		Address string `json:"address"`
		Name    string `json:"name"`
		Symbol  string `json:"symbol"`
		Creator string `json:"creator"`
	} `json:"zoraCreateContracts"`
}
type customTransport struct {
	underlyingTransport http.RoundTripper
	apiKey              string
}

func (t *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("X-API-KEY", t.apiKey)
	return t.underlyingTransport.RoundTrip(req)
}

// NewProvider creates a new zora Provider
func NewProvider(httpClient *http.Client) *Provider {

	// if api key exists, add to headers with X-API-KEY
	if env.GetString("ZORA_API_KEY") != "" {
		httpClient.Transport = &customTransport{
			underlyingTransport: httpClient.Transport,
			apiKey:              env.GetString("ZORA_API_KEY"),
		}
	}
	return &Provider{
		zgql:       graphql.NewClient(zoraURL, graphql.WithHTTPClient(httpClient)),
		ggql:       graphql.NewClient(goldskyURL, graphql.WithHTTPClient(httpClient)),
		httpClient: httpClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {

	return multichain.BlockchainInfo{
		Chain:   persist.ChainZora,
		ChainID: 7777777,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the zora Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	return d.getTokensByWalletAddressPaginate(ctx, addr, "")
}

func (d *Provider) getTokensWithRequest(ctx context.Context, req string, owner, collection persist.Address, endCursor string) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

	greq := graphql.NewRequest(req)
	greq.Var("address", owner)
	greq.Var("collection", collection)
	greq.Var("after", endCursor)
	greq.Var("limit", 500)

	resp := getTokensResponse{}
	err := d.zgql.Run(ctx, greq, &resp)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting tokens from zora: %w", err)
	}

	nodesToTokens, _ := util.Map(resp.Tokens.Nodes, func(n tokenNode) (token, error) {
		return n.Token, nil
	})
	tokens, contracts, err := d.tokensToChainAgnostic(ctx, nodesToTokens)
	if err != nil {
		return nil, nil, err
	}

	if resp.Tokens.PageInfo.hasNextPage {
		nextTokens, nextContracts, err := d.getTokensWithRequest(ctx, req, owner, collection, resp.Tokens.PageInfo.endCursor)
		if err != nil {
			return nil, nil, err
		}
		tokens = append(tokens, nextTokens...)
		contracts = append(contracts, nextContracts...)
	}

	return tokens, contracts, nil
}

func (d *Provider) GetTokensByTokenIdentifiersAndOwner(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("implement me")
}

func (d *Provider) getTokensByWalletAddressPaginate(ctx context.Context, addr persist.Address, endCursor string) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	req := fmt.Sprintf(`query tokensByWalletAddress($address: String!, $after:String!, $limit:Int!) {
  tokens(where:{ownerAddresses:[$address]}, networks:{network:ZORA, chain: ZORA_MAINNET}, pagination: {limit: $limit, after:$after}, sort:{sortKey: TRANSFERRED, sortDirection: ASC}) {
    nodes {
      token {
        tokenId
        tokenUrl
        tokenStandard
        tokenContract {
          collectionAddress
          name
          symbol
          description
        }
        collectionName
        metadata
        description
        name
        owner
        image {
          url
        }
      }
    }
    pageInfo {
      limit
      endCursor
      hasNextPage
    }
  }
}`)

	return d.getTokensWithRequest(ctx, req, addr, "", endCursor)
}

func (d *Provider) getTokensByContractAddressPaginate(ctx context.Context, addr persist.Address, endCursor string) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	req := fmt.Sprintf(`query tokensByContractAddress($collection: String!, $after:String!, $limit:Int!) {
  tokens(where:{collectionAddresses:[$collection]}, networks:{network:ZORA, chain: ZORA_MAINNET}, pagination: {limit: $limit, after:$after}, sort:{sortKey: TRANSFERRED, sortDirection: ASC}) {
    nodes {
      token {
        tokenId
        tokenUrl
        tokenStandard
        tokenContract {
          collectionAddress
          name
          symbol
          description
        }
        collectionName
        metadata
        description
        name
        owner
        image {
          url
        }
      }
    }
    pageInfo {
      limit
      endCursor
      hasNextPage
    }
  }
}`)

	tokens, contracts, err := d.getTokensWithRequest(ctx, req, addr, "", endCursor)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, errors.New("no contract found")
	}
	return tokens, contracts[0], nil
}

func (d *Provider) getTokensByWalletAddressAndContractPaginate(ctx context.Context, addr, contractAddress persist.Address, endCursor string) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	req := fmt.Sprintf(`query tokensByAll($address: String!, $collection: String!, $after:String!, $limit:Int!) {
  tokens(where:{collectionAddresses: [$collection], ownerAddresses: [$address]}, networks:{network:ZORA, chain: ZORA_MAINNET}, pagination: {limit: $limit, after:$after}, sort:{sortKey: TRANSFERRED, sortDirection: ASC}) {
    nodes {
      token {
        tokenId
        tokenUrl
        tokenStandard
        tokenContract {
          collectionAddress
          name
          symbol
          description
        }
        collectionName
        metadata
        description
        name
        owner
        image {
          url
        }
      }
    }
    pageInfo {
      limit
      endCursor
      hasNextPage
    }
  }
}`)

	tokens, contracts, err := d.getTokensWithRequest(ctx, req, addr, contractAddress, endCursor)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for address %s amd owner %s", contractAddress, addr)
	}

	return tokens, contracts[0], nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the zora Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return d.getTokensByContractAddressPaginate(ctx, contractAddress, "")
}

func (d *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, ownerAddress persist.Address, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return d.getTokensByWalletAddressAndContractPaginate(ctx, ownerAddress, contractAddress, "")
}

// GetContractByAddress retrieves an zora contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req := graphql.NewRequest(fmt.Sprintf(`{
	  "query": "query contractByAddress($address:String!) {
  collections(where:{collectionAddresses:[$address]}, networks:{network:ZORA, chain: ZORA_MAINNET}, pagination: {limit: 1}, sort:{sortKey: CREATED, sortDirection: ASC}) {
    nodes {
      address
      description
      name
      symbol
    }
  }",
	  "variables": {
		"address": %s
	  }
	}`, addr.String()))

	resp := getContractsResponse{}
	err := d.zgql.Run(ctx, req, &resp)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	if len(resp.Collections.Nodes) == 0 {
		return multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for address %s", addr.String())
	}

	return d.contractToChainAgnostic(ctx, resp.Collections.Nodes[0])

}

// GetContractsByOwnerAddress retrieves all contracts owned by a given address
func (d *Provider) GetContractsByOwnerAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticContract, error) {
	req := graphql.NewRequest(`query createdTokens($creator: Bytes!) {
  zoraCreateContracts(where: {creator: $creator}) {
    address
    name
    symbol
    creator
  }
}`)

	req.Var("creator", addr.String())

	resp := getZoraCreateContractsResponse{}
	err := d.ggql.Run(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.ZoraCreateContracts) == 0 {
		return nil, fmt.Errorf("no contract found for address %s", addr.String())
	}

	result := make([]multichain.ChainAgnosticContract, len(resp.ZoraCreateContracts))
	for i, contract := range resp.ZoraCreateContracts {
		result[i] = multichain.ChainAgnosticContract{
			Descriptors: multichain.ChainAgnosticContractDescriptors{
				Symbol:         contract.Symbol,
				Name:           contract.Name,
				CreatorAddress: persist.Address(strings.ToLower(contract.Creator)),
			},
			Address: persist.Address(strings.ToLower(contract.Address)),
		}
	}

	return result, nil
}

func (d *Provider) tokensToChainAgnostic(ctx context.Context, tokens []token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	result := make([]multichain.ChainAgnosticToken, len(tokens))
	contracts := map[string]multichain.ChainAgnosticContract{}
	for i, token := range tokens {
		var tokenType persist.TokenType
		switch token.TokenStandard {
		case "ERC721":
			tokenType = persist.TokenTypeERC721
		case "ERC1155":
			tokenType = persist.TokenTypeERC1155
		default:
			panic(fmt.Sprintf("unknown token standard %s %+v", token.TokenStandard, token))
		}
		result[i] = multichain.ChainAgnosticToken{
			Descriptors: multichain.ChainAgnosticTokenDescriptors{
				Name:        token.Name,
				Description: token.Description,
			},
			TokenType:       tokenType,
			TokenMetadata:   token.Metadata,
			TokenID:         persist.TokenID(token.TokenID.toBase16String()),
			Quantity:        "1",
			OwnerAddress:    persist.Address(strings.ToLower(token.Owner)),
			ContractAddress: persist.Address(strings.ToLower(token.TokenContract.CollectionAddress)),
			ExternalURL:     token.TokenURL,
			FallbackMedia: persist.FallbackMedia{
				// IPFS url
				ImageURL: persist.NullString(token.Image.URL),
			},
		}

		if _, ok := contracts[token.TokenContract.CollectionAddress]; ok {
			continue
		}

		contract, err := d.contractToChainAgnostic(ctx, token.TokenContract)
		if err != nil {
			return nil, nil, err
		}

		contracts[token.TokenContract.CollectionAddress] = contract

	}

	contractResults := util.MapValues(contracts)

	return result, contractResults, nil

}

func (d *Provider) contractToChainAgnostic(ctx context.Context, contract tokenContract) (multichain.ChainAgnosticContract, error) {
	creator, err := d.getContractCreator(ctx, persist.Address(strings.ToLower(contract.CollectionAddress)))
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return multichain.ChainAgnosticContract{
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:         contract.Symbol,
			Name:           contract.Name,
			Description:    contract.Description,
			CreatorAddress: creator,
		},
		Address: persist.Address(strings.ToLower(contract.CollectionAddress)),
	}, nil
}

func (d *Provider) getContractCreator(ctx context.Context, address persist.Address) (persist.Address, error) {
	req := graphql.NewRequest(`query createdContract($address:Bytes!) {
  zoraCreateContracts(where: {address:$address}) {
    creator
  }
}`)
	req.Var("address", address.String())

	resp := getContractCreatorResponse{}
	err := d.ggql.Run(ctx, req, &resp)
	if err != nil {
		return "", err
	}
	if len(resp.ZoraCreateContracts) == 0 {
		return "", fmt.Errorf("contract not found at address %s", address)
	}
	return persist.Address(strings.ToLower(resp.ZoraCreateContracts[0].Creator)), nil
}
