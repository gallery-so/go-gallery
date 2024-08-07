package reservoir

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("RESERVOIR_API_KEY", "required")
}

const (
	ethMainnetBaseURL  = "https://api.reservoir.tools"
	optimismBaseURL    = "https://api-optimism.reservoir.tools"
	polygonBaseURL     = "https://api-polygon.reservoir.tools"
	arbitrumBaseURL    = "https://api-arbitrum.reservoir.tools"
	zoraBaseURL        = "https://api-zora.reservoir.tools"
	baseBaseURL        = "https://api-base.reservoir.tools"
	baseSepoliaBaseURL = "https://api-sepolia.reservoir.tools"
)

const (
	userTokensEndpointTemplate  = "%s/users/%s/tokens/v7"
	tokensEndpointTemplate      = "%s/tokens/v7"
	collectionsEndpointTemplate = "%s/collections/v7"
)

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
	TokenID         persist.HexTokenID
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
	ID              string          `json:"id"`
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
	// reservoir doesn't keep data for parent contracts - only collections in the parent contract
	// e.g collection data is available for projects within Art Blocks, but not for the Art Blocks
	// contract itself. We use another fetcher to get that data.
	cFetcher common.ContractFetcher
	r        *retry.Retryer
}

// NewProvider creates a new Reservoir provider
func NewProvider(ctx context.Context, httpClient *http.Client, chain persist.Chain, l retry.Limiter) (*Provider, func()) {
	apiURL := map[persist.Chain]string{
		persist.ChainETH:      ethMainnetBaseURL,
		persist.ChainOptimism: optimismBaseURL,
		persist.ChainPolygon:  polygonBaseURL,
		persist.ChainArbitrum: arbitrumBaseURL,
		persist.ChainZora:     zoraBaseURL,
		persist.ChainBase:     baseBaseURL,
	}[chain]
	if apiURL == "" {
		panic(fmt.Sprintf("no reservoir api url set for chain %s", chain))
	}

	apiKey := env.GetString("RESERVOIR_API_KEY")
	if apiKey == "" {
		panic("no reservoir api key set")
	}

	r, cleanup := retry.New(l, httpClient)

	return &Provider{
		apiURL:     apiURL,
		apiKey:     apiKey,
		chain:      chain,
		httpClient: httpClient,
		r:          r,
	}, cleanup
}

func (p *Provider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForWallet(ctx, ownerAddress, outCh)
	}()
	return assetsToTokens(ctx, p.chain, p.cFetcher, ownerAddress, outCh)
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, ownerAddress persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForWallet(ctx, ownerAddress, outCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		streamAssetsToTokens(ctx, p.chain, p.cFetcher, ownerAddress, outCh, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit int, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForContract(ctx, contractAddress, outCh)
	}()

	tokens, contracts, err := assetsToTokens(ctx, p.chain, p.cFetcher, "", outCh)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}

	if len(contracts) == 0 {
		return nil, common.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}

	return tokens, contracts[0], nil
}

// GetTokensIncrementallyByContractAddress returns tokens for a contract address
func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForContract(ctx, address, outCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		streamAssetsToTokens(ctx, p.chain, p.cFetcher, address, outCh, recCh, errCh)
	}()
	return recCh, errCh
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti common.ChainAgnosticIdentifiers, ownerAddress persist.Address) (common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForTokenIdentifiersAndOwner(ctx, ownerAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()

	tokens, contracts, err := assetsToTokens(ctx, p.chain, p.cFetcher, ownerAddress, outCh)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	if len(tokens) == 0 {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID, OwnerAddress: ownerAddress}
	}

	if len(contracts) == 0 {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: ti.ContractAddress}
	}

	return tokens[0], contracts[0], nil
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (common.ChainAgnosticTokenDescriptors, common.ChainAgnosticContractDescriptors, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForToken(ctx, ti.ContractAddress, ti.TokenID, outCh)
	}()

	// ownerAddress is omitted, but its not required in this context
	tokens, contracts, err := assetsToTokens(ctx, p.chain, p.cFetcher, "", outCh)
	if err != nil {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, err
	}

	if len(tokens) == 0 {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, ErrTokenNotFoundByIdentifiers{ContractAddress: ti.ContractAddress, TokenID: ti.TokenID}
	}

	if len(contracts) == 0 {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, ErrCollectionNotFoundByAddress{Address: ti.ContractAddress}
	}

	return tokens[0].Descriptors, contracts[0].Descriptors, nil
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForToken(ctx, ti.ContractAddress, ti.TokenID, outCh)
	}()

	// ownerAddress is omitted, but its not required in this context
	tokens, _, err := assetsToTokens(ctx, p.chain, p.cFetcher, "", outCh)
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

