package reservoir

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

func init() {
	env.RegisterValidation("RESERVOIR_API_KEY", "required")
}

type TokenID string

func (t TokenID) String() string {
	return string(t)
}

func (t TokenID) ToTokenID() persist.TokenID {

	if strings.HasPrefix(t.String(), "0x") {
		big, ok := new(big.Int).SetString(strings.TrimPrefix(t.String(), "0x"), 16)
		if !ok {
			return ""
		}
		return persist.TokenID(big.Text(16))
	}
	big, ok := new(big.Int).SetString(t.String(), 10)
	if !ok {
		return ""
	}
	return persist.TokenID(big.Text(16))

}

type ErrTokenNotFoundByIdentifiers struct {
	ContractAddress persist.Address
	TokenID         TokenID
	OwnerAddress    persist.Address
}

func (e ErrTokenNotFoundByIdentifiers) Error() string {
	return fmt.Sprintf("token not found for contract %s, tokenID %s, owner %s", e.ContractAddress, e.TokenID, e.OwnerAddress)
}

/*
 {
      "token": {
        "contract": "string",
        "tokenId": "string",
        "kind": "string",
        "name": "string",
        "image": "string",
        "imageSmall": "string",
        "imageLarge": "string",
        "metadata": {},
        "supply": 0,
        "remainingSupply": 0,
        "rarityScore": 0,
        "rarityRank": 0,
        "media": "string",
        "collection": {
          "id": "string",
          "name": "string",
          "imageUrl": "string",
          "openseaVerificationStatus": "string",
        }
      },
      "ownership": {
        "tokenCount": "string",
        "acquiredAt": "string"
      }
    }
  ],
  "continuation": "string"
*/

type Token struct {
	Contract   persist.Address       `json:"contract"`
	TokenID    TokenID               `json:"tokenId"`
	Kind       string                `json:"kind"`
	Name       string                `json:"name"`
	Metadata   persist.TokenMetadata `json:"metadata"`
	Media      string                `json:"media"`
	Image      string                `json:"image"`
	ImageSmall string                `json:"imageSmall"`
	ImageLarge string                `json:"imageLarge"`
	Collection Collection            `json:"collection"`
}

type Collection struct {
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	ImageURL                  string `json:"imageUrl"`
	OpenseaVerificationStatus string `json:"openseaVerificationStatus"`
}

type Ownership struct {
	TokenCount string `json:"tokenCount"`
	AcquiredAt string `json:"acquiredAt"`
}

type TokenWithOwnership struct {
	Token     Token     `json:"token"`
	Ownership Ownership `json:"ownership"`
}

type UserTokensResponse struct {
	Tokens       []TokenWithOwnership `json:"tokens"`
	Continuation string               `json:"continuation"`
}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	chain      persist.Chain
	apiURL     string
	apiKey     string
	httpClient *http.Client
}

// NewProvider creates a new ethereum Provider
func NewProvider(chain persist.Chain, httpClient *http.Client) *Provider {
	var apiURL string
	switch chain {
	case persist.ChainETH:
		apiURL = "https://api.reservoir.tools"
	case persist.ChainOptimism:
		apiURL = "https://api-optimism.reservoir.tools"
	case persist.ChainPolygon:
		apiURL = "https://api-polygon.reservoir.tools"
	case persist.ChainArbitrum:
		apiURL = "https://api-arbitrum-nova.reservoir.tools"
	case persist.ChainZora:
		apiURL = "https://api-zora.reservoir.tools"
	case persist.ChainBase:
		apiURL = "https://api-base.reservoir.tools"
	}

	if apiURL == "" {
		panic(fmt.Sprintf("no reservoir api url set for chain %d", chain))
	}

	apiKey := env.GetString("RESERVOIR_API_KEY")

	return &Provider{
		apiURL:     apiURL,
		apiKey:     apiKey,
		chain:      chain,
		httpClient: httpClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	chainID := 0
	switch d.chain {
	case persist.ChainOptimism:
		chainID = 10
	case persist.ChainPolygon:
		chainID = 137
	case persist.ChainArbitrum:
		chainID = 42161
	case persist.ChainZora:
		chainID = 7777777
	case persist.ChainBase:
		chainID = 84531
	}
	return multichain.BlockchainInfo{
		Chain:   d.chain,
		ChainID: chainID,
	}, nil
}

func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tokens, err := d.getTokensByWalletAddressPaginate(ctx, addr, "")
	if err != nil {
		return nil, nil, err
	}
	t, c := tokensWithOwnershipToAgnosticTokens(tokens, addr)
	return t, c, nil
}

