package opensea

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sourcegraph/conc/pool"
)

func init() {
	env.RegisterValidation("OPENSEA_API_KEY", "required")
}

const (
	pageSize = 50
	poolSize = 12
)

var newPool = func(ctx context.Context, s int) *pool.ContextPool {
	return pool.New().WithMaxGoroutines(s).WithContext(ctx)
}

var ErrAPIKeyExpired = errors.New("opensea api key expired")

var (
	baseURL, _                        = url.Parse("https://api.opensea.io/api/v2")
	getContractEndpointTemplate       = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s")
	getCollectionEndpointTemplate     = fmt.Sprintf("%s/%s", baseURL.String(), "collections/%s")
	getNftEndpointTemplate            = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s/nfts/%s")
	getNftsByWalletEndpointTemplate   = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/account/%s/nfts")
	getNftsByContractEndpointTemplate = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s/nfts")
)

// Map of chains to OS chain identifiers
var chainToIdentifier = map[persist.Chain]string{
	persist.ChainETH:      "ethereum",
	persist.ChainPolygon:  "matic",
	persist.ChainOptimism: "optimism",
	persist.ChainArbitrum: "arbitrum",
	persist.ChainBase:     "base",
	persist.ChainZora:     "zora",
}

type ErrOpenseaRateLimited struct{ Err error }

func (e ErrOpenseaRateLimited) Unwrap() error { return e.Err }
func (e ErrOpenseaRateLimited) Error() string {
	return fmt.Sprintf("rate limited by opensea: %s", e.Err)
}

type fetchOptions struct {
	// Skip fetching fallback media from reservoir.
	// This is useful in certain contexts where fallback media isn't needed
	// such as fetching a token's metadata or its descriptors
	ExcludeFallbackMedia bool
}

