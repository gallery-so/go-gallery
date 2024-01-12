package alchemy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/util"
)

type TokenURI struct {
	Gateway string `json:"gateway"`
	Raw     string `json:"raw"`
}

type Media struct {
	Raw       string `json:"raw"`
	Gateway   string `json:"gateway"`
	Thumbnail string `json:"thumbnail"`
	Format    string `json:"format"`
	Bytes     int    `json:"bytes"`
}

type OpenseaCollection struct {
	CollectionName string `json:"collectionName"`
	Description    string `json:"description"`
	ImageURL       string `json:"imageUrl"`
}

type ContractMetadata struct {
	Name              string                  `json:"name"`
	Symbol            string                  `json:"symbol"`
	TotalSupply       string                  `json:"totalSupply"`
	TokenType         string                  `json:"tokenType"`
	ContractDeployer  persist.EthereumAddress `json:"contractDeployer"`
	OpenseaCollection OpenseaCollection       `json:"openSea"`
}

type Contract struct {
	Address          string                  `json:"address"`
	Title            string                  `json:"title"`
	ContractDeployer persist.EthereumAddress `json:"contractDeployer"`
	Opensea          OpenseaCollection       `json:"openSea"`
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

type TokenMetadata struct {
	TokenType string `json:"tokenType"`
}

type TokenIdentifiers struct {
	TokenID       TokenID          `json:"tokenId"`
	TokenMetadata ContractMetadata `json:"tokenMetadata"`
}

type SpamInfo struct {
	IsSpam string `json:"isSpam"`
}

type Token struct {
	Contract         Contract              `json:"contract"`
	ID               TokenIdentifiers      `json:"id"`
	Balance          string                `json:"balance"`
	Title            string                `json:"title"`
	Description      string                `json:"description"`
	TokenURI         TokenURI              `json:"tokenUri"`
	Media            []Media               `json:"media"`
	Metadata         persist.TokenMetadata `json:"metadata"`
	ContractMetadata ContractMetadata      `json:"contractMetadata"`
	TimeLastUpdated  time.Time             `json:"timeLastUpdated"`
	SpamInfo         SpamInfo              `json:"spamInfo"`
}

type OwnerWithBalances struct {
	OwnerAddress  persist.EthereumAddress `json:"ownerAddress"`
	TokenBalances []TokenBalance          `json:"tokenBalances"`
}

type TokenBalance struct {
	TokenID TokenID `json:"tokenId"`
	Balance int     `json:"balance"`
}

type tokensPaginated interface {
	GetTokensFromResponse(resp *http.Response) ([]Token, error)
	GetNextPageKey() string
}

type getNFTsResponse struct {
	OwnedNFTs  []Token `json:"ownedNfts"`
	PageKey    string  `json:"pageKey"`
	TotalCount int     `json:"totalCount"`
}

func (r *getNFTsResponse) GetTokensFromResponse(resp *http.Response) ([]Token, error) {
	r.OwnedNFTs = nil
	if err := json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (%s)", err, resp.Request.URL)
	}
	return r.OwnedNFTs, nil
}

func (r getNFTsResponse) GetNextPageKey() string {
	return r.PageKey
}

type getNFTsForCollectionResponse struct {
	NFTs      []Token `json:"nfts"`
	NextToken TokenID `json:"nextToken"`
}

func (r *getNFTsForCollectionResponse) GetTokensFromResponse(resp *http.Response) ([]Token, error) {
	r.NFTs = nil

	if err := json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, err
	}
	return r.NFTs, nil
}

func (r getNFTsForCollectionResponse) GetNextPageKey() string {
	return r.NextToken.String()
}

type getOwnersOfCollectionResponse struct {
	Owners []OwnerWithBalances `json:"ownerAddresses"`
}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	chain         persist.Chain
	alchemyAPIURL string
	httpClient    *http.Client
	cache         *redis.Cache
}

// NewProvider creates a new ethereum Provider
func NewProvider(chain persist.Chain, httpClient *http.Client, cache *redis.Cache) *Provider {
	// currently using v2 endpoints, alchemy recently added v3
	var apiURL string
	switch chain {
	case persist.ChainETH:
		apiURL = env.GetString("ALCHEMY_API_URL")
	case persist.ChainOptimism:
		apiURL = env.GetString("ALCHEMY_OPTIMISM_API_URL")
	case persist.ChainPolygon:
		apiURL = env.GetString("ALCHEMY_POLYGON_API_URL")
	case persist.ChainArbitrum:
		apiURL = env.GetString("ALCHEMY_ARBITRUM_API_URL")
	}

	if apiURL == "" {
		panic(fmt.Sprintf("no alchemy api url set for chain %d", chain))
	}

	return &Provider{
		alchemyAPIURL: apiURL,
		chain:         chain,
		httpClient:    httpClient,
		cache:         cache,
	}
}

