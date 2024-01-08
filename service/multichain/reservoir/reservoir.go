package reservoir

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("RESERVOIR_API_KEY", "required")
}

var (
	ethMainnetBaseURL = "https://api.reservoir.tools"
	optimismBaseURL   = "https://api-optimism.reservoir.tools"
	polygonBaseURL    = "https://api-polygon.reservoir.tools"
	arbitrumBaseURL   = "https://api-arbitrum.reservoir.tools"
	zoraBaseURL       = "https://api-zora.reservoir.tools"
	baseBaseURL       = "https://api-base.reservoir.tools"
)

var (
	userTokensEndpointTemplate  = "%s/users/%s/tokens/v7"
	tokensEndpointTemplate      = "%s/tokens/v7"
	collectionsEndpointTemplate = "%s/collections/v7"
)

var chainToBaseURL = map[persist.Chain]string{
	persist.ChainETH:      ethMainnetBaseURL,
	persist.ChainOptimism: optimismBaseURL,
	persist.ChainPolygon:  polygonBaseURL,
	persist.ChainArbitrum: arbitrumBaseURL,
	persist.ChainZora:     zoraBaseURL,
	persist.ChainBase:     baseBaseURL,
}

func checkURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func mustUserTokensEndpoint(baseURL string, address persist.Address) *url.URL {
	s := fmt.Sprintf(userTokensEndpointTemplate, baseURL, address)
	return checkURL(s)
}

func mustTokensEndpoint(baseURL string) *url.URL {
	s := fmt.Sprintf(tokensEndpointTemplate, baseURL)
	return checkURL(s)
}

func mustCollectionsEndpoint(baseURL string) *url.URL {
	s := fmt.Sprintf(collectionsEndpointTemplate, baseURL)
	return checkURL(s)
}

type ErrTokenNotFoundByIdentifiers struct {
	ContractAddress persist.Address
	TokenID         persist.TokenID
	OwnerAddress    persist.Address
}

func (e ErrTokenNotFoundByIdentifiers) Error() string {
	return fmt.Sprintf("token not found for contract %s, tokenID %s, owner %s", e.ContractAddress, e.TokenID, e.OwnerAddress)
}

type ErrCollectionNotFoundByAddress struct {
	Address persist.Address
}

func (e ErrCollectionNotFoundByAddress) Error() string {
	return fmt.Sprintf("collection not found for address %s", e.Address)
}

type Token struct {
	Contract    persist.Address       `json:"contract"`
	TokenID     string                `json:"tokenId"`
	Kind        string                `json:"kind"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Metadata    persist.TokenMetadata `json:"metadata"`
	Media       string                `json:"media"`
	Image       string                `json:"image"`
	ImageSmall  string                `json:"imageSmall"`
	ImageLarge  string                `json:"imageLarge"`
	Collection  Collection            `json:"collection"`
	Owner       persist.Address       `json:"owner"`
	IsSpam      bool                  `json:"isSpam"`
}

type Collection struct {
	ID              persist.Address `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ImageURL        string          `json:"imageUrl"`
	Creator         persist.Address `json:"creator"`
	Symbol          string          `json:"symbol"`
	PrimaryContract persist.Address `json:"primaryContract"`
}

type Ownership struct {
	TokenCount string `json:"tokenCount"`
	AcquiredAt string `json:"acquiredAt"`
}

type TokenWithOwnership struct {
	Token     Token     `json:"token"`
	Ownership Ownership `json:"ownership"`
}

type pageResult struct {
	Tokens []TokenWithOwnership
	Err    error
}

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	chain      persist.Chain
	apiURL     string
	apiKey     string
	httpClient *http.Client
}

