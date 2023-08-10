package infura

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
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

type Metadata map[string]any

func (m *Metadata) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || strings.EqualFold(string(b), "null") {
		return nil
	}

	var s string
	var newM map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		if err := json.Unmarshal(b, &newM); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w (%s)", err, string(b))
		}
	} else {
		if err := json.Unmarshal([]byte(s), &newM); err != nil {
			return fmt.Errorf("failed to unmarshal metadata from string: %w (%s) (%s)", err, s, string(b))
		}
	}
	*m = newM

	return nil
}

type Token struct {
	Contract  persist.EthereumAddress `json:"contract"`
	TokenID   TokenID                 `json:"tokenId"`
	Supply    string                  `json:"supply"`
	TokenType string                  `json:"type"`
	Metadata  Metadata                `json:"metadata"`
	Name      string                  `json:"name"`
}

type getNFTsForOwnerResponse struct {
	Cursor string  `json:"cursor"`
	Assets []Token `json:"assets"`
}

type getOwnersResponse struct {
	PageSize int     `json:"pageSize"`
	Cursor   string  `json:"cursor"`
	Owners   []Owner `json:"owners"`
}

type Owner struct {
	TokenAddress persist.EthereumAddress `json:"tokenAddress"`
	TokenID      TokenID                 `json:"tokenId"`
	Amount       string                  `json:"amount"`
	OwnerOf      persist.EthereumAddress `json:"ownerOf"`
	ContractType string                  `json:"contractType"`
	Name         string                  `json:"name"`
	Symbol       string                  `json:"symbol"`
	Metadata     Metadata                `json:"metadata"`
}

func (r *getNFTsForOwnerResponse) GetTokensFromResponse(resp *http.Response) ([]Token, error) {
	r.Assets = nil
	r.Cursor = ""
	if err := json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, err
	}
	return r.Assets, nil
}

func (r *getNFTsForOwnerResponse) GetNextPageKey() string {
	return url.QueryEscape(r.Cursor)
}

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
func (d *Provider) GetBlockchainInfo() multichain.BlockchainInfo {
	return multichain.BlockchainInfo{
		Chain:      persist.ChainETH,
		ChainID:    0,
		ProviderID: "infura",
	}
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tokens, err := getNFTsPaginate(ctx, fmt.Sprintf("%s/accounts/%s/assets/nfts", baseURL, address), limit, offset, "", p.httpClient, p.apiKey, p.apiSecret, &getNFTsForOwnerResponse{})
	if err != nil {
		return nil, nil, err
	}

	if len(tokens) == 0 {
		return nil, nil, nil
	}

	return p.ownedTokensToChainAgnosticTokens(ctx, address, tokens)
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tids multichain.ChainAgnosticIdentifiers, owner persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	owners, err := p.getOwnersPaginate(ctx, tids, "")
	if err != nil {

		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(owners) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no owners found for token %s with owner %s", tids, owner)
	}

	tokens, contracts, err := p.ownersToTokensForOwner(ctx, owner, owners)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(tokens) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens found for owner %s with tids %s", owner, tids)
	}

	return tokens[0], contracts[0], nil
}

func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tids multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	owners, err := p.getOwnersPaginate(ctx, tids, "")
	if err != nil {

		return nil, multichain.ChainAgnosticContract{}, err
	}

	if len(owners) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no owners found for token %s", tids)
	}

	tokens, contracts, err := p.ownersToTokens(ctx, owners)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	if len(tokens) == 0 || len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens or contracts found for token with tids %s", tids)
	}

	return tokens, contracts[0], nil
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, tids multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	owners, err := p.getOwnersPaginate(ctx, tids, "")
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}

	if len(owners) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, fmt.Errorf("no owners found for token %s", tids)
	}

	tokens, contracts, err := p.ownersToTokensForOwner(ctx, persist.Address(owners[0].OwnerOf), []Owner{owners[0]})
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}

	if len(tokens) == 0 || len(contracts) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, fmt.Errorf("no tokens or contracts found for token with tids %s", tids)
	}

	return tokens[0].Descriptors, contracts[0].Descriptors, nil
}

func (d *Provider) getOwnersPaginate(ctx context.Context, tids multichain.ChainAgnosticIdentifiers, pageKey string) ([]Owner, error) {

	owners := []Owner{}

	url := fmt.Sprintf("%s/nfts/%s/%s/owners", baseURL, tids.ContractAddress, tids.TokenID.Base10String())

	if pageKey != "" {
		url = fmt.Sprintf("%s?cursor=%s", url, pageKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(d.apiKey, d.apiSecret)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get owners from infura api: %s", resp.Status)
	}

	ownersResp := getOwnersResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&ownersResp); err != nil {
		return nil, err
	}

	owners = append(owners, ownersResp.Owners...)

	if ownersResp.Cursor != "" && ownersResp.Cursor != pageKey {

		newOwners, err := d.getOwnersPaginate(ctx, tids, ownersResp.Cursor)
		if err != nil {
			return nil, err
		}

		owners = append(owners, newOwners...)
	}

	return owners, nil
}