func (d *Provider) ProviderInfo() multichain.ProviderInfo {
	return multichain.ProviderInfo{
		Chain:      d.chain,
		ChainID:    persist.MustChainToChainID(d.chain),
		ProviderID: "alchemy",
	}
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTs?owner=%s&withMetadata=true", d.alchemyAPIURL, addr)
	if d.chain == persist.ChainPolygon {
		url += "&excludeFilters[]=SPAM"
	}
	tokens, err := getNFTsPaginate(ctx, url, 100, "pageKey", 0, 0, "", d.httpClient, nil, &getNFTsResponse{})
	if err != nil {
		return nil, nil, err
	}

	cTokens, cContracts := alchemyTokensToChainAgnosticTokensForOwner(persist.EthereumAddress(addr), tokens)

	return cTokens, cContracts, d.cacheMetadatasForTokens(ctx, cTokens...)
}

// GetTokensIncrementallyByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)

	url := fmt.Sprintf("%s/getNFTs?owner=%s&withMetadata=true", d.alchemyAPIURL, addr)
	if d.chain == persist.ChainPolygon {
		url += "&excludeFilters[]=SPAM"
	}
	alchemyRec := make(chan []Token)
	subErrChan := make(chan error)

	go func() {
		defer close(alchemyRec)
		_, err := getNFTsPaginate(ctx, url, 100, "pageKey", 0, 0, "", d.httpClient, alchemyRec, &getNFTsResponse{})
		if err != nil {
			subErrChan <- err
			return
		}
	}()

	go func() {
		defer close(rec)
	outer:
		for {
			select {
			case err := <-subErrChan:
				errChan <- err
				return
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case tokens, ok := <-alchemyRec:
				if !ok {
					break outer
				}
				cTokens, cContracts := alchemyTokensToChainAgnosticTokensForOwner(persist.EthereumAddress(addr), tokens)
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    cTokens,
					Contracts: cContracts,
				}
			}
		}
	}()

	return rec, errChan
}

// GetTokensIncrementallyByContractAddress retrieves tokens incrementaly for a contract address on the Ethereum Blockchain
func (d *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, addr persist.Address, limit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)

	url := fmt.Sprintf("%s/getNFTsForCollection?contractAddress=%s&withMetadata=true", d.alchemyAPIURL, addr)
	if d.chain == persist.ChainPolygon {
		url += "&excludeFilters[]=SPAM"
	}
	alchemyRec := make(chan []Token)
	subErrChan := make(chan error)

	go func() {
		defer close(alchemyRec)
		_, err := getNFTsPaginate(ctx, url, 100, "startToken", limit, 0, "", d.httpClient, alchemyRec, &getNFTsForCollectionResponse{})
		if err != nil {
			subErrChan <- err
			return
		}
	}()

	go func() {
		defer close(rec)
		for {
			select {
			case err := <-subErrChan:
				errChan <- err
				return
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case tokens, ok := <-alchemyRec:
				if !ok {
					return
				}
				cTokens, cContracts, err := d.alchemyContractTokensToChainAgnosticTokens(ctx, addr, tokens)
				if err != nil {
					errChan <- err
					return
				}
				if len(cContracts) == 0 {
					errChan <- fmt.Errorf("no contract found for contract address %s", addr)
					return
				}
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    cTokens,
					Contracts: cContracts,
				}
			}
		}
	}()

	return rec, errChan
}

