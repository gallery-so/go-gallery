package infura

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
)

func init() {
	env.RegisterValidation("INFURA_API_KEY", "required")
	env.RegisterValidation("INFURA_API_SECRET", "required")
}

type tokensPaginated interface {
	GetTokensFromResponse(resp *http.Response) ([]Token, error)
	GetNextPageKey() string
}

type TokenID string

func (t TokenID) String() string {
	return string(t)
}

func (t TokenID) ToTokenID() persist.TokenID {

	big, ok := new(big.Int).SetString(t.String(), 10)
	if !ok {
		return ""
	}
	return persist.TokenID(big.Text(16))

}

type Token struct {
	Contract  persist.EthereumAddress `json:"contract"`
	TokenID   TokenID                 `json:"token_id"`
	Supply    string                  `json:"supply"`
	TokenType string                  `json:"type"`
	Metadata  persist.TokenMetadata   `json:"metadata"`
}

type getNFTsForOwnerResponse struct {
	Cursor string  `json:"cursor"`
	Assets []Token `json:"assets"`
}

func (r *getNFTsForOwnerResponse) GetTokensFromResponse(resp *http.Response) ([]Token, error) {
	r.Assets = nil
	if err := json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, err
	}
	return r.Assets, nil
}

func (r *getNFTsForOwnerResponse) GetNextPageKey() string {
	return r.Cursor
}

// example request URL https://nft.api.infura.io/networks/1/accounts/0x0a267cf51ef038fc00e71801f5a524aec06e4f07/assets/nfts

const baseURL = "https://nft.api.infura.io/networks/1"

const (
	pageSize = 100
)

type Provider struct {
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

func NewProvider(httpClient *http.Client) *Provider {
	return &Provider{
		apiKey:     env.GetString("INFURA_API_KEY"),
		apiSecret:  env.GetString("INFURA_API_SECRET"),
		httpClient: httpClient,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainETH,
		ChainID: 0,
	}, nil
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	panic("implement me")
}

func (p *Provider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tids multichain.ChainAgnosticIdentifiers, owner persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	panic("implement me")
}

func getNFTsPaginate[T tokensPaginated](ctx context.Context, startingURL string, limit, offset int, pageKey string, httpClient *http.Client, key, secret string, result T) ([]Token, error) {

	tokens := []Token{}
	url := startingURL

	if pageKey != "" {
		url = fmt.Sprintf("%s&cursor=%s", url, pageKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(key, secret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get tokens from alchemy api: %s", resp.Status)
	}

	newTokens, err := result.GetTokensFromResponse(resp)
	if err != nil {
		return nil, err
	}

	if offset > 0 && offset < pageSize {
		if len(newTokens) > offset {
			newTokens = newTokens[offset:]
		} else {
			newTokens = nil
		}
	}

	if limit > 0 && limit < pageSize {
		if len(newTokens) > limit {
			newTokens = newTokens[:limit]
		}
	}

	tokens = append(tokens, newTokens...)

	if result.GetNextPageKey() != "" && result.GetNextPageKey() != pageKey {

		if limit > 0 {
			limit -= pageSize
		}
		if offset > 0 {
			offset -= pageSize
		}
		newTokens, err := getNFTsPaginate(ctx, startingURL, limit, offset, result.GetNextPageKey(), httpClient, key, secret, result)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, newTokens...)
	}

	return tokens, nil
}

type ContractMetadata struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}

func (p *Provider) getContractMetadata(ctx context.Context, contract persist.EthereumAddress) (multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/nfts/%s", baseURL, contract)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	req.SetBasicAuth(p.apiKey, p.apiSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get tokens from infura api: %s", resp.Status)
	}

	var contractMetadata ContractMetadata
	if err := json.NewDecoder(resp.Body).Decode(&contractMetadata); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	chainAgnosticContract := multichain.ChainAgnosticContract{
		Address: persist.Address(contract),
		Symbol:  contractMetadata.Symbol,
		Name:    contractMetadata.Name,
	}

	return chainAgnosticContract, nil
}

func (p *Provider) ownedTokensToChainAgnosticTokens(ctx context.Context, owner persist.Address, tokens []Token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

	chainAgnosticTokens := []multichain.ChainAgnosticToken{}
	chainAgnosticContracts := []multichain.ChainAgnosticContract{}

	seenContracts := map[persist.EthereumAddress]bool{}

	for _, token := range tokens {

		newToken := ownedTokenToChainAgnosticToken(owner, token)

		if !seenContracts[token.Contract] {
			chainAgnosticContract, err := p.getContractMetadata(ctx, token.Contract)
			if err != nil {
				return nil, nil, err
			}
			chainAgnosticContracts = append(chainAgnosticContracts, chainAgnosticContract)
			seenContracts[token.Contract] = true
		}

		chainAgnosticTokens = append(chainAgnosticTokens, newToken)
	}

	return chainAgnosticTokens, chainAgnosticContracts, nil
}

func ownedTokenToChainAgnosticToken(owner persist.Address, token Token) multichain.ChainAgnosticToken {
	var tokenType persist.TokenType

	switch token.TokenType {
	case "ERC721":
		tokenType = persist.TokenTypeERC721
	case "ERC1155":
		tokenType = persist.TokenTypeERC1155
	}

	b, ok := new(big.Int).SetString(token.Supply, 10)
	if !ok {
		b = big.NewInt(1)
	}

	chainAgnosticToken := multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		TokenMetadata:   token.Metadata,
		TokenID:         token.TokenID.ToTokenID(),
		OwnerAddress:    owner,
		ContractAddress: persist.Address(token.Contract),
		Quantity:        persist.HexString(b.Text(16)),
	}

	return chainAgnosticToken
}
