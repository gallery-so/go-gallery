package alchemy

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
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

type Metadata struct {
	Image           string `json:"image"`
	AnimationURL    string `json:"animation_url"`
	ExternalURL     string `json:"external_url"`
	BackgroundColor string `json:"background_color"`
	Name            string `json:"name"`
	Description     string `json:"description"`
}

// UnmarshalJSON is a custom unmarshaler for the Metadata struct
func (m *Metadata) UnmarshalJSON(data []byte) error {

	type Alias Metadata
	aux := Alias{}

	if err := json.Unmarshal(data, &aux); err != nil {

		asString := ""
		if err := json.Unmarshal(data, &asString); err != nil {
			fmt.Printf("failed to unmarshal Metadata as string: %s", err)
			return nil
		}

		fmt.Println("Metadata as string:", asString)
		return nil
	}

	*m = Metadata(aux)
	return nil
}

type ContractMetadata struct {
	Name             string `json:"name"`
	Symbol           string `json:"symbol"`
	TotalSupply      string `json:"totalSupply"`
	TokenType        string `json:"tokenType"`
	ContractDeployer string `json:"contractDeployer"`
}

type Contract struct {
	Address string `json:"address"`
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
	Contract         Contract         `json:"contract"`
	ID               TokenIdentifiers `json:"id"`
	Balance          string           `json:"balance"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	TokenURI         TokenURI         `json:"tokenUri"`
	Media            []Media          `json:"media"`
	Metadata         Metadata         `json:"metadata"`
	ContractMetadata ContractMetadata `json:"contractMetadata"`
	TimeLastUpdated  time.Time        `json:"timeLastUpdated"`
	SpamInfo         SpamInfo         `json:"spamInfo"`
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

type getNFTsForCollectionWithOwnerResponse struct {
	owner     persist.EthereumAddress
	d         *Provider
	ctx       context.Context
	NFTs      []Token `json:"nfts"`
	NextToken TokenID `json:"nextToken"`
}

func (r *getNFTsForCollectionWithOwnerResponse) GetTokensFromResponse(resp *http.Response) ([]Token, error) {
	r.NFTs = nil
	if err := json.NewDecoder(resp.Body).Decode(r); err != nil {
		return nil, err
	}

	return util.Filter(r.NFTs, func(t Token) bool {
		owners, err := r.d.getOwnersForToken(r.ctx, t)
		if err != nil {
			return false
		}
		return util.Contains(owners, r.owner)
	}, true), nil
}

func (r getNFTsForCollectionWithOwnerResponse) GetNextPageKey() string {
	return r.NextToken.String()
}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	chain         persist.Chain
	alchemyAPIURL string
	httpClient    *http.Client
}

// NewProvider creates a new ethereum Provider
func NewProvider(chain persist.Chain, httpClient *http.Client) *Provider {
	var apiURL string
	switch chain {
	case persist.ChainETH:
		apiURL = env.GetString("ALCHEMY_API_URL")
	case persist.ChainOptimism:
		apiURL = env.GetString("ALCHEMY_OPTIMISM_API_URL")
	case persist.ChainPolygon:
		apiURL = env.GetString("ALCHEMY_POLYGON_API_URL")
	}

	if apiURL == "" {
		panic(fmt.Sprintf("no alchemy api url set for chain %d", chain))
	}

	return &Provider{
		alchemyAPIURL: apiURL,
		chain:         chain,
		httpClient:    httpClient,
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
	}
	return multichain.BlockchainInfo{
		Chain:   d.chain,
		ChainID: chainID,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTs?owner=%s&withMetadata=true", d.alchemyAPIURL, addr)
	if d.chain == persist.ChainPolygon {
		url += "&excludeFilters[]=SPAM"
	}
	tokens, err := getNFTsPaginate(ctx, url, 100, "pageKey", limit, offset, "", d.httpClient, &getNFTsResponse{})
	if err != nil {
		return nil, nil, err
	}

	cTokens, cContracts := alchemyTokensToChainAgnosticTokensForOwner(persist.EthereumAddress(addr), tokens)

	return cTokens, cContracts, nil
}

func getNFTsPaginate[T tokensPaginated](ctx context.Context, baseURL string, defaultLimit int, pageKeyName string, limit, offset int, pageKey string, httpClient *http.Client, result T) ([]Token, error) {

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
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		asString, _ := ioutil.ReadAll(resp.Body)
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

	tokens = append(tokens, newTokens...)

	if nextPageKey != "" && nextPageKey != pageKey {

		if limit > 0 {
			limit -= defaultLimit
		}
		if offset > 0 {
			offset -= defaultLimit
		}
		newTokens, err := getNFTsPaginate(ctx, baseURL, defaultLimit, pageKeyName, limit, offset, nextPageKey, httpClient, result)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, newTokens...)
	}

	return tokens, nil
}

// // GetTokenMetadataByTokenIdentifiers retrieves a token's metadata for a given contract address and token ID
// func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (persist.TokenMetadata, error) {
// 	tokens, _, err := d.getTokenWithMetadata(ctx, ti, false, 0)
// 	if err != nil {
// 		return persist.TokenMetadata{}, err
// 	}

// 	if len(tokens) == 0 {
// 		return persist.TokenMetadata{}, fmt.Errorf("no token found for contract address %s and token ID %s", ti.ContractAddress, ti.TokenID)
// 	}

// 	token := tokens[0]
// 	return token.TokenMetadata, nil
// }

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
		return nil, multichain.ChainAgnosticContract{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := util.GetErrFromResp(resp)
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get token metadata from alchemy api: %s (%w)", resp.Status, err)
	}

	// will have most of the fields empty
	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	if token.Metadata.Image == "" && forceRefresh == false {
		return d.getTokenWithMetadata(ctx, ti, true, timeout)
	}

	tokens, contracts, err := d.alchemyTokensToChainAgnosticTokens(ctx, []Token{token})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("failed to get contracts from alchemy api")
	}

	return tokens, contracts[0], nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Ethereum Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTsForCollection?contractAddress=%s&withMetadata=true&tokenUriTimeoutInMs=20000", d.alchemyAPIURL, contractAddress)
	tokens, err := getNFTsPaginate(ctx, url, 100, "startToken", limit, offset, "", d.httpClient, &getNFTsForCollectionResponse{})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	cTokens, cContracts, err := d.alchemyTokensToChainAgnosticTokens(ctx, tokens)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(cContracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for contract address %s", contractAddress)
	}
	return cTokens, cContracts[0], nil
}

func (d *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTsForCollection?contractAddress=%s&withMetadata=true&tokenUriTimeoutInMs=20000", d.alchemyAPIURL, contractAddress)
	tokens, err := getNFTsPaginate(ctx, url, 100, "startToken", limit, offset, "", d.httpClient, &getNFTsForCollectionWithOwnerResponse{owner: persist.EthereumAddress(ownerAddress), d: d, ctx: ctx})
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	cTokens, cContracts, err := d.alchemyTokensToChainAgnosticTokens(ctx, tokens)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(cContracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for contract address %s", contractAddress)
	}
	return cTokens, cContracts[0], nil
}

func (d *Provider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
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

type contractMetadataResponse struct {
	Address          persist.EthereumAddress `json:"address"`
	ContractMetadata ContractMetadata        `json:"contractMetadata"`
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getContractMetadata?contract=%s", d.alchemyAPIURL, addr)
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

	var contractMetadataResponse contractMetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&contractMetadataResponse); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return multichain.ChainAgnosticContract{
		Address:        persist.Address(contractMetadataResponse.Address),
		Symbol:         contractMetadataResponse.ContractMetadata.Symbol,
		Name:           contractMetadataResponse.ContractMetadata.Name,
		CreatorAddress: persist.Address(contractMetadataResponse.ContractMetadata.ContractDeployer),
	}, nil

}

func (d *Provider) GetCommunityOwners(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticCommunityOwner, error) {
	owners, err := d.paginateCollectionOwners(ctx, contractAddress, limit, offset, "")
	if err != nil {
		return nil, err
	}
	result := make([]multichain.ChainAgnosticCommunityOwner, 0, limit)

	for _, owner := range owners {
		result = append(result, multichain.ChainAgnosticCommunityOwner{
			Address: persist.Address(owner),
		})
	}

	return result, nil
}

type collectionOwnersResponse struct {
	Owners  []persist.EthereumAddress `json:"owners"`
	PageKey string                    `json:"pageKey"`
}

func (d *Provider) paginateCollectionOwners(ctx context.Context, contractAddress persist.Address, limit, offset int, pagekey string) ([]persist.EthereumAddress, error) {
	allOwners := make([]persist.EthereumAddress, 0, limit)
	url := fmt.Sprintf("%s/getCollectionOwners?contractAddress=%s", d.alchemyAPIURL, contractAddress)
	if pagekey != "" {
		url = fmt.Sprintf("%s&pageKey=%s", url, pagekey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get collection owners from alchemy api: %s", resp.Status)
	}

	var collectionOwnersResponse collectionOwnersResponse
	if err := json.NewDecoder(resp.Body).Decode(&collectionOwnersResponse); err != nil {
		return nil, err
	}

	if offset > 0 && offset < 50000 {
		if len(collectionOwnersResponse.Owners) > offset {
			collectionOwnersResponse.Owners = collectionOwnersResponse.Owners[offset:]
		} else {
			collectionOwnersResponse.Owners = nil
		}
	}

	if limit > 0 && limit < 50000 {
		if len(collectionOwnersResponse.Owners) > limit {
			collectionOwnersResponse.Owners = collectionOwnersResponse.Owners[:limit]
		}
	}

	allOwners = append(allOwners, collectionOwnersResponse.Owners...)

	if collectionOwnersResponse.PageKey != "" {
		if limit > 0 && limit > 50000 {
			limit -= 50000
		}
		if offset > 0 && offset > 50000 {
			offset -= 50000
		}
		owners, err := d.paginateCollectionOwners(ctx, contractAddress, limit, offset, collectionOwnersResponse.PageKey)
		if err != nil {
			return nil, err
		}
		allOwners = append(collectionOwnersResponse.Owners, owners...)
	}

	return allOwners, nil
}

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	url := fmt.Sprintf("%s/getNFTs?owner=%s&contractAddresses[]=%s&withMetadata=true&orderBy=transferTime", d.alchemyAPIURL, ownerAddress, contractAddress)
	tokens, err := getNFTsPaginate(ctx, url, 100, "pageKey", limit, offset, "", d.httpClient, &getNFTsResponse{})
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

	t := multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		Name:            token.Title,
		Description:     token.Description,
		TokenURI:        persist.TokenURI(token.TokenURI.Raw),
		TokenMetadata:   alchemyTokenToMetadata(token),
		FallbackMedia:   alchemyTokenToFallback(token),
		TokenID:         token.ID.TokenID.ToTokenID(),
		Quantity:        persist.HexString(bal.Text(16)),
		OwnerAddress:    persist.Address(owner),
		ContractAddress: persist.Address(token.Contract.Address),
		ExternalURL:     token.Metadata.ExternalURL,
	}

	isSpam, err := strconv.ParseBool(token.SpamInfo.IsSpam)
	if err == nil {
		t.IsSpam = &isSpam
	}

	return t, multichain.ChainAgnosticContract{
		Address:        persist.Address(token.Contract.Address),
		Symbol:         token.ContractMetadata.Symbol,
		Name:           token.ContractMetadata.Name,
		CreatorAddress: persist.Address(token.ContractMetadata.ContractDeployer),
	}
}

func alchemyTokenToMetadata(token Token) persist.TokenMetadata {
	firstWithFormat, hasFormat := util.FindFirst(token.Media, func(m Media) bool {
		return m.Format != ""
	})
	metadata := persist.TokenMetadata{
		"name":         token.Metadata.Name,
		"description":  token.Metadata.Description,
		"external_url": token.Metadata.ExternalURL,
	}

	if token.Metadata.AnimationURL != "" {
		metadata["animation_url"] = token.Metadata.AnimationURL
	}

	if hasFormat {
		metadata["media_type"] = firstWithFormat.Format
		metadata["format"] = firstWithFormat.Format

		if _, ok := metadata["animation_url"]; !ok {
			medType := media.RawFormatToMediaType(firstWithFormat.Format)
			if medType.IsAnimationLike() {
				metadata["animation_url"] = token.Metadata.Image
				if firstWithFormat.Thumbnail != "" {
					metadata["image_url"] = firstWithFormat.Thumbnail
				}
				return metadata
			}
		}
	}

	if token.Metadata.Image != "" {
		metadata["image_url"] = token.Metadata.Image
	}
	return metadata
}

func alchemyTokenToFallback(token Token) persist.FallbackMedia {
	firstWithThumbnail, hasThumbnail := util.FindFirst(token.Media, func(m Media) bool {
		return m.Thumbnail != ""
	})
	if !hasThumbnail {
		return persist.FallbackMedia{}
	}

	return persist.FallbackMedia{
		ImageURL: persist.NullString(firstWithThumbnail.Thumbnail),
	}
}