func getNFTsPaginate[T tokensPaginated](ctx context.Context, baseURL string, defaultLimit int, pageKeyName string, limit, offset int, pageKey string, httpClient *http.Client, rec chan<- []Token, result T) ([]Token, error) {

	tokens := []Token{}
	u := baseURL

	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	q := parsedURL.Query()

	if pageKey != "" && pageKeyName != "" {
		q.Set(pageKeyName, pageKey)
	}

	parsedURL.RawQuery = q.Encode()
	u = parsedURL.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokens from alchemy api: %w (url: %s)", err, u)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		asString, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get tokens from alchemy api: %s (err: %s) (url: %s)", resp.Status, asString, u)
	}

	newTokens, err := result.GetTokensFromResponse(resp)
	if err != nil {
		return nil, err
	}

	nextPageKey := result.GetNextPageKey()

	logger.For(ctx).Infof("got %d tokens for (cur page: %s, next page %s)", len(newTokens), pageKey, nextPageKey)

	if offset > 0 && offset < defaultLimit {
		if len(newTokens) > offset {
			newTokens = newTokens[offset:]
		} else {
			newTokens = nil
		}
	}

	if limit > 0 && limit < defaultLimit {
		if len(newTokens) > limit {
			newTokens = newTokens[:limit]
		}
	}

	if rec != nil {
		rec <- newTokens
	}

	tokens = append(tokens, newTokens...)

	if nextPageKey != "" && nextPageKey != pageKey {

		if limit > 0 {
			limit -= defaultLimit
		}
		if offset > 0 {
			offset -= defaultLimit
		}
		newTokens, err := getNFTsPaginate(ctx, baseURL, defaultLimit, pageKeyName, limit, offset, nextPageKey, httpClient, rec, result)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, newTokens...)
	}

	return tokens, nil
}

func (d *Provider) getOwnersForContract(ctx context.Context, contract persist.EthereumAddress) ([]OwnerWithBalances, error) {

	u := d.alchemyAPIURL + "/getOwnersForCollection"

	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	q := parsedURL.Query()

	q.Set("contractAddress", contract.String())
	q.Set("withTokenBalances", "true")

	parsedURL.RawQuery = q.Encode()
	u = parsedURL.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		asString, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get tokens from alchemy api: %s (err: %s) (url: %s)", resp.Status, asString, u)
	}

	result := &getOwnersOfCollectionResponse{}
	err = json.NewDecoder(resp.Body).Decode(result)
	if err != nil {
		return nil, err
	}

	return result.Owners, nil
}

func (d *Provider) getTokenWithMetadata(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, forceRefresh bool, timeout time.Duration) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	if timeout == 0 {
		timeout = (time.Second * 20) / time.Millisecond
	}
	url := fmt.Sprintf("%s/getNFTMetadata?contractAddress=%s&tokenId=%s&tokenUriTimeoutInMs=%d&refreshCache=%t", d.alchemyAPIURL, ti.ContractAddress, ti.TokenID.Base10String(), timeout, forceRefresh)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get token metadata from alchemy api: %w (url: %s)", err, url)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := util.GetErrFromResp(resp)
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get token metadata from alchemy api: %s (%w)", resp.Status, err)
	}

	// will have most of the fields empty
	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to decode token metadata response: %w (url: %s)", err, url)
	}

	tokens, contracts, err := d.alchemyTokensToChainAgnosticTokens(ctx, []Token{token})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to convert token to chain agnostic token: %w (url: %s)", err, url)
	}

	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get contracts from alchemy api")
	}

	return tokens, contracts[0], nil
}

func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	if d.chain == persist.ChainETH {
		// don't use alchemy for ETH
		return nil, fmt.Errorf("not implemented")
	}

	// cached, err := d.fetchMetadataFromCache(ctx, ti)
	// if cached != nil && err == nil {
	// 	logger.For(ctx).Infof("got cached metadata for %s", ti)
	// 	return cached, nil
	// }

	logger.For(ctx).Infof("no cached metadata for %s", ti)

	tokens, _, err := d.getTokenWithMetadata(ctx, ti, true, 0)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, nil
	}

	// return tokens[0].TokenMetadata, d.cacheMetadatasForTokens(ctx, tokens...)
	return tokens[0].TokenMetadata, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Ethereum Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTsForCollection?contractAddress=%s&withMetadata=true&tokenUriTimeoutInMs=20000", d.alchemyAPIURL, contractAddress)
	tokens, err := getNFTsPaginate(ctx, url, 100, "startToken", limit, offset, "", d.httpClient, nil, &getNFTsForCollectionResponse{})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	cTokens, cContracts, err := d.alchemyContractTokensToChainAgnosticTokens(ctx, contractAddress, tokens)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(cContracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for contract address %s", contractAddress)
	}

	return cTokens, cContracts[0], d.cacheMetadatasForTokens(ctx, cTokens...)
}