func (p Provider) GetContractByAddress(ctx context.Context, chain persist.Chain, contractAddress persist.Address) (common.ChainAgnosticContract, error) {
	c, err := p.fetchCollectionByAddress(ctx, contractAddress)
	if err != nil {
		return common.ChainAgnosticContract{}, ErrCollectionNotFoundByAddress{Address: contractAddress}
	}
	return collectionToAgnosticContract(ctx, chain, p.cFetcher, c, contractAddress)
}

// GetTokensByTokenIdentifiersBatch returns a slice tokens from a list of token identifiers
// Data is returned in the same order as the input. If a token is not found, the zero-value is used instead.
func (p Provider) GetTokensByTokenIdentifiersBatch(ctx context.Context, tIDs []common.ChainAgnosticIdentifiers) ([]common.ChainAgnosticToken, []error) {
	asTokens := util.MapWithoutError(tIDs, func(ti common.ChainAgnosticIdentifiers) persist.TokenIdentifiers {
		return persist.NewTokenIdentifiers(ti.ContractAddress, ti.TokenID, p.chain)
	})

	outCh := make(chan pageResult)
	go func() {
		defer close(outCh)
		p.streamAssetsForTokens(ctx, asTokens, outCh)
	}()

	ret := make([]common.ChainAgnosticToken, len(tIDs))
	errs := make([]error, len(tIDs))

	tokens, _, err := assetsToTokens(ctx, p.chain, nil, "", outCh)
	if err != nil {
		// fill with the same error
		for i := range tIDs {
			errs[i] = err
		}
		return nil, errs
	}

	lookup := make(map[persist.TokenIdentifiers]common.ChainAgnosticToken)
	for _, t := range tokens {
		lookup[persist.TokenIdentifiers{
			TokenID:         t.TokenID,
			ContractAddress: t.ContractAddress,
			Chain:           p.chain,
		}] = t
	}

	for i, t := range asTokens {
		if r, ok := lookup[t]; !ok {
			errs[i] = fmt.Errorf("reservoir unable to find token(chain=%s, contract=%s, tokenId=%s)", t.Chain, t.ContractAddress, t.TokenID)
		} else {
			ret[i] = r
		}
	}

	return ret, errs
}

