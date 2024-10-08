package poap

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

/*
{
    "event": {
      "id": 0,
      "fancy_id": "string",
      "name": "string",
      "event_url": "string",
      "image_url": "string",
      "country": "string",
      "city": "string",
      "description": "string",
      "year": 0,
      "start_date": "string",
      "end_date": "string",
      "expiry_date": "string",
      "supply": 0
    },
    "tokenId": "string",
    "owner": "string",
    "chain": "string",
    "created": "string"
  }
*/

type tokenID string
type poapToken struct {
	Event struct {
		ID          int    `json:"id"`
		FancyID     string `json:"fancy_id"`
		Name        string `json:"name"`
		EventURL    string `json:"event_url"`
		ImageURL    string `json:"image_url"`
		Country     string `json:"country"`
		City        string `json:"city"`
		Description string `json:"description"`
		Year        int    `json:"year"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		ExpiryDate  string `json:"expiry_date"`
		Supply      int    `json:"supply"`
	} `json:"event"`
	TokenID tokenID `json:"tokenId"`
	Owner   string  `json:"owner"`
	Chain   string  `json:"chain"`
	Created string  `json:"created"`
}

/*
{
  "id": 1,
  "fancy_id": "some-event-2022",
  "name": "Example event 2022",
  "event_url": "https://poap.xyz",
  "image_url": "https://poap.xyz/image.png",
  "country": "Argentina",
  "city": "Buenos Aires",
  "description": "This is an example event",
  "year": 2022,
  "start_date": "07-18-2022",
  "end_date": "07-20-2022",
  "expiry_date": "08-31-2022",
  "created_date": "2022-07-12T14:22:45.278Z",
  "from_admin": true,
  "virtual_event": true,
  "event_template_id": 1,
  "event_host_id": 1,
  "secret_code": "234789",
  "email": "test@test.com",
  "private_event": true
}
*/

type poapEvent struct {
	ID          int    `json:"id"`
	FancyID     string `json:"fancy_id"`
	Name        string `json:"name"`
	EventURL    string `json:"event_url"`
	ImageURL    string `json:"image_url"`
	Description string `json:"description"`
}

// Provider is an the struct for retrieving data from the Tezos blockchain
type Provider struct {
	apiURL     string
	apiKey     string
	authToken  string
	httpClient *http.Client
}

// NewProvider creates a new Tezos Provider
func NewProvider(httpClient *http.Client) *Provider {
	return &Provider{
		apiURL:     "https://api.poap.tech",
		apiKey:     env.GetString("POAP_API_KEY"),
		authToken:  env.GetString("POAP_AUTH_TOKEN"),
		httpClient: httpClient,
	}
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Poap Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {

	// DOES NOT SUPPORT PAGINATION
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/actions/scan/%s", d.apiURL, addr.String()), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, util.GetErrFromResp(resp)
	}
	var tokens []poapToken
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, nil, err
	}

	resultTokens, resultContracts := d.poapsToTokens(tokens)
	return resultTokens, resultContracts, nil
}

func (d *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan common.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	go func() {
		defer close(rec)
		tokens, contracts, err := d.GetTokensByWalletAddress(ctx, addr)
		if err != nil {
			errChan <- err
			return
		}
		rec <- common.ChainAgnosticTokensAndContracts{
			Tokens:    tokens,
			Contracts: contracts,
		}
	}()
	return rec, errChan
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Poap Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	return nil, common.ChainAgnosticContract{}, fmt.Errorf("poap has no way to retrieve tokens by contract address")
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Poap Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers common.ChainAgnosticIdentifiers, limit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	tid := tokenIdentifiers.TokenID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/token/0x%s", d.apiURL, tid), nil)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, common.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}

	return []common.ChainAgnosticToken{d.poapToToken(token)}, d.poapToContract(token), nil
}

func (d *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, tokenIdentifiers common.ChainAgnosticIdentifiers) (common.ChainAgnosticTokenDescriptors, common.ChainAgnosticContractDescriptors, error) {
	tokens, contract, err := d.GetTokensByTokenIdentifiers(ctx, tokenIdentifiers, 1, 0)
	if err != nil {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, err
	}
	if len(tokens) == 0 {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, fmt.Errorf("no token found")
	}

	firstToken := tokens[0]
	return firstToken.Descriptors, contract.Descriptors, nil
}

// GetTokenByTokenIdentifiersAndOwner retrieves tokens for a token identifiers and owner address
func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers common.ChainAgnosticIdentifiers, ownerAddress persist.Address) (common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	tid := tokenIdentifiers.TokenID

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/token/0x%s", d.apiURL, tid), nil)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	return d.poapToToken(token), d.poapToContract(token), nil
}

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contract persist.Address, addr persist.Address, limit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/actions/scan/%s/%s", d.apiURL, addr.String(), contract), nil)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, common.ChainAgnosticContract{}, err
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}

	resultToken := d.poapToToken(token)
	resultContract := d.poapToContract(token)
	return []common.ChainAgnosticToken{resultToken}, resultContract, nil
}

// GetContractByAddress retrieves an Poap contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (common.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/events/%s", d.apiURL, addr), nil)
	if err != nil {
		return common.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return common.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return common.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var event poapEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return common.ChainAgnosticContract{}, err
	}
	return d.eventToContract(event), nil
}

func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	t, _, err := d.GetTokensByTokenIdentifiers(ctx, ti, 1, 0)
	if err != nil {
		return persist.TokenMetadata{}, err
	}
	if len(t) == 0 {
		return persist.TokenMetadata{}, fmt.Errorf("no token found for %s", ti)
	}

	return t[0].TokenMetadata, nil
}

// we should assume when using this function that the array is all of the tokens un paginated and we will need to paginate it with the offset and limit
func (d *Provider) poapsToTokens(pPoap []poapToken) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract) {
	tokens := make([]common.ChainAgnosticToken, 0, len(pPoap))
	contracts := make([]common.ChainAgnosticContract, 0, len(pPoap))
	for _, poap := range pPoap {
		tokens = append(tokens, d.poapToToken(poap))
		contracts = append(contracts, d.poapToContract(poap))
	}
	return tokens, contracts
}
func (d *Provider) poapToToken(pPoap poapToken) common.ChainAgnosticToken {

	return common.ChainAgnosticToken{
		OwnerAddress: persist.Address(pPoap.Owner),
		TokenID:      persist.HexTokenID(pPoap.TokenID.toBase16()),
		Descriptors: common.ChainAgnosticTokenDescriptors{
			Name:        pPoap.Event.Name,
			Description: pPoap.Event.Description,
		},
		Quantity:        "1",
		ExternalURL:     pPoap.Event.EventURL,
		ContractAddress: persist.Address(pPoap.Event.FancyID),
		TokenType:       persist.TokenTypeERC721,
		FallbackMedia: persist.FallbackMedia{
			ImageURL: persist.NullString(pPoap.Event.ImageURL),
		},
		TokenMetadata: persist.TokenMetadata{
			"event_id":    pPoap.Event.ID,
			"supply":      pPoap.Event.Supply,
			"event_url":   pPoap.Event.EventURL,
			"image_url":   pPoap.Event.ImageURL,
			"country":     pPoap.Event.Country,
			"city":        pPoap.Event.City,
			"description": pPoap.Event.Description,
			"year":        pPoap.Event.Year,
			"start_date":  pPoap.Event.StartDate,
			"end_date":    pPoap.Event.EndDate,
			"expiry_date": pPoap.Event.ExpiryDate,
			"name":        pPoap.Event.Name,
			"chain":       pPoap.Chain,
			"created":     pPoap.Created,
		},
	}
}

func (d *Provider) poapToContract(pPoap poapToken) common.ChainAgnosticContract {

	return common.ChainAgnosticContract{
		Address: persist.Address(pPoap.Event.FancyID),
		Descriptors: common.ChainAgnosticContractDescriptors{
			Name: pPoap.Event.Name,
		},
	}
}

func (d *Provider) eventToContract(pEvent poapEvent) common.ChainAgnosticContract {
	return common.ChainAgnosticContract{
		Address: persist.Address(pEvent.FancyID),
		Descriptors: common.ChainAgnosticContractDescriptors{
			Name: pEvent.Name,
		},
	}
}

func (t tokenID) toBigInt() *big.Int {
	i, _ := big.NewInt(0).SetString(string(t), 10)
	return i
}
func (t tokenID) toBase16() string {
	return t.toBigInt().Text(16)
}