func (d *Provider) alchemyContractTokensToChainAgnosticTokens(ctx context.Context, contractAddress persist.Address, tokens []Token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	owners, err := d.getOwnersForContract(ctx, persist.EthereumAddress(contractAddress))
	if err != nil {
		return nil, nil, err
	}

	tokenIDToOwner := make(map[TokenID][]persist.EthereumAddress)
	for _, owner := range owners {
		for _, tokenID := range owner.TokenBalances {
			tokenIDToOwner[tokenID.TokenID] = append(tokenIDToOwner[tokenID.TokenID], owner.OwnerAddress)
		}
	}

	ownersToTokens := make(map[persist.EthereumAddress][]Token)
	for _, token := range tokens {
		for _, owner := range tokenIDToOwner[token.ID.TokenID] {
			ownersToTokens[owner] = append(ownersToTokens[owner], token)
		}
	}

	cTokens, cContracts := alchemyTokensToChainAgnosticTokensWithOwners(ctx, ownersToTokens)
	if err != nil {
		return nil, nil, err
	}
	return cTokens, cContracts, nil
}

func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	tokens, contract, err := d.getTokenWithMetadata(ctx, tokenIdentifiers, true, 0)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(tokens) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no token found for contract address %s and token ID %s", tokenIdentifiers.ContractAddress, tokenIdentifiers.TokenID)
	}

	token, ok := util.FindFirst(tokens, func(t multichain.ChainAgnosticToken) bool {
		return t.OwnerAddress == ownerAddress
	})
	if !ok {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no token found for contract address %s and token ID %s and owner address %s", tokenIdentifiers.ContractAddress, tokenIdentifiers.TokenID, ownerAddress)
	}

	return token, contract, nil
}

func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return d.getTokenWithMetadata(ctx, tokenIdentifiers, true, 0)
}

func (d *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	tokens, contract, err := d.getTokenWithMetadata(ctx, ti, true, 0)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	if len(tokens) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, fmt.Errorf("no token found for contract address %s and token ID %s", ti.ContractAddress, ti.TokenID)
	}
	firstToken := tokens[0]
	return firstToken.Descriptors, contract.Descriptors, nil
}

type GetContractMetadataResponse struct {
	Address          persist.EthereumAddress `json:"address"`
	ContractMetadata ContractMetadata        `json:"contractMetadata"`
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getContractMetadata?contractAddress=%s", d.alchemyAPIURL, addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get contract metadata from alchemy api: %s", resp.Status)
	}

	var contractMetadataResponse GetContractMetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&contractMetadataResponse); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return multichain.ChainAgnosticContract{
		Address: persist.Address(contractMetadataResponse.Address),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:          contractMetadataResponse.ContractMetadata.Symbol,
			Name:            contractMetadataResponse.ContractMetadata.Name,
			OwnerAddress:    persist.Address(contractMetadataResponse.ContractMetadata.ContractDeployer),
			Description:     contractMetadataResponse.ContractMetadata.OpenseaCollection.Description,
			ProfileImageURL: contractMetadataResponse.ContractMetadata.OpenseaCollection.ImageURL,
		},
	}, nil

}

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTs?owner=%s&contractAddresses[]=%s&withMetadata=true&orderBy=transferTime", d.alchemyAPIURL, ownerAddress, contractAddress)
	tokens, err := getNFTsPaginate(ctx, url, 100, "pageKey", limit, offset, "", d.httpClient, nil, &getNFTsResponse{})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	cTokens, cContracts := alchemyTokensToChainAgnosticTokensForOwner(persist.EthereumAddress(ownerAddress), tokens)

	if len(cContracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for contract address %s", contractAddress)
	}
	return cTokens, cContracts[0], nil
}

func alchemyTokensToChainAgnosticTokensForOwner(owner persist.EthereumAddress, tokens []Token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	chainAgnosticTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	chainAgnosticContracts := make([]multichain.ChainAgnosticContract, 0, len(tokens))
	seenContracts := make(map[persist.Address]bool)
	for _, token := range tokens {
		cToken, cContract := alchemyTokenToChainAgnosticToken(owner, token)
		if _, ok := seenContracts[cContract.Address]; !ok {
			seenContracts[cContract.Address] = true
			chainAgnosticContracts = append(chainAgnosticContracts, cContract)
		}
		chainAgnosticTokens = append(chainAgnosticTokens, cToken)
	}
	return chainAgnosticTokens, chainAgnosticContracts
}

func (d *Provider) alchemyTokensToChainAgnosticTokens(ctx context.Context, tokens []Token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	chainAgnosticTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	chainAgnosticContracts := make([]multichain.ChainAgnosticContract, 0, len(tokens))
	seenContracts := make(map[persist.Address]bool)
	for _, token := range tokens {
		owners, err := d.getOwnersForToken(ctx, token)
		if err != nil {
			return nil, nil, err
		}
		for _, owner := range owners {
			cToken, cContract := alchemyTokenToChainAgnosticToken(owner, token)
			if _, ok := seenContracts[cContract.Address]; !ok {
				seenContracts[cContract.Address] = true
				chainAgnosticContracts = append(chainAgnosticContracts, cContract)
			}
			chainAgnosticTokens = append(chainAgnosticTokens, cToken)
		}
	}
	return chainAgnosticTokens, chainAgnosticContracts, nil
}