func getNFTsPaginate[T tokensPaginated](ctx context.Context, startingURL string, limit, offset int, pageKey string, httpClient *http.Client, key, secret string, result T) ([]Token, error) {

	tokens := []Token{}
	u := startingURL

	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	q := parsedURL.Query()

	if pageKey != "" {
		q.Set("cursor", pageKey)
	}

	parsedURL.RawQuery = q.Encode()

	u = parsedURL.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		return nil, fmt.Errorf("failed to get tokens from infura api: %s (%s)", resp.Status, u)
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

	next := result.GetNextPageKey()

	if next != "" && next != pageKey {

		if limit > 0 {
			limit -= pageSize
		}
		if offset > 0 {
			offset -= pageSize
		}
		newTokens, err := getNFTsPaginate(ctx, startingURL, limit, offset, next, httpClient, key, secret, result)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, newTokens...)
	}

	return tokens, nil
}

type TokenMetadata struct {
	Contract persist.Address `json:"contract"`
	TokenID  persist.TokenID `json:"token_id"`
	Metadata Metadata        `json:"metadata"`
}

// GetTokenMetadataByTokenIdentifiers retrieves a token's metadata for a given contract address and token ID
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	meta, err := p.getMetadata(ctx, ti, true)
	if err != nil {
		logger.For(ctx).Errorf("failed to get metadata for token %s: %s", ti.TokenID, err.Error())
		return p.getMetadata(ctx, ti, false)
	}
	return meta, nil

}

func (p *Provider) getMetadata(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, refresh bool) (persist.TokenMetadata, error) {
	if refresh {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	url := fmt.Sprintf("%s/nfts/%s/tokens/%s?resyncMetadata=%t", baseURL, ti.ContractAddress, ti.TokenID.Base10String(), refresh)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return persist.TokenMetadata{}, err
	}

	req.SetBasicAuth(p.apiKey, p.apiSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return persist.TokenMetadata{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return persist.TokenMetadata{}, fmt.Errorf("failed to get token metadata from infura api: %s", resp.Status)
	}

	tokenMetadata := TokenMetadata{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenMetadata); err != nil {
		return persist.TokenMetadata{}, err
	}

	return persist.TokenMetadata(tokenMetadata.Metadata), nil
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
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol: contractMetadata.Symbol,
			Name:   contractMetadata.Name,
		},
	}

	return chainAgnosticContract, nil
}

func (p *Provider) ownersToTokensForOwner(ctx context.Context, owner persist.Address, owners []Owner) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	result := Token{}
	found := false
	for _, o := range owners {
		if strings.EqualFold(owner.String(), o.OwnerOf.String()) {
			result = Token{
				Contract:  o.TokenAddress,
				TokenID:   o.TokenID,
				Supply:    o.Amount,
				TokenType: o.ContractType,
				Metadata:  o.Metadata,
				Name:      o.Name,
			}
			found = true
			break
		}
	}

	if !found {
		return nil, nil, fmt.Errorf("owner %s not found", owner.String())
	}

	return p.ownedTokensToChainAgnosticTokens(ctx, owner, []Token{result})
}

func (p *Provider) ownersToTokens(ctx context.Context, owners []Owner) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	result := map[string]Token{}

	for _, o := range owners {

		result[o.OwnerOf.String()] = Token{
			Contract:  o.TokenAddress,
			TokenID:   o.TokenID,
			Supply:    o.Amount,
			TokenType: o.ContractType,
			Metadata:  o.Metadata,
			Name:      o.Name,
		}

	}

	agnosticTokens := []multichain.ChainAgnosticToken{}
	agnosticContracts := []multichain.ChainAgnosticContract{}
	seenContracts := map[persist.Address]bool{}
	for owner, token := range result {
		a, c, err := p.ownedTokensToChainAgnosticTokens(ctx, persist.Address(strings.ToLower(owner)), []Token{token})
		if err != nil {
			return nil, nil, err
		}
		for _, contract := range c {
			if !seenContracts[contract.Address] {
				agnosticContracts = append(agnosticContracts, contract)
				seenContracts[contract.Address] = true
			}
		}
		agnosticTokens = append(agnosticTokens, a...)
	}
	return agnosticTokens, agnosticContracts, nil
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
		TokenMetadata:   persist.TokenMetadata(token.Metadata),
		TokenID:         token.TokenID.ToTokenID(),
		OwnerAddress:    owner,
		ContractAddress: persist.Address(token.Contract),
		Quantity:        persist.HexString(b.Text(16)),
		Descriptors: multichain.ChainAgnosticTokenDescriptors{
			Name: token.Name,
		},
	}

	return chainAgnosticToken
}
