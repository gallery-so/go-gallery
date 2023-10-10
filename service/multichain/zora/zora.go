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

type getTokensResponse struct {
	Tokens      []zoraBalanceToken `json:"results"`
	HasNextPage bool               `json:"has_next_page"`
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the zora Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/user/%s/tokens?chain_names=ZORA-MAINNET&sort_direction=DESC", zoraRESTURL, addr.String())
	return d.getTokens(ctx, addr, url, nil)
}

func (d *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	url := fmt.Sprintf("%s/user/%s/tokens?chain_names=ZORA-MAINNET&sort_direction=DESC", zoraRESTURL, addr.String())
	go func() {
		_, _, err := d.getTokens(ctx, addr, url, rec)
		if err != nil {
			errChan <- err
			return
		}
	}()
	return rec, errChan
}

func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, owner persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/contract/ZORA_MAINNET/%s/%s", zoraRESTURL, ti.ContractAddress.String(), ti.TokenID.Base10String())
	return d.getToken(ctx, owner, url)
}

func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	// there is no way to get a single token by token ID, so we have to get all tokens for the contract and then filter
	url := fmt.Sprintf("%s/contract/ZORA_MAINNET/%s/%s", zoraRESTURL, ti.ContractAddress.String(), ti.TokenID.Base10String())
	token, _, err := d.getToken(ctx, "", url)
	if err != nil {
		return nil, err
	}

	return token.TokenMetadata, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the zora Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/tokens/ZORA_MAINNET/%s?offset=%d&limit=%d&sort_key=CREATED&sort_direction=DESC", zoraRESTURL, contractAddress.String(), offset, limit)
	tokens, contracts, err := d.getTokens(ctx, "", url, nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contracts) != 1 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("invalid number of contracts returned from zora: %d", len(contracts))
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
	url := fmt.Sprintf("%s/contract/ZORA_MAINNET/%s", zoraRESTURL, addr.String())
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

func (d *Provider) getTokens(ctx context.Context, ownerAddr persist.Address, url string, rec chan<- multichain.ChainAgnosticTokensAndContracts) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
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

		var result getTokensResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			return nil, nil, err
		}

		tokens, contracts, err := d.tokensToChainAgnostic(ctx, result.Tokens, ownerAddr)
		if err != nil {
			return nil, nil, err
		}

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

	tokens, contracts, err := d.tokensToChainAgnostic(ctx, []zoraBalanceToken{{
		Balance: 1,
		Token:   result,
	}}, ownerAddress)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	if len(tokens) != 1 || len(contracts) != 1 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("invalid number of tokens or contracts returned from zora: %d %d", len(tokens), len(contracts))
	}

	return tokens[0], contracts[0], nil
}

func (d *Provider) tokensToChainAgnostic(ctx context.Context, tokens []zoraBalanceToken, owner persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	result := make([]multichain.ChainAgnosticToken, len(tokens))
	contracts := map[string]multichain.ChainAgnosticContract{}
	for i, token := range tokens {
		var tokenType persist.TokenType
		switch token.Token.TokenStandard {
		case "ERC721":
			tokenType = persist.TokenTypeERC721
		case "ERC1155":
			tokenType = persist.TokenTypeERC1155
		default:
			panic(fmt.Sprintf("unknown token standard %s %+v", token.Token.TokenStandard, token))
		}
		metadataName, _ := token.Token.Metadata["name"].(string)
		metadataDescription, _ := token.Token.Metadata["description"].(string)

		balanceAsBig := new(big.Int).SetInt64(int64(token.Balance))
		balanceAsHex := balanceAsBig.Text(16)
		if balanceAsHex == "" {
			balanceAsHex = "1"
		}

		result[i] = multichain.ChainAgnosticToken{
			Descriptors: multichain.ChainAgnosticTokenDescriptors{
				Name:        metadataName,
				Description: metadataDescription,
			},
			TokenType:       tokenType,
			TokenMetadata:   token.Token.Metadata,
			TokenID:         persist.TokenID(token.Token.TokenID.toBase16String()),
			Quantity:        persist.HexString(balanceAsHex),
			OwnerAddress:    persist.Address(util.FirstNonEmptyString(strings.ToLower(token.Token.Owner), owner.String())),
			ContractAddress: persist.Address(strings.ToLower(token.Token.CollectionAddress)),

			FallbackMedia: persist.FallbackMedia{
				ImageURL: persist.NullString(token.Token.Media.ImagePreview.EncodedPreview),
			},
		}

		if _, ok := contracts[token.Token.CollectionAddress]; ok {
			continue
		}

		contracts[token.Token.CollectionAddress] = d.contractToChainAgnostic(ctx, token)

	}

	contractResults := util.MapValues(contracts)

	return result, contractResults, nil

}

func (d *Provider) contractToChainAgnostic(ctx context.Context, token zoraBalanceToken) multichain.ChainAgnosticContract {

	return multichain.ChainAgnosticContract{
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:         token.Token.Mintable.Collection.Symbol,
			Name:           token.Token.Mintable.Collection.Name,
			Description:    token.Token.Mintable.Collection.Description,
			CreatorAddress: persist.Address(strings.ToLower(token.Token.Mintable.CreatorAddress)),
		},
		Address: persist.Address(strings.ToLower(token.Token.CollectionAddress)),
	}
}