func (p *Provider) paginateTokens(ctx context.Context, req *http.Request, outCh chan<- pageResult) {
	for {
		resp, err := p.r.Do(req)
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

func (p *Provider) streamAssetsForToken(ctx context.Context, contractAddress persist.Address, tokenID persist.HexTokenID, outCh chan<- pageResult) {
	endpoint := mustTokensEndpoint(p.apiURL)
	setToken(endpoint, contractAddress, tokenID)
	p.paginateTokens(ctx, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForTokens(ctx context.Context, tIDs []persist.TokenIdentifiers, outCh chan<- pageResult) {
	// Reservoir chokes with a batch size greater than 10
	for _, batch := range util.ChunkBy(tIDs, 10) {
		endpoint := mustTokensEndpoint(p.apiURL)
		if err := setTokens(endpoint, batch); err != nil {
			outCh <- pageResult{Err: err}
			return
		}
		p.paginateTokens(ctx, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
	}
}

func (p *Provider) streamAssetsForWallet(ctx context.Context, addr persist.Address, outCh chan<- pageResult) {
	endpoint := mustUserTokensEndpoint(p.apiURL, addr)
	setPagingParams(endpoint, "acquiredAt")
	p.paginateTokens(ctx, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, ownerAddress, contractAddress persist.Address, tokenID persist.HexTokenID, outCh chan<- pageResult) {
	endpoint := mustUserTokensEndpoint(p.apiURL, ownerAddress)
	setLimit(endpoint, 1)
	setToken(endpoint, contractAddress, tokenID)
	p.paginateTokens(ctx, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
}

func (p *Provider) streamAssetsForContract(ctx context.Context, contractAddress persist.Address, outCh chan<- pageResult) {
	endpoint := mustTokensEndpoint(p.apiURL)
	setCollection(endpoint, contractAddress)
	setPagingParams(endpoint, "tokenId")
	p.paginateTokens(ctx, mustAuthRequest(ctx, endpoint, p.apiKey), outCh)
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

func (p *Provider) fetchBlockScoutMetadata(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
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

func assetsToTokens(ctx context.Context, chain persist.Chain, cFetcher common.ContractFetcher, ownerAddress persist.Address, outCh <-chan pageResult) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {
	resultTokens := make([]common.ChainAgnosticToken, 0, len(outCh))
	resultContracts := make([]common.ChainAgnosticContract, 0, len(outCh))
	seenCollections := make(map[string]common.ChainAgnosticContract)
	for page := range outCh {
		if page.Err != nil {
			return nil, nil, page.Err
		}
		for _, t := range page.Tokens {
			resultTokens = append(resultTokens, assetToAgnosticToken(t, ownerAddress))

			collectionID := t.Token.Collection.ID

			if _, ok := seenCollections[collectionID]; !ok {
				c, err := collectionToAgnosticContract(ctx, chain, cFetcher, t.Token.Collection, t.Token.Contract)
				if err != nil {
					return nil, nil, page.Err
				}

				seenCollections[collectionID] = c
				resultContracts = append(resultContracts, seenCollections[collectionID])
			}
		}
	}
	return resultTokens, resultContracts, nil
}

func streamAssetsToTokens(ctx context.Context, chain persist.Chain, cFetcher common.ContractFetcher, ownerAddress persist.Address, outCh <-chan pageResult, recCh chan<- common.ChainAgnosticTokensAndContracts, errCh chan<- error) {
	seenCollections := make(map[string]common.ChainAgnosticContract)

	for page := range outCh {
		if page.Err != nil {
			errCh <- page.Err
			return
		}

		resultTokens := make([]common.ChainAgnosticToken, 0, len(page.Tokens))
		resultContracts := make([]common.ChainAgnosticContract, 0, len(page.Tokens))

		for _, t := range page.Tokens {
			resultTokens = append(resultTokens, assetToAgnosticToken(t, ownerAddress))

			collectionID := t.Token.Collection.ID

			if _, ok := seenCollections[collectionID]; !ok {
				c, err := collectionToAgnosticContract(ctx, chain, cFetcher, t.Token.Collection, t.Token.Contract)
				if err != nil {
					errCh <- err
					return
				}
				seenCollections[collectionID] = c
			}

			resultContracts = append(resultContracts, seenCollections[collectionID])
		}

		recCh <- common.ChainAgnosticTokensAndContracts{
			Tokens:    resultTokens,
			Contracts: resultContracts,
		}
	}
}

func assetToAgnosticToken(t TokenWithOwnership, ownerAddress persist.Address) common.ChainAgnosticToken {
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

	descriptors := common.ChainAgnosticTokenDescriptors{
		Name:        t.Token.Name,
		Description: t.Token.Description,
	}

	if ownerAddress == "" {
		ownerAddress = t.Token.Owner
	}

	return common.ChainAgnosticToken{
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

func collectionToAgnosticContract(ctx context.Context, chain persist.Chain, cFetcher common.ContractFetcher, c Collection, contractAddress persist.Address) (common.ChainAgnosticContract, error) {
	// reservoir doesn't keep parent contract data
	if cFetcher != nil {
		if isSharedContract(c.ID) {
			return cFetcher.GetContractByAddress(ctx, contractAddress)
		}
		// Grails doesn't follow the shared contract format
		if platform.IsGrails(chain, contractAddress, c.Symbol) {
			return cFetcher.GetContractByAddress(ctx, contractAddress)
		}
	}
	return common.ChainAgnosticContract{
		Address: contractAddress,
		Descriptors: common.ChainAgnosticContractDescriptors{
			Symbol:          c.Symbol,
			Name:            c.Name,
			Description:     c.Description,
			ProfileImageURL: c.ImageURL,
			OwnerAddress:    c.Creator,
		},
	}, nil
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

func setToken(url *url.URL, contractAddress persist.Address, tokenID persist.HexTokenID) {
	setTokens(url, []persist.TokenIdentifiers{{ContractAddress: contractAddress, TokenID: tokenID}})
}

func setTokens(url *url.URL, tIDs []persist.TokenIdentifiers) error {
	if len(tIDs) > 50 {
		return errors.New("max limit is 50")
	}
	if len(tIDs) == 0 {
		return errors.New("no tokens provided")
	}
	query := url.Query()
	for _, tID := range tIDs {
		query.Add("tokens", fmt.Sprintf("%s:%s", tID.ContractAddress, tID.TokenID.Base10String()))
	}
	url.RawQuery = query.Encode()
	return nil
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

func mustAuthRequest(ctx context.Context, url *url.URL, apiKey string) *http.Request {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("x-api-key", apiKey)
	return req
}

func isSharedContract(collectionID string) bool {
	// shared contracts follow the format: <contract-address>:<namespace>
	if parts := strings.SplitN(collectionID, ":", 2); len(parts) == 2 {
		return true
	}
	return false
}