func (d *Provider) getTokensByWalletAddressPaginate(ctx context.Context, addr persist.Address, pageKey string) ([]TokenWithOwnership, error) {
	u := fmt.Sprintf("%s/users/%s/tokens/v7", d.apiURL, addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("x-api-key", d.apiKey)
	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 200))
	q.Add("sort_by", "acquiredAt")
	if pageKey != "" {
		q.Add("continuation", pageKey)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	var result UserTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	tokens := result.Tokens
	if len(tokens) == 200 && result.Continuation != "" {
		nextPageTokens, err := d.getTokensByWalletAddressPaginate(ctx, addr, result.Continuation)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, nextPageTokens...)
	}

	return tokens, nil
}
func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	u := fmt.Sprintf("%s/users/%s/tokens/v7", d.apiURL, ownerAddress)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	req.Header.Add("x-api-key", d.apiKey)
	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 1))
	q.Add("tokens", fmt.Sprintf("%s:%s", tokenIdentifiers.ContractAddress, tokenIdentifiers.TokenID.Base10String()))

	req.URL.RawQuery = q.Encode()

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	var result UserTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(result.Tokens) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrTokenNotFoundByIdentifiers{ContractAddress: tokenIdentifiers.ContractAddress, TokenID: TokenID(tokenIdentifiers.TokenID.Base10String()), OwnerAddress: ownerAddress}
	}

	t, c := tokensWithOwnershipToAgnosticTokens(result.Tokens, ownerAddress)
	if len(t) == 0 || len(c) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrTokenNotFoundByIdentifiers{ContractAddress: tokenIdentifiers.ContractAddress, TokenID: TokenID(tokenIdentifiers.TokenID.Base10String()), OwnerAddress: ownerAddress}
	}
	return t[0], c[0], nil
}

func tokensWithOwnershipToAgnosticTokens(tokens []TokenWithOwnership, ownerAddress persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	ts := []multichain.ChainAgnosticToken{}
	cs := []multichain.ChainAgnosticContract{}
	for _, t := range tokens {
		var tokenType persist.TokenType
		switch t.Token.Kind {
		case "erc721":
			tokenType = persist.TokenTypeERC721
		case "erc1155":
			tokenType = persist.TokenTypeERC1155
		case "erc20":
			tokenType = persist.TokenTypeERC20
		}
		var tokenQuantity persist.HexString
		b, ok := new(big.Int).SetString(t.Ownership.TokenCount, 10)
		if !ok {
			b, ok = new(big.Int).SetString(t.Ownership.TokenCount, 16)
			if !ok {
				tokenQuantity = persist.HexString("1")
			} else {
				tokenQuantity = persist.HexString(b.Text(16))
			}
		} else {
			tokenQuantity = persist.HexString(b.Text(16))
		}
		description, _ := t.Token.Metadata["description"].(string)
		ts = append(ts, multichain.ChainAgnosticToken{
			ContractAddress: t.Token.Contract,
			Descriptors: multichain.ChainAgnosticTokenDescriptors{
				Name:        t.Token.Name,
				Description: description,
			},
			TokenType:     tokenType,
			TokenID:       t.Token.TokenID.ToTokenID(),
			Quantity:      tokenQuantity,
			OwnerAddress:  ownerAddress,
			TokenMetadata: t.Token.Metadata,
			FallbackMedia: persist.FallbackMedia{
				ImageURL: persist.NullString(t.Token.Image),
			},
		})
		cs = append(cs, multichain.ChainAgnosticContract{
			Address: t.Token.Contract,
			Descriptors: multichain.ChainAgnosticContractDescriptors{
				Name:            t.Token.Collection.Name,
				ProfileImageURL: t.Token.Collection.ImageURL,
			},
		})
	}
	return ts, cs
}
