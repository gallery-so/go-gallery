package zora

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/machinebox/graphql"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var zoraRESTURL = "https://api.zora.co/discover"
var goldskyURL = "https://api.goldsky.com/api/public/project_clhk16b61ay9t49vm6ntn4mkz/subgraphs/zora-create-zora-mainnet/stable/gn"

// Provider is an the struct for retrieving data from the zora blockchain
type Provider struct {
	zoraAPIKey string
	httpClient *http.Client
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

type zoraToken struct {
	ChainName         string         `json:"chain_name"`
	CollectionAddress string         `json:"collection_address"`
	TokenID           tokenID        `json:"token_id"`
	TokenStandard     string         `json:"token_standard"`
	Owner             string         `json:"owner"`
	Metadata          map[string]any `json:"metadata"`
	Mintable          struct {
		CreatorAddress string `json:"creator_address"`
		Collection     struct {
			Symbol      string `json:"symbol"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
	} `json:"mintable"`
	Media struct {
		ImagePreview struct {
			Raw            string `json:"raw"`
			MimeType       string `json:"mime_type"`
			EncodedLarge   string `json:"encoded_large"`
			EncodedPreview string `json:"encoded_preview"`
		} `json:"image_preview"`
		MimeType string `json:"mime_type"`
	} `json:"media"`
}
type zoraBalanceToken struct {
	Balance int       `json:"balance"`
	Token   zoraToken `json:"token"`
}

type getContractCreatorResponse struct {
	ZoraCreateContracts []struct {
		Creator string `json:"creator"`
	} `json:"zoraCreateContracts"`
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
	if t.underlyingTransport == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.underlyingTransport.RoundTrip(req)
}

// NewProvider creates a new zora Provider
func NewProvider(httpClient *http.Client) *Provider {

	zoraClient := httpClient

	// if api key exists, add to headers with X-API-KEY
	if env.GetString("ZORA_API_KEY") != "" {
		zoraClient = &http.Client{
			Transport: &customTransport{
				underlyingTransport: httpClient.Transport,
				apiKey:              env.GetString("ZORA_API_KEY"),
			},
			CheckRedirect: httpClient.CheckRedirect,
			Jar:           httpClient.Jar,
			Timeout:       httpClient.Timeout,
		}
	}

	return &Provider{
		ggql:       graphql.NewClient(goldskyURL, graphql.WithHTTPClient(httpClient)),
		httpClient: zoraClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo() multichain.BlockchainInfo {

	return multichain.BlockchainInfo{
		Chain:      persist.ChainZora,
		ChainID:    7777777,
		ProviderID: "zora",
	}
}

type getBalanceTokensResponse struct {
	Tokens      []zoraBalanceToken `json:"results"`
	HasNextPage bool               `json:"has_next_page"`
}

type getTokensResponse struct {
	Tokens      []zoraToken `json:"results"`
	HasNextPage bool        `json:"has_next_page"`
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the zora Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/user/%s/tokens?chain_names=ZORA-MAINNET&sort_direction=DESC", zoraRESTURL, addr.String())
	return d.getTokens(ctx, url, nil, true)
}

func (d *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	url := fmt.Sprintf("%s/user/%s/tokens?chain_names=ZORA-MAINNET&sort_direction=DESC", zoraRESTURL, addr.String())
	go func() {
		_, _, err := d.getTokens(ctx, url, rec, true)
		if err != nil {
			errChan <- err
			return
		}
	}()
	return rec, errChan
}

func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, owner persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/contract/ZORA-MAINNET/%s/%s", zoraRESTURL, ti.ContractAddress.String(), ti.TokenID.Base10String())
	return d.getToken(ctx, owner, url)
}

func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	url := fmt.Sprintf("%s/contract/ZORA-MAINNET/%s/%s", zoraRESTURL, ti.ContractAddress.String(), ti.TokenID.Base10String())
	token, _, err := d.getToken(ctx, "", url)
	if err != nil {
		return nil, err
	}

	return token.TokenMetadata, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the zora Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/tokens/ZORA-MAINNET/%s?&sort_key=CREATED&sort_direction=DESC", zoraRESTURL, contractAddress.String())
	tokens, contracts, err := d.getTokens(ctx, url, nil, false)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contracts) != 1 {
		logger.For(ctx).Warnf("invalid number of contracts returned from zora: %d", len(contracts))
		return nil, multichain.ChainAgnosticContract{}, nil
	}
	return tokens, contracts[0], nil

}

func (d *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, ownerAddress persist.Address, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	tokens, contracts, err := d.GetTokensByWalletAddress(ctx, ownerAddress)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	// filter for contract and tokens
	filteredTokens := []multichain.ChainAgnosticToken{}
	for _, token := range tokens {
		if strings.EqualFold(token.ContractAddress.String(), contractAddress.String()) {
			filteredTokens = append(filteredTokens, token)
		}
	}
	foundContract := multichain.ChainAgnosticContract{}
	for _, contract := range contracts {
		if strings.EqualFold(contract.Address.String(), contractAddress.String()) {
			foundContract = contract
		}
	}

	return filteredTokens, foundContract, nil
}

// GetContractByAddress retrieves an zora contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/contract/ZORA-MAINNET/%s", zoraRESTURL, addr.String())
	_, contract, err := d.getToken(ctx, "", url)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return contract, nil

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

func (d *Provider) getTokens(ctx context.Context, url string, rec chan<- multichain.ChainAgnosticTokensAndContracts, balance bool) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	offset := 0
	limit := 50
	allTokens := []multichain.ChainAgnosticToken{}
	allContracts := []multichain.ChainAgnosticContract{}
	for ; ; offset += limit {
		urlWithPagination := fmt.Sprintf("%s&offset=%d&limit=%d", url, offset, limit)
		logger.For(ctx).Infof("getting zora tokens from %s", urlWithPagination)
		req, err := http.NewRequestWithContext(ctx, "GET", urlWithPagination, nil)
		if err != nil {
			return nil, nil, err
		}
		resp, err := d.httpClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()

		if balance {
			var result getBalanceTokensResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				return nil, nil, err
			}

			logger.For(ctx).Infof("zora raw tokens retrieved: %d", len(result.Tokens))

			tokens, contracts := d.balanceTokensToChainAgnostic(ctx, result.Tokens)

			allTokens = append(allTokens, tokens...)
			allContracts = append(allContracts, contracts...)

			if rec != nil {
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    tokens,
					Contracts: contracts,
				}
			}

			if len(result.Tokens) < limit || !result.HasNextPage {
				break
			}
		} else {
			var result getTokensResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				return nil, nil, err
			}

			logger.For(ctx).Infof("zora raw tokens retrieved: %d", len(result.Tokens))

			tokens, contracts := d.tokensToChainAgnostic(ctx, result.Tokens)

			allTokens = append(allTokens, tokens...)
			allContracts = append(allContracts, contracts...)

			if rec != nil {
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    tokens,
					Contracts: contracts,
				}
			}

			if len(result.Tokens) < limit || !result.HasNextPage {
				break
			}
		}

	}

	logger.For(ctx).Infof("zora tokens retrieved: %d", len(allTokens))

	return allTokens, allContracts, nil
}

func (d *Provider) getToken(ctx context.Context, ownerAddress persist.Address, url string) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()

	var result zoraToken
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	tokens, contracts := d.tokensToChainAgnostic(ctx, []zoraToken{result})

	if len(tokens) == 0 || len(contracts) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("invalid number of tokens or contracts returned from zora: %d %d", len(tokens), len(contracts))
	}

	if ownerAddress != "" {
		for _, token := range tokens {
			if strings.EqualFold(token.OwnerAddress.String(), ownerAddress.String()) {
				return token, contracts[0], nil
			}
		}
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no token found for owner %s", ownerAddress.String())
	}

	return tokens[0], contracts[0], nil
}

func (d *Provider) balanceTokensToChainAgnostic(ctx context.Context, tokens []zoraBalanceToken) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	result := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	contracts := map[string]multichain.ChainAgnosticContract{}
	for _, token := range tokens {
		converted, err := d.tokenToAgnostic(ctx, token.Token)
		if err != nil {
			logger.For(ctx).Errorf("error converting zora token %+v: %s", token, err.Error())
			continue
		}
		balanceAsBig := new(big.Int).SetInt64(int64(token.Balance))
		balanceAsHex := balanceAsBig.Text(16)
		if balanceAsHex == "" {
			balanceAsHex = "1"
		}
		converted.Quantity = persist.HexString(balanceAsHex)

		result = append(result, converted)

		if _, ok := contracts[token.Token.CollectionAddress]; ok {
			continue
		}

		contracts[token.Token.CollectionAddress] = d.contractToChainAgnostic(ctx, token.Token)

	}

	contractResults := util.MapValues(contracts)

	logger.For(ctx).Infof("zora tokens converted: %d (%d)", len(result), len(contractResults))

	return result, contractResults

}

func (d *Provider) tokensToChainAgnostic(ctx context.Context, tokens []zoraToken) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	result := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	contracts := map[string]multichain.ChainAgnosticContract{}
	for _, token := range tokens {
		converted, err := d.tokenToAgnostic(ctx, token)
		if err != nil {
			logger.For(ctx).Errorf("error converting zora token %+v: %s", token, err.Error())
			continue
		}
		result = append(result, converted)

		if _, ok := contracts[token.CollectionAddress]; ok {
			continue
		}

		contracts[token.CollectionAddress] = d.contractToChainAgnostic(ctx, token)

	}

	contractResults := util.MapValues(contracts)

	logger.For(ctx).Infof("zora tokens converted: %d (%d)", len(result), len(contractResults))

	return result, contractResults

}

func (*Provider) tokenToAgnostic(ctx context.Context, token zoraToken) (multichain.ChainAgnosticToken, error) {
	var tokenType persist.TokenType
	switch token.TokenStandard {
	case "ERC721":
		tokenType = persist.TokenTypeERC721
	case "ERC1155":
		tokenType = persist.TokenTypeERC1155
	default:
		return multichain.ChainAgnosticToken{}, fmt.Errorf("unknown token standard %s", token.TokenStandard)
	}
	metadataName, _ := token.Metadata["name"].(string)
	metadataDescription, _ := token.Metadata["description"].(string)

	return multichain.ChainAgnosticToken{
		Descriptors: multichain.ChainAgnosticTokenDescriptors{
			Name:        metadataName,
			Description: metadataDescription,
		},
		TokenType:       tokenType,
		TokenMetadata:   token.Metadata,
		TokenID:         persist.TokenID(token.TokenID.toBase16String()),
		Quantity:        persist.HexString("1"),
		OwnerAddress:    persist.Address(strings.ToLower(token.Owner)),
		ContractAddress: persist.Address(strings.ToLower(token.CollectionAddress)),

		FallbackMedia: persist.FallbackMedia{
			ImageURL: persist.NullString(token.Media.ImagePreview.EncodedPreview),
		},
	}, nil
}

func (d *Provider) contractToChainAgnostic(ctx context.Context, token zoraToken) multichain.ChainAgnosticContract {

	return multichain.ChainAgnosticContract{
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:         token.Mintable.Collection.Symbol,
			Name:           token.Mintable.Collection.Name,
			Description:    token.Mintable.Collection.Description,
			CreatorAddress: persist.Address(strings.ToLower(token.Mintable.CreatorAddress)),
		},
		Address: persist.Address(strings.ToLower(token.CollectionAddress)),
	}
}