// Asset is an NFT from OpenSea
type Asset struct {
	Identifier    string  `json:"identifier"`
	Collection    string  `json:"collection"`
	Contract      string  `json:"contract"`
	TokenStandard string  `json:"token_standard"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ImageURL      string  `json:"image_url"`
	AnimationURL  string  `json:"animation_url"`
	MetadataURL   string  `json:"metadata_url"`
	Owners        []Owner `json:"owners"`
}

type Owner struct {
	Address  string `json:"address"`
	Quantity int    `json:"quantity"`
}

// Collection is a collection from OpenSea
type Collection struct {
	Description string `json:"description"`
	Owner       string `json:"owner"`
	ImageURL    string `json:"image_url"`
	ProjectURL  string `json:"project_url"`
}

// Contract represents an NFT contract from Opensea
type Contract struct {
	Address         persist.Address `json:"address"`
	ChainIdentifier string          `json:"chain_identifier"`
	Collection      string          `json:"collection"`
	Name            string          `json:"name"`
}

func FetchAssetsForTokenIdentifiers(ctx context.Context, chain persist.Chain, contractAddress persist.Address, tokenID persist.TokenID) ([]Asset, error) {
	outCh := make(chan assetsReceived)

	go func() {
		defer close(outCh)
		streamAssetsForToken(ctx, http.DefaultClient, chain, contractAddress, tokenID, outCh)
	}()

	assets := make([]Asset, 0)
	for a := range outCh {
		if a.Err != nil {
			return nil, a.Err
		}
		assets = append(assets, a.Assets...)
	}

	return assets, nil
}

type Provider struct {
	Chain      persist.Chain
	httpClient *http.Client
}

// NewProvider creates a new provider for OpenSea
func NewProvider(httpClient *http.Client, chain persist.Chain) *Provider {
	mustChainIdentifierFrom(chain)
	return &Provider{httpClient: httpClient, Chain: chain}
}

// GetTokensByWalletAddress returns a list of tokens for an address
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.Chain, ownerAddress, outCh)
	}()
	return p.assetsToTokens(ctx, ownerAddress, outCh, nil)
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for an address
func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, ownerAddress persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	outCh := make(chan assetsReceived, 32)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.Chain, ownerAddress, outCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, ownerAddress, outCh, recCh, errCh, nil)
	}()
	return recCh, errCh
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	assetsCh := make(chan assetsReceived)
	go func() {
		defer close(assetsCh)
		streamAssetsForContract(ctx, p.httpClient, p.Chain, address, assetsCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, address, assetsCh, recCh, errCh, nil)
	}()
	return recCh, errCh
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForContract(ctx, p.httpClient, p.Chain, contractAddress, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh, nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	var contract multichain.ChainAgnosticContract
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

// GetTokensByTokenIdentifiers returns a list of tokens for a list of token identifiers
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return p.GetTokensByTokenIdentifiersOptions(ctx, ti, nil)
}

// GetTokensByTokenIdentifiersOptions supports configuring options for fetching tokens
func (p *Provider) GetTokensByTokenIdentifiersOptions(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, opt *fetchOptions) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForToken(ctx, p.httpClient, p.Chain, ti.ContractAddress, ti.TokenID, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh, opt)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForTokenIdentifiersAndOwner(ctx, p.httpClient, p.Chain, ownerAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()

	tokens, contracts, err := p.assetsToTokens(ctx, ownerAddress, outCh, nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}

	if len(tokens) > 0 {
		return tokens[0], contract, nil
	}
	return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no tokens found for %s", ti)
}

// GetTokenMetadataByTokenIdentifiers retrieves a token's metadata for a given contract address and token ID
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	tokens, _, err := p.GetTokensByTokenIdentifiersOptions(ctx, ti, &fetchOptions{ExcludeFallbackMedia: true})
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens found for %s", ti)
	}

	return tokens[0].TokenMetadata, nil
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	tokens, contract, err := p.GetTokensByTokenIdentifiersOptions(ctx, ti, &fetchOptions{ExcludeFallbackMedia: true})
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}

	if len(tokens) == 0 {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, fmt.Errorf("no tokens found for %s", ti)
	}

	return tokens[0].Descriptors, contract.Descriptors, nil
}

// GetContractByAddress returns a contract for a contract address
func (p *Provider) GetContractByAddress(ctx context.Context, contractAddress persist.Address) (multichain.ChainAgnosticContract, error) {
	cc, err := fetchContractCollectionByAddress(ctx, p.httpClient, p.Chain, contractAddress)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return contractToChainAgnosticContract(cc.Contract, cc.Collection), nil
}

func (p *Provider) assetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived, opt *fetchOptions) (tokens []multichain.ChainAgnosticToken, contracts []multichain.ChainAgnosticContract, err error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, ownerAddress, outCh, recCh, errCh, opt)
	}()

	if err = <-errCh; err != nil {
		return nil, nil, err
	}

	for page := range recCh {
		tokens = append(tokens, page.Tokens...)
		contracts = append(contracts, page.Contracts...)
	}

	return tokens, contracts, nil
}

func (p *Provider) streamAssetsToTokens(
	ctx context.Context,
	ownerAddress persist.Address,
	outCh <-chan assetsReceived,
	recCh chan<- multichain.ChainAgnosticTokensAndContracts,
	errCh chan<- error,
	opt *fetchOptions,
) {
	contracts := &sync.Map{}                            // used to avoid duplicate contract fetches
	contractsL := make(map[persist.Address]*sync.Mutex) // contract job locks
	mu := sync.RWMutex{}                                // manages access to contract locks

	for page := range outCh {
		page := page

		if page.Err != nil {
			errCh <- page.Err
			return
		}

		if len(page.Assets) == 0 {
			continue
		}

		// fetch contracts
		addresses := util.MapWithoutError(page.Assets, func(a Asset) persist.Address { return persist.Address(a.Contract) })
		addresses = util.Dedupe(addresses, true)

		for _, a := range addresses {
			err := p.getChainAgnosticContract(ctx, a, contracts, contractsL, &mu)
			if err != nil {
				errCh <- err
				return
			}
		}

		var out multichain.ChainAgnosticTokensAndContracts

		for _, a := range page.Assets {
			contract, ok := contracts.Load(persist.Address(a.Contract))
			if !ok {
				panic("contract should have been loaded")
			}

			tokens, err := p.assetToChainAgnosticTokens(ctx, ownerAddress, a)
			if err != nil {
				errCh <- err
				return
			}

			out.Tokens = append(out.Tokens, tokens...)
			out.Contracts = append(out.Contracts, contract.(multichain.ChainAgnosticContract))
		}

		recCh <- out
	}
}

type contractCollection struct {
	Contract   Contract
	Collection Collection
}

func (p *Provider) getChainAgnosticContract(ctx context.Context, address persist.Address, seenContracts *sync.Map, contractLocks map[persist.Address]*sync.Mutex, mu *sync.RWMutex) error {
	_, ok := seenContracts.Load(address)
	// already have the contract
	if ok {
		return nil
	}

	// If we don't have a lock yet, create one
	mu.Lock()
	if _, hasJobLock := contractLocks[address]; !hasJobLock {
		contractLocks[address] = &sync.Mutex{}
	}
	mu.Unlock()

	// Acquire a lock for the job
	mu.RLock()
	jobMu := contractLocks[address]
	mu.RUnlock()

	// Process the contract
	jobMu.Lock()
	defer jobMu.Unlock()

	// Check again if we've seen the contract, since another job may have processed it while we were waiting for the lock
	_, ok = seenContracts.Load(address)
	if ok {
		return nil
	}

	c, err := fetchContractCollectionByAddress(ctx, p.httpClient, p.Chain, address)
	if err != nil {
		return err
	}

	seenContracts.Store(address, contractToChainAgnosticContract(c.Contract, c.Collection))
	return nil
}

func (p *Provider) assetToChainAgnosticTokens(ctx context.Context, ownerAddress persist.Address, asset Asset) ([]multichain.ChainAgnosticToken, error) {
	typ, err := tokenTypeFromAsset(asset)
	if err != nil {
		return nil, err
	}

	if ownerAddress != "" {
		token := assetToAgnosticToken(asset, typ, ownerAddress, 1)
		return []multichain.ChainAgnosticToken{token}, nil
	}

	// No input owner address provided. OS doesn't return the owner of the token a few endpoints
	// like when paginating a contract or a collection. The owner isn't really important for our purposes,
	// but we'll to add it to the token if its already available.

	if typ == persist.TokenTypeERC721 && len(asset.Owners) == 1 {
		token := assetToAgnosticToken(asset, typ, persist.Address(asset.Owners[0].Address), 1)
		return []multichain.ChainAgnosticToken{token}, nil
	}

	if typ == persist.TokenTypeERC1155 && len(asset.Owners) > 0 {
		tokens := make([]multichain.ChainAgnosticToken, len(asset.Owners))
		for i, o := range asset.Owners {
			tokens[i] = assetToAgnosticToken(asset, typ, persist.Address(o.Address), o.Quantity)
		}
		return tokens, nil
	}

	token := assetToAgnosticToken(asset, typ, "", 1)
	return []multichain.ChainAgnosticToken{token}, nil
}

func fetchContractCollectionByAddress(ctx context.Context, client *http.Client, chain persist.Chain, contractAddress persist.Address) (contractCollection, error) {
	contract, err := fetchContractByAddress(ctx, client, chain, contractAddress)
	if err != nil {
		return contractCollection{}, err
	}
	collection, err := fetchCollectionBySlug(ctx, client, chain, contract.Collection)
	if err != nil {
		return contractCollection{}, err
	}
	return contractCollection{contract, collection}, nil
}

func fetchContractByAddress(ctx context.Context, client *http.Client, chain persist.Chain, contractAddress persist.Address) (contract Contract, err error) {
	endpoint := mustContractEndpoint(chain, contractAddress)
	resp, err := retry.RetryRequest(client, mustAuthRequest(ctx, endpoint))
	if err != nil {
		return Contract{}, wrapRateLimitErr(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Contract{}, util.ErrHTTP{URL: endpoint.String(), Status: resp.StatusCode, Err: util.BodyAsError(resp)}
	}

	err = util.UnmarshallBody(&contract, resp.Body)
	if err != nil {
		return Contract{}, err
	}

	return contract, nil
}

func fetchCollectionBySlug(ctx context.Context, client *http.Client, chain persist.Chain, slug string) (collection Collection, err error) {
	endpoint := mustCollectionEndpoint(slug)
	resp, err := retry.RetryRequest(client, mustAuthRequest(ctx, endpoint))
	if err != nil {
		return Collection{}, wrapRateLimitErr(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Collection{}, util.ErrHTTP{URL: endpoint.String(), Status: resp.StatusCode, Err: util.BodyAsError(resp)}
	}

	err = util.UnmarshallBody(&collection, resp.Body)
	if err != nil {
		return Collection{}, err
	}

	return collection, nil
}

func checkURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func mustContractEndpoint(chain persist.Chain, address persist.Address) *url.URL {
	s := fmt.Sprintf(getContractEndpointTemplate, mustChainIdentifierFrom(chain), address)
	return checkURL(s)
}

func mustCollectionEndpoint(slug string) *url.URL {
	s := fmt.Sprintf(getCollectionEndpointTemplate, slug)
	return checkURL(s)
}

func mustNftEndpoint(chain persist.Chain, address persist.Address, tokenID persist.TokenID) *url.URL {
	s := fmt.Sprintf(getNftEndpointTemplate, mustChainIdentifierFrom(chain), address, tokenID.Base10String())
	return checkURL(s)
}

func mustNftsByWalletEndpoint(chain persist.Chain, address persist.Address) *url.URL {
	s := fmt.Sprintf(getNftsByWalletEndpointTemplate, mustChainIdentifierFrom(chain), address)
	return checkURL(s)
}

func mustNftsByContractEndpoint(chain persist.Chain, address persist.Address) *url.URL {
	s := fmt.Sprintf(getNftsByContractEndpointTemplate, mustChainIdentifierFrom(chain), address)
	return checkURL(s)
}

func streamAssetsForToken(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, tokenID persist.TokenID, outCh chan assetsReceived) {
	endpoint := mustNftEndpoint(chain, address, tokenID)
	setPagingParams(endpoint)
	paginateAssets(ctx, client, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForWallet(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint := mustNftsByWalletEndpoint(chain, address)
	setPagingParams(endpoint)
	paginateAssets(ctx, client, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForContract(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint := mustNftsByContractEndpoint(chain, address)
	setPagingParams(endpoint)
	paginateAssets(ctx, client, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, client *http.Client, chain persist.Chain, ownerAddress, contractAddress persist.Address, tokenID persist.TokenID, outCh chan assetsReceived) {
	endpoint := mustNftsByWalletEndpoint(chain, ownerAddress)
	setPagingParams(endpoint)
	paginateAssetsFilter(ctx, client, mustAuthRequest(ctx, endpoint), outCh, func(a Asset) bool {
		// OS doesn't let you filter by tokenID and owner, so we have to filter for only the token
		ca := persist.NewChainAddress(persist.Address(a.Contract), chain)
		if (ca.Address() == contractAddress) && (persist.MustTokenID(a.Identifier) == tokenID) {
			return true
		}
		return false
	})
}

func tokenTypeFromAsset(asset Asset) (persist.TokenType, error) {
	switch asset.TokenStandard {
	case "erc721", "cryptopunks":
		return persist.TokenTypeERC721, nil
	case "erc1155":
		return persist.TokenTypeERC1155, nil
	default:
		return "", fmt.Errorf("unknown token standard: %s", asset.TokenStandard)
	}
}

func contractToChainAgnosticContract(contract Contract, collection Collection) multichain.ChainAgnosticContract {
	desc := multichain.ChainAgnosticContractDescriptors{
		Symbol:          "", // OpenSea doesn't provide this, but it isn't exposed in the schema anyway
		Name:            contract.Name,
		OwnerAddress:    persist.Address(collection.Owner),
		Description:     collection.Description,
		ProfileImageURL: collection.ImageURL,
	}
	return multichain.ChainAgnosticContract{
		Address:     persist.Address(contract.Address.String()),
		Descriptors: desc,
		IsSpam:      util.ToPointer(contractNameIsSpam(contract.Name)),
	}
}

func assetToAgnosticToken(asset Asset, tokenType persist.TokenType, tokenOwner persist.Address, quantity int) multichain.ChainAgnosticToken {
	return multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		Descriptors:     multichain.ChainAgnosticTokenDescriptors{Name: asset.Name, Description: asset.Description},
		TokenURI:        persist.TokenURI(asset.MetadataURL),
		TokenID:         persist.MustTokenID(asset.Identifier),
		OwnerAddress:    tokenOwner,
		ContractAddress: persist.Address(asset.Contract),
		TokenMetadata:   metadataFromAsset(asset),
		Quantity:        persist.HexString(fmt.Sprintf("%x", quantity)),
	}
}

func metadataFromAsset(asset Asset) persist.TokenMetadata {
	m := persist.TokenMetadata{
		"name":          asset.Name,
		"description":   asset.Description,
		"image_url":     asset.ImageURL,
		"animation_url": asset.AnimationURL,
	}
	// ENS
	if persist.Address(asset.Contract) == eth.EnsAddress {
		m["profile_image"] = fmt.Sprintf("https://metadata.ens.domains/mainnet/avatar/%s", asset.Name)
	}
	return m
}

// mustAuthRequest returns a http.Request with authorization headers
func mustAuthRequest(ctx context.Context, url *url.URL) *http.Request {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("X-API-KEY", env.GetString("OPENSEA_API_KEY"))
	return req
}

type pageResult struct {
	Next string  `json:"next"`
	NFTs []Asset `json:"nfts"`
	NFT  Asset   `json:"nft"`
}

type errorResult struct {
	Errors []string `json:"errors"`
}

type assetsReceived struct {
	Assets []Asset
	Err    error
}

func paginateAssets(ctx context.Context, client *http.Client, req *http.Request, outCh chan assetsReceived) {
	paginateAssetsFilter(ctx, client, req, outCh, nil)
}

// paginatesAssetsFilter fetches assets from OpenSea and sends them to outCh. An optional keepAssetFilter can be provided to filter out an asset
// after it is fetched if keepAssetFilter evaluates to false. This is useful for filtering out assets that can't be filtered natively by the API.
func paginateAssetsFilter(ctx context.Context, client *http.Client, req *http.Request, outCh chan assetsReceived, keepAssetFilter func(a Asset) bool) {
	pages := 0
	for {
		resp, err := retry.RetryRequest(client, req)
		if err != nil {
			err = wrapRateLimitErr(ctx, err)
			logger.For(ctx).Errorf("failed to get tokens from opensea: %s", err)
			outCh <- assetsReceived{Err: err}
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			logger.For(ctx).Errorf("failed to get tokens from opensea: %s", ErrAPIKeyExpired)
			outCh <- assetsReceived{Err: ErrAPIKeyExpired}
			return
		}

		if resp.StatusCode >= http.StatusInternalServerError {
			logger.For(ctx).Errorf("internal server error from opensea: %s", util.BodyAsError(resp))
			outCh <- assetsReceived{Err: util.ErrHTTP{
				URL:    req.URL.String(),
				Status: resp.StatusCode,
				Err:    util.BodyAsError(resp),
			}}
		}

		if resp.StatusCode >= http.StatusBadRequest {
			var errResp errorResult

			if err := util.UnmarshallBody(&errResp, resp.Body); err != nil {
				logger.For(ctx).Errorf("failed to read response from opensea: %s", err)
				outCh <- assetsReceived{Err: err}
				return
			}

			if len(errResp.Errors) > 0 {
				err := fmt.Errorf(errResp.Errors[0])
				logger.For(ctx).Warnf("unable to find tokens from opensea: %s", err)
				outCh <- assetsReceived{Err: err}
				return
			}
		}

		page := pageResult{}

		if err := util.UnmarshallBody(&page, resp.Body); err != nil {
			logger.For(ctx).Errorf("failed to read response from opensea: %s", err)
			outCh <- assetsReceived{Err: err}
			return
		}

		// If we're paginating a single token, we'll get a single asset back
		if page.NFT.Identifier != "" {
			if keepAssetFilter != nil {
				if keepAssetFilter(page.NFT) {
					logger.For(ctx).Infof("got target token from page=%d from opensea", pages)
					outCh <- assetsReceived{Assets: []Asset{page.NFT}}
				}
				return
			}
			logger.For(ctx).Infof("got target tokens from page=%dfrom opensea", pages)
			outCh <- assetsReceived{Assets: []Asset{page.NFT}}
			return
		}

		// Otherwise, we're paginating a collection or contract
		if keepAssetFilter == nil {
			logger.For(ctx).Infof("got %d tokens from page=%d from opensea", len(page.NFTs), pages)
			outCh <- assetsReceived{Assets: page.NFTs}
		} else {
			filtered := util.Filter(page.NFTs, keepAssetFilter, true)
			logger.For(ctx).Infof("got %d tokens after filtering from page=%d from opensea", len(page.NFTs), pages)
			outCh <- assetsReceived{Assets: filtered}
		}
		if page.Next == "" {
			return
		}
		setNext(req.URL, page.Next)
		pages++
	}
}

func setNext(url *url.URL, next string) {
	query := url.Query()
	query.Set("next", next)
	url.RawQuery = query.Encode()
}

func setPagingParams(url *url.URL) {
	query := url.Query()
	query.Set("limit", strconv.Itoa(pageSize))
	url.RawQuery = query.Encode()
}

func contractNameIsSpam(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".lens-follower")
}

func mustChainIdentifierFrom(c persist.Chain) string {
	id, ok := chainToIdentifier[c]
	if !ok {
		panic(fmt.Sprintf("unknown chain identifier: %d", c))
	}
	return id
}

func wrapRateLimitErr(ctx context.Context, err error) error {
	if !util.ErrorIs[retry.ErrOutOfRetries](err) {
		return err
	}
	err = ErrOpenseaRateLimited{err}
	logger.For(ctx).Error(err)
	sentryutil.ReportError(ctx, err)
	return err
}
