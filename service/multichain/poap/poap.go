package poap

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/mikeydub/go-gallery/service/multichain"
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

/*
{
  "limit": 10,
  "offset": 0,
  "total": 0,
  "tokens": [
    {
      "created": "string",
      "id": "string",
      "owner": {
        "id": "string",
        "tokensOwned": 0,
        "ens": "string"
      },
      "transferCount": "string"
    }
  ]
}
*/

type eventPoaps struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
	Tokens []struct {
		ID    string `json:"id"`
		Owner struct {
			ID  string `json:"id"`
			ENS string `json:"ens"`
		} `json:"owner"`
	}
}

// Provider is an the struct for retrieving data from the Tezos blockchain
type Provider struct {
	apiURL     string
	apiKey     string
	authToken  string
	httpClient *http.Client
}

// NewProvider creates a new Tezos Provider
func NewProvider(httpClient *http.Client, apiKey string, authToken string) *Provider {
	return &Provider{
		apiURL:     "https://api.poap.tech",
		apiKey:     apiKey,
		authToken:  authToken,
		httpClient: httpClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainPOAP,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Poap Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

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

	resultTokens, resultContracts := d.poapsToTokens(tokens, limit, offset)
	return resultTokens, resultContracts, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Poap Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("poap has no way to retrieve tokens by contract address")
}

// GetTokensByContractAddressAndOwner retrieves tokens for a contract address with owner on the Poap Blockchain
func (d *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, owner, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("poap has no way to retrieve tokens by contract address")
}

func (d *Provider) GetCommunityOwners(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticCommunityOwner, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/events/%s", d.apiURL, contractAddress), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, err
	}
	var event poapEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, err
	}
	nextReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/events/%d/poaps&limit=%d&offset=%d", d.apiURL, event.ID, limit, offset), nil)
	if err != nil {
		return nil, err
	}
	nextReq.Header.Set("X-API-KEY", d.apiKey)
	nextReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.authToken))
	nextResp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer nextResp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, err
	}
	var eventPoaps eventPoaps
	if err := json.NewDecoder(nextResp.Body).Decode(&eventPoaps); err != nil {
		return nil, err
	}
	var owners []multichain.ChainAgnosticCommunityOwner
	for _, token := range eventPoaps.Tokens {
		owners = append(owners, multichain.ChainAgnosticCommunityOwner{
			Address: persist.Address(token.Owner.ID), // TODO is this the address?
		})
	}
	return owners, nil
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Poap Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	tid := tokenIdentifiers.TokenID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/tokens/%s", d.apiURL, tid), nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	return []multichain.ChainAgnosticToken{d.poapToToken(token)}, d.poapToContract(token), nil
}

// GetTokensByTokenIdentifiersAndOwner retrieves tokens for a token identifiers and owner address
func (d *Provider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	tid := tokenIdentifiers.TokenID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/tokens/%s", d.apiURL, tid), nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	return d.poapToToken(token), d.poapToContract(token), nil
}

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contract persist.Address, addr persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/actions/scan/%s/%s", d.apiURL, addr.String(), contract), nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	resultToken := d.poapToToken(token)
	resultContract := d.poapToContract(token)
	return []multichain.ChainAgnosticToken{resultToken}, resultContract, nil
}

// GetContractByAddress retrieves an Poap contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/events/%s", d.apiURL, addr), nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	req.Header.Set("X-API-KEY", d.apiKey)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var event poapEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return d.eventToContract(event), nil
}

func (d *Provider) GetDisplayNameByAddress(ctx context.Context, addr persist.Address) string {
	return addr.String()
}

// we should assume when using this function that the array is all of the tokens un paginated and we will need to paginate it with the offset and limit
func (d *Provider) poapsToTokens(pPoap []poapToken, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	tokens := make([]multichain.ChainAgnosticToken, 0, len(pPoap))
	contracts := make([]multichain.ChainAgnosticContract, 0, len(pPoap))
	if limit < 1 {
		limit = len(pPoap)
	}
	for i, poap := range pPoap {
		if i < offset {
			continue
		}
		tokens = append(tokens, d.poapToToken(poap))
		contracts = append(contracts, d.poapToContract(poap))
		if i >= (offset + limit) {
			break
		}
	}
	return tokens, contracts
}
func (d *Provider) poapToToken(pPoap poapToken) multichain.ChainAgnosticToken {

	return multichain.ChainAgnosticToken{
		OwnerAddress:    persist.Address(pPoap.Owner),
		TokenID:         persist.TokenID(pPoap.TokenID.toBase16()),
		Name:            pPoap.Event.Name,
		Description:     pPoap.Event.Description,
		Quantity:        "1",
		ExternalURL:     pPoap.Event.EventURL,
		ContractAddress: persist.Address(pPoap.Event.FancyID),
		TokenType:       persist.TokenTypeERC721,
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

func (d *Provider) poapToContract(pPoap poapToken) multichain.ChainAgnosticContract {

	return multichain.ChainAgnosticContract{
		Address: persist.Address(pPoap.Event.FancyID),
		Name:    pPoap.Event.Name,
	}
}

func (d *Provider) eventToContract(pEvent poapEvent) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address: persist.Address(pEvent.FancyID),
		Name:    pEvent.Name,
	}
}

func (t tokenID) toBigInt() *big.Int {
	i, _ := big.NewInt(0).SetString(string(t), 10)
	return i
}
func (t tokenID) toBase16() string {
	return t.toBigInt().Text(16)
}