// NewProvider creates a new ethereum Provider
func NewProvider(httpClient *http.Client, chain persist.Chain) *Provider {
	apiURL := chainToBaseURL[chain]
	if apiURL == "" {
		panic(fmt.Sprintf("no reservoir api url set for chain %d", chain))
	}

	apiKey := env.GetString("RESERVOIR_API_KEY")
	if apiKey == "" {
		panic("no reservoir api key set")
	}

	return &Provider{
		apiURL:     apiURL,
		apiKey:     apiKey,
		chain:      chain,
		httpClient: httpClient,
	}
}

func (p *Provider) ProviderInfo() multichain.ProviderInfo {
	return multichain.ProviderInfo{
		Chain:      p.chain,
		ChainID:    persist.MustChainToChainID(p.chain),
		ProviderID: "reservoir",
	}
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForWallet(ctx, ownerAddress, outCh)
	}()
	return assetsToTokens(ownerAddress, outCh)
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, ownerAddress persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForWallet(ctx, ownerAddress, outCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		streamAssetsToTokens(ownerAddress, outCh, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit int, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForContract(ctx, contractAddress, outCh)
	}()

	tokens, contracts, err := assetsToTokens("", outCh)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}

	return tokens, contracts[0], nil
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForContract(ctx, address, outCh)
	}()
	go func() {
		defer close(recCh)
		streamAssetsToTokens(address, outCh, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForTokenIdentifiersAndOwner(ctx, ownerAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()

	tokens, contracts, err := assetsToTokens(ownerAddress, outCh)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	if len(tokens) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
	}

	if len(contracts) == 0 {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: ti.ContractAddress}
	}

	return tokens[0], contracts[0], nil
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForToken(ctx, ti.ContractAddress, ti.TokenID, outCh)
	}()

	// ownerAddress is omitted, but its not required in this context
	tokens, contracts, err := assetsToTokens("", outCh)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}

	if len(tokens) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
	}

	if len(contracts) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, ErrCollectionNotFoundByAddress{Address: ti.ContractAddress}
	}

	return tokens[0].Descriptors, contracts[0].Descriptors, nil
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForToken(ctx, ti.ContractAddress, ti.TokenID, outCh)
	}()

	// ownerAddress is omitted, but its not required in this context
	tokens, _, err := assetsToTokens("", outCh)
	if err != nil && p.chain == persist.ChainBase {
		return p.fetchBlockScoutMetadata(ctx, ti)
	}
	if err != nil {
		return persist.TokenMetadata{}, err
	}

	if len(tokens) == 0 {
		return persist.TokenMetadata{}, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
	}

	return tokens[0].TokenMetadata, nil
}

func (p *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, ownerAddress persist.Address, contractAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForContractAndOwner(ctx, ownerAddress, contractAddress, outCh)
	}()

	tokens, contracts, err := assetsToTokens("", outCh)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	if len(contracts) == 0 {
		return nil, multichain.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}

	return tokens, contracts[0], nil
}

func (p Provider) GetContractByAddress(ctx context.Context, contractAddress persist.Address) (multichain.ChainAgnosticContract, error) {
	c, err := p.fetchCollectionByAddress(ctx, contractAddress)
	if err != nil {
		return multichain.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}
	return collectionToAgnosticContract(c, contractAddress), nil
}

func paginateTokens(ctx context.Context, client *http.Client, req *http.Request, outCh chan<- pageResult) {
	for {
		resp, err := retry.RetryRequest(client, req)
		if err != nil {
			outCh <- pageResult{Err: err}
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			outCh <- pageResult{Err: errors.New("API key expired")}
			return
		}

		if resp.StatusCode != http.StatusOK {
			outCh <- pageResult{Err: util.BodyAsError(resp)}
			return
		}

		page := struct {
			Tokens       []TokenWithOwnership `json:"tokens"`
			Continuation string               `json:"continuation"`
		}{}

		if err := util.UnmarshallBody(&page, resp.Body); err != nil {
			outCh <- pageResult{Err: err}
			return
		}

		outCh <- pageResult{Tokens: page.Tokens}

		if page.Continuation == "" {
			return
		}

		setNext(req.URL, page.Continuation)
	}
}