func alchemyTokensToChainAgnosticTokensWithOwners(ctx context.Context, tokens map[persist.EthereumAddress][]Token) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract) {
	chainAgnosticTokens := make([]multichain.ChainAgnosticToken, 0, len(tokens))
	chainAgnosticContracts := make([]multichain.ChainAgnosticContract, 0, len(tokens))
	seenContracts := make(map[persist.Address]bool)
	for owner, ownerTokens := range tokens {
		for _, token := range ownerTokens {
			cToken, cContract := alchemyTokenToChainAgnosticToken(owner, token)
			if _, ok := seenContracts[cContract.Address]; !ok {
				seenContracts[cContract.Address] = true
				chainAgnosticContracts = append(chainAgnosticContracts, cContract)
			}
			chainAgnosticTokens = append(chainAgnosticTokens, cToken)
		}
	}
	return chainAgnosticTokens, chainAgnosticContracts
}

type ownersResponse struct {
	Owners []persist.EthereumAddress `json:"owners"`
}

func (d *Provider) getOwnersForToken(ctx context.Context, token Token) ([]persist.EthereumAddress, error) {
	url := fmt.Sprintf("%s/getOwnersForToken?contractAddress=%s&tokenId=%s", d.alchemyAPIURL, token.Contract.Address, token.ID.TokenID)
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var owners ownersResponse
	if err := json.NewDecoder(resp.Body).Decode(&owners); err != nil {
		return nil, err
	}

	if len(owners.Owners) == 0 {
		return nil, fmt.Errorf("no owners found for token %s-%s", token.ID.TokenID, token.Contract.Address)
	}

	return owners.Owners, nil
}

func (d *Provider) cacheMetadatasForTokens(ctx context.Context, tokens ...multichain.ChainAgnosticToken) error {
	for _, token := range tokens {
		if token.TokenMetadata != nil && len(token.TokenMetadata) > 0 {
			if token.TokenMetadata["name"] == "" && token.TokenMetadata["description"] == "" && token.TokenMetadata["external_url"] == "" && token.TokenMetadata["image_url"] == "" && token.TokenMetadata["animation_url"] == "" {
				continue
			}
			mar, err := json.Marshal(token.TokenMetadata)
			if err != nil {
				return err
			}
			err = d.cache.Set(ctx, fmt.Sprintf("%s-%d", multichain.ChainAgnosticIdentifiers{ContractAddress: token.ContractAddress, TokenID: token.TokenID}, d.chain), mar, time.Minute*10)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func alchemyTokenToChainAgnosticToken(owner persist.EthereumAddress, token Token) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract) {

	var tokenType persist.TokenType
	switch token.ID.TokenMetadata.TokenType {
	case "ERC721":
		tokenType = persist.TokenTypeERC721
	case "ERC1155":
		tokenType = persist.TokenTypeERC1155
	}

	bal, ok := new(big.Int).SetString(token.Balance, 10)
	if !ok {
		bal = big.NewInt(1)
	}

	externalURL, _ := token.Metadata["external_url"].(string)

	t := multichain.ChainAgnosticToken{
		TokenType: tokenType,
		Descriptors: multichain.ChainAgnosticTokenDescriptors{
			Name:        token.Title,
			Description: token.Description,
		},
		TokenURI:        persist.TokenURI(token.TokenURI.Raw),
		TokenMetadata:   token.Metadata,
		TokenID:         token.ID.TokenID.ToTokenID(),
		Quantity:        persist.HexString(bal.Text(16)),
		OwnerAddress:    persist.Address(owner),
		ContractAddress: persist.Address(token.Contract.Address),
		ExternalURL:     externalURL,
	}

	isSpam, err := strconv.ParseBool(token.SpamInfo.IsSpam)
	if err == nil {
		t.IsSpam = &isSpam
	}

	contractSpam := contractNameIsSpam(token.ContractMetadata.Name)

	return t, multichain.ChainAgnosticContract{
		Address: persist.Address(token.Contract.Address),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:       token.ContractMetadata.Symbol,
			Name:         token.ContractMetadata.Name,
			OwnerAddress: persist.Address(token.ContractMetadata.ContractDeployer),
		},
		IsSpam: &contractSpam,
	}
}

func contractNameIsSpam(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".lens-follower")
}
