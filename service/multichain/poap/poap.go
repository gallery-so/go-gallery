package poap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

const poapAddress = "0x22c1f6050e56d2876009903609a2cc3fef83b415"

var poapContract = multichain.ChainAgnosticContract{
	Address: poapAddress,
	Symbol:  "The Proof of Attendance Protocol",
	Name:    "POAP",
}

var errNoContracts = errors.New("only one contract exists for POAP and all tokens live on this one contract")

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
		Description string `json:"description"`
		Supply      int    `json:"supply"`
	} `json:"event"`
	TokenID tokenID `json:"tokenId"`
	Owner   string  `json:"owner"`
	Chain   string  `json:"chain"`
}

// Provider is an the struct for retrieving data from the Tezos blockchain
type Provider struct {
	apiURL     string
	apiKey     string
	httpClient *http.Client
}

// NewProvider creates a new Tezos Provider
func NewProvider(httpClient *http.Client, apiKey string) *Provider {
	return &Provider{
		apiURL:     "https://api.poap.tech",
		apiKey:     apiKey,
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
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/actions/scan/%s", d.apiURL, addr.String()), nil)
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

	return d.poapsToTokens(tokens), []multichain.ChainAgnosticContract{poapContract}, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Poap Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return nil, multichain.ChainAgnosticContract{}, errNoContracts
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Poap Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tid := tokenIdentifiers.TokenID
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/tokens/%s", d.apiURL, tid), nil)
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
	var token poapToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, nil, err
	}

	return []multichain.ChainAgnosticToken{d.poapToToken(token)}, []multichain.ChainAgnosticContract{poapContract}, nil
}

// GetContractByAddress retrieves an Poap contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	return poapContract, nil
}

// RefreshToken refreshes the metadata for a given token.
func (d *Provider) RefreshToken(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, owner persist.Address) error {
	return nil
}

// UpdateMediaForWallet updates media for the tokens owned by a wallet on the Poap Blockchain
func (d *Provider) UpdateMediaForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	return nil
}

// RefreshContract refreshes the metadata for a contract
func (d *Provider) RefreshContract(ctx context.Context, addr persist.Address) error {
	return nil
}

// ValidateTokensForWallet validates tokens for a wallet address on the Poap Blockchain
func (d *Provider) ValidateTokensForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	return nil

}

// VerifySignature does nothing because POAPs are not really on a blockchain with a wallet
func (d *Provider) VerifySignature(pCtx context.Context, pPubKey persist.PubKey, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {
	return true, nil
}

func (d *Provider) poapsToTokens(pPoap []poapToken) []multichain.ChainAgnosticToken {
	result := make([]multichain.ChainAgnosticToken, 0, len(pPoap))
	for _, poap := range pPoap {
		result = append(result, d.poapToToken(poap))
	}
	return result
}
func (d *Provider) poapToToken(pPoap poapToken) multichain.ChainAgnosticToken {

	return multichain.ChainAgnosticToken{
		OwnerAddress:    persist.Address(pPoap.Owner),
		TokenID:         persist.TokenID(pPoap.TokenID.toBase16()),
		Name:            pPoap.Event.Name,
		Description:     pPoap.Event.Description,
		Quantity:        "1",
		ExternalURL:     pPoap.Event.EventURL,
		ContractAddress: poapAddress,
		Media: persist.Media{
			MediaType: persist.MediaTypeImage,
			MediaURL:  persist.NullString(pPoap.Event.ImageURL),
		},
		TokenType: persist.TokenTypeERC721,
		TokenMetadata: persist.TokenMetadata{
			"supply":      pPoap.Event.Supply,
			"event_url":   pPoap.Event.EventURL,
			"image_url":   pPoap.Event.ImageURL,
			"description": pPoap.Event.Description,
			"name":        pPoap.Event.Name,
			"chain":       pPoap.Chain,
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