func (p *Provider) streamAssetsForToken(ctx context.Context, contractAddress persist.Address, tokenID persist.TokenID, outCh chan<- pageResult) {
	endpoint := mustTokensEndpoint(p.apiURL)
	setToken(endpoint, contractAddress, tokenID)
	paginateTokens(ctx, p.httpClient, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForWallet(ctx context.Context, addr persist.Address, outCh chan<- pageResult) {
	endpoint := mustUserTokensEndpoint(p.apiURL, addr)
	setPagingParams(endpoint, "acquiredAt")
	paginateTokens(ctx, p.httpClient, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, ownerAddress, contractAddress persist.Address, tokenID persist.TokenID, outCh chan<- pageResult) {
	endpoint := mustUserTokensEndpoint(p.apiURL, ownerAddress)
	setLimit(endpoint, 1)
	setToken(endpoint, contractAddress, tokenID)
	paginateTokens(ctx, p.httpClient, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForContract(ctx context.Context, contractAddress persist.Address, outCh chan<- pageResult) {
	endpoint := mustTokensEndpoint(p.apiURL)
	setCollection(endpoint, contractAddress)
	setPagingParams(endpoint, "tokenId")
	paginateTokens(ctx, p.httpClient, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForContractAndOwner(ctx context.Context, ownerAddress, contractAddress persist.Address, outCh chan<- pageResult) {
	endpoint := mustUserTokensEndpoint(p.apiURL, ownerAddress)
	setContract(endpoint, contractAddress)
	setPagingParams(endpoint, "acquiredAt")
	paginateTokens(ctx, p.httpClient, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) fetchCollectionByAddress(ctx context.Context, contractAddress persist.Address) (Collection, error) {
	endpoint := mustCollectionsEndpoint(p.apiURL)
	setLimit(endpoint, 1)
	setCollectionID(endpoint, contractAddress)

	resp, err := p.httpClient.Do(mustAuthRequest(ctx, endpoint, p.apiKey))
	if err != nil {
		return Collection{}, err
	}

	defer resp.Body.Close()

	res := struct {
		Collections []Collection `json:"collections"`
	}{}

	if err := util.UnmarshallBody(&res, resp.Body); err != nil {
		return Collection{}, err
	}

	if len(res.Collections) == 0 {
		return Collection{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}

	return res.Collections[0], nil
}

func (p *Provider) fetchBlockScoutMetadata(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	u := fmt.Sprintf("https://base.blockscout.com/api/v2/tokens/%s/instances/%s", ti.ContractAddress, ti.TokenID.Base10String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	res := struct {
		AnimationURL   string                `json:"animation_url"`
		ExternalAppURL string                `json:"external_app_url"`
		ID             string                `json:"id"`
		ImageURL       string                `json:"image_url"`
		IsUnique       bool                  `json:"is_unique"`
		Metadata       persist.TokenMetadata `json:"metadata"`
	}{}

	if err := util.UnmarshallBody(&res, resp.Body); err != nil {
		return nil, err
	}

	if len(res.Metadata) == 0 {
		return nil, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
	}

	if _, ok := util.FindFirstFieldFromMap(res.Metadata, "image", "image_url", "imageURL").(string); !ok {
		res.Metadata["image_url"] = res.ImageURL
	}

	if _, ok := util.FindFirstFieldFromMap(res.Metadata, "animation", "animation_url").(string); !ok {
		res.Metadata["animation_url"] = res.AnimationURL
	}

	return res.Metadata, nil
}

func assetsToTokens(ownerAddress persist.Address, outCh <-chan pageResult) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	resultTokens := make([]multichain.ChainAgnosticToken, 0, len(outCh))
	resultContracts := make([]multichain.ChainAgnosticContract, 0, len(outCh))
	seenContracts := make(map[persist.Address]bool)
	for page := range outCh {
		if page.Err != nil {
			return nil, nil, page.Err
		}
		for _, t := range page.Tokens {
			resultTokens = append(resultTokens, assetToAgnosticToken(t, ownerAddress))
			if !seenContracts[t.Token.Contract] {
				resultContracts = append(resultContracts, collectionToAgnosticContract(t.Token.Collection, t.Token.Contract))
			}
		}
	}
	return resultTokens, resultContracts, nil
}

func streamAssetsToTokens(ownerAddress persist.Address, outCh <-chan pageResult, recCh chan<- multichain.ChainAgnosticTokensAndContracts, errCh chan<- error) {
	for page := range outCh {
		if page.Err != nil {
			errCh <- page.Err
		}
		resultTokens := make([]multichain.ChainAgnosticToken, 0, len(page.Tokens))
		resultContracts := make([]multichain.ChainAgnosticContract, 0, len(page.Tokens))
		for _, t := range page.Tokens {
			resultTokens = append(resultTokens, assetToAgnosticToken(t, ownerAddress))
			resultContracts = append(resultContracts, collectionToAgnosticContract(t.Token.Collection, t.Token.Contract))

		}
		recCh <- multichain.ChainAgnosticTokensAndContracts{
			Tokens:    resultTokens,
			Contracts: resultContracts,
		}
	}
}

func assetToAgnosticToken(t TokenWithOwnership, ownerAddress persist.Address) multichain.ChainAgnosticToken {
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

	descriptors := multichain.ChainAgnosticTokenDescriptors{
		Name:        t.Token.Name,
		Description: t.Token.Description,
	}

	if ownerAddress == "" {
		ownerAddress = t.Token.Owner
	}

	return multichain.ChainAgnosticToken{
		ContractAddress: t.Token.Contract,
		Descriptors:     descriptors,
		TokenType:       tokenType,
		TokenID:         persist.MustTokenID(t.Token.TokenID),
		Quantity:        tokenQuantity,
		OwnerAddress:    ownerAddress,
		TokenMetadata:   t.Token.Metadata,
		FallbackMedia:   persist.FallbackMedia{ImageURL: persist.NullString(t.Token.Image)},
		IsSpam:          util.ToPointer(t.Token.IsSpam),
	}
}

func collectionToAgnosticContract(c Collection, contractAddress persist.Address) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address: contractAddress,
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:          c.Symbol,
			Name:            c.Name,
			Description:     c.Description,
			ProfileImageURL: c.ImageURL,
			OwnerAddress:    c.Creator,
		},
	}
}

func setPagingParams(url *url.URL, sortBy string) {
	query := url.Query()
	query.Set("limit", "100")
	query.Set("sort_by", sortBy)
	url.RawQuery = query.Encode()
}

func setNext(url *url.URL, next string) {
	query := url.Query()
	query.Set("continuation", next)
	url.RawQuery = query.Encode()
}

func setLimit(url *url.URL, limit int) {
	query := url.Query()
	query.Set("limit", fmt.Sprintf("%d", limit))
	url.RawQuery = query.Encode()
}

func setToken(url *url.URL, contractAddress persist.Address, tokenID persist.TokenID) {
	query := url.Query()
	query.Set("tokens", fmt.Sprintf("%s:%s", contractAddress, tokenID.Base10String()))
	url.RawQuery = query.Encode()
}

func setCollectionID(url *url.URL, contractAddress persist.Address) {
	query := url.Query()
	query.Set("id", contractAddress.String())
	url.RawQuery = query.Encode()
}

func setCollection(url *url.URL, contractAddress persist.Address) {
	query := url.Query()
	query.Set("collection", contractAddress.String())
	url.RawQuery = query.Encode()
}

func setContract(url *url.URL, contractAddress persist.Address) {
	query := url.Query()
	query.Set("contract", contractAddress.String())
	url.RawQuery = query.Encode()
}

func mustAuthRequest(ctx context.Context, url *url.URL, apiKey string) *http.Request {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("x-api-key", apiKey)
	return req
}
