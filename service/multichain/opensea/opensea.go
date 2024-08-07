package opensea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/sourcegraph/conc/pool"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

func init() {
	env.RegisterValidation("OPENSEA_API_KEY", "required")
}

const (
	pageSize = 100
	poolSize = 12
)

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
	Name        string `json:"name"`
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

type Provider struct {
	Chain persist.Chain
	r     *retry.Retryer
}

// NewProvider creates a new provider for OpenSea
func NewProvider(ctx context.Context, httpClient *http.Client, chain persist.Chain, l retry.Limiter) (*Provider, func()) {
	mustChainIdentifierFrom(chain)
	r, cleanup := retry.New(l, httpClient)
	return &Provider{Chain: chain, r: r}, cleanup
}

// GetTokensByWalletAddress returns a list of tokens for an address
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, ownerAddress persist.Address) ([]common.ChainAgnosticToken, []common.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.r, p.Chain, ownerAddress, outCh)
	}()
	return p.assetsToTokens(ctx, ownerAddress, outCh)
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for an address
func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, ownerAddress persist.Address) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	outCh := make(chan assetsReceived, 32)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.r, p.Chain, ownerAddress, outCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, ownerAddress, outCh, recCh, errCh)
	}()
	return recCh, errCh
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	assetsCh := make(chan assetsReceived)
	go func() {
		defer close(assetsCh)
		streamAssetsForContract(ctx, p.r, p.Chain, address, assetsCh)
	}()
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, address, assetsCh, recCh, errCh)
	}()
	return recCh, errCh
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForContract(ctx, p.r, p.Chain, contractAddress, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	var contract common.ChainAgnosticContract
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

// GetTokensByTokenIdentifiers returns a list of tokens for a list of token identifiers
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) ([]common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForToken(ctx, p.r, p.Chain, ti.ContractAddress, ti.TokenID, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh)
	if err != nil {
		return nil, common.ChainAgnosticContract{}, err
	}
	contract := common.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti common.ChainAgnosticIdentifiers, ownerAddress persist.Address) (common.ChainAgnosticToken, common.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForTokenIdentifiersAndOwner(ctx, p.r, p.Chain, ownerAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()

	tokens, contracts, err := p.assetsToTokens(ctx, ownerAddress, outCh)
	if err != nil {
		return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, err
	}

	contract := common.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}

	if len(tokens) > 0 {
		return tokens[0], contract, nil
	}
	return common.ChainAgnosticToken{}, common.ChainAgnosticContract{}, fmt.Errorf("no tokens found for %s", ti)
}

// GetTokenMetadataByTokenIdentifiers retrieves a token's metadata for a given contract address and token ID
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	tokens, _, err := p.GetTokensByTokenIdentifiers(ctx, ti)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens found for %s", ti)
	}

	return tokens[0].TokenMetadata, nil
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, ti common.ChainAgnosticIdentifiers) (common.ChainAgnosticTokenDescriptors, common.ChainAgnosticContractDescriptors, error) {
	tokens, contract, err := p.GetTokensByTokenIdentifiers(ctx, ti)
	if err != nil {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, err
	}

	if len(tokens) == 0 {
		return common.ChainAgnosticTokenDescriptors{}, common.ChainAgnosticContractDescriptors{}, fmt.Errorf("no tokens found for %s", ti)
	}

	return tokens[0].Descriptors, contract.Descriptors, nil
}

// GetContractByAddress returns a contract for a contract address
func (p *Provider) GetContractByAddress(ctx context.Context, contractAddress persist.Address) (common.ChainAgnosticContract, error) {
	cc, err := fetchContractCollectionByAddress(ctx, p.r, p.Chain, contractAddress)
	if err != nil {
		return common.ChainAgnosticContract{}, err
	}
	return contractCollectionToChainAgnosticContract(cc), nil
}

func (p *Provider) assetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived) (tokens []common.ChainAgnosticToken, contracts []common.ChainAgnosticContract, err error) {
	recCh := make(chan common.ChainAgnosticTokensAndContracts, poolSize)
	errCh := make(chan error)
	go func() {
		defer close(recCh)
		defer close(errCh)
		p.streamAssetsToTokens(ctx, ownerAddress, outCh, recCh, errCh)
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
	recCh chan<- common.ChainAgnosticTokensAndContracts,
	errCh chan<- error,
) {
	cachedContracts := &sync.Map{}                      // used to avoid duplicate contract fetches
	contractsL := make(map[persist.Address]*sync.Mutex) // contract job locks
	mu := sync.RWMutex{}                                // manages access to contract locks

	for page := range outCh {
		page := page
		wp := pool.New().WithContext(ctx).WithMaxGoroutines(1) // bump this back up when rate limits are increased

		if page.Err != nil {
			errCh <- wrapMissingContractErr(p.Chain, page.Err)
			return
		}

		if len(page.Assets) == 0 {
			continue
		}

		var out common.ChainAgnosticTokensAndContracts

		// fetch contracts
		addresses := make([]persist.Address, len(page.Assets))
		for i, asset := range page.Assets {
			addr := persist.Address(asset.Contract)
			addresses[i] = addr
			wp.Go(func(context.Context) error {
				return p.getChainAgnosticContract(ctx, addr, asset.Collection, cachedContracts, contractsL, &mu)
			})
		}

		err := wp.Wait()
		if err != nil {
			errCh <- err
			return
		}

		for _, asset := range page.Assets {
			addr := persist.Address(asset.Contract)
			contract, ok := cachedContracts.Load(addr)
			if !ok {
				panic("contract should have been loaded")
			}

			tokens, err := p.assetToChainAgnosticTokens(ownerAddress, asset)
			if err != nil {
				errCh <- err
				return
			}

			out.Tokens = append(out.Tokens, tokens...)
			out.Contracts = append(out.Contracts, contract.(common.ChainAgnosticContract))
		}

		recCh <- out
	}
}

type contractCollection struct {
	Contract   Contract
	Collection Collection
}

func (p *Provider) getChainAgnosticContract(ctx context.Context, contractAddress persist.Address, collectionSlug string, seenContracts *sync.Map, contractLocks map[persist.Address]*sync.Mutex, mu *sync.RWMutex) error {
	_, ok := seenContracts.Load(contractAddress)
	// already have the contract
	if ok {
		return nil
	}

	// If we don't have a lock yet, create one
	mu.Lock()
	if _, hasJobLock := contractLocks[contractAddress]; !hasJobLock {
		contractLocks[contractAddress] = &sync.Mutex{}
	}
	mu.Unlock()

	// Acquire a lock for the job
	mu.RLock()
	jobMu := contractLocks[contractAddress]
	mu.RUnlock()

	// Process the contract
	jobMu.Lock()
	defer jobMu.Unlock()

	// Check again if we've seen the contract, since another job may have processed it while we were waiting for the lock
	_, ok = seenContracts.Load(contractAddress)
	if ok {
		return nil
	}

	contractCollection, err := fetchContractCollectionByAddress(ctx, p.r, p.Chain, contractAddress)
	if err != nil {
		return err
	}

	seenContracts.Store(contractAddress, contractCollectionToChainAgnosticContract(contractCollection))
	return nil
}

func (p *Provider) assetToChainAgnosticTokens(ownerAddress persist.Address, asset Asset) ([]common.ChainAgnosticToken, error) {
	typ, err := tokenTypeFromAsset(asset)
	if err != nil {
		return nil, err
	}

	if ownerAddress != "" {
		token := assetToAgnosticToken(asset, typ, ownerAddress, 1)
		return []common.ChainAgnosticToken{token}, nil
	}

	// No input owner address provided. OS doesn't return the owner of the token a few endpoints
	// like when paginating a contract or a collection. The owner isn't really important for our purposes,
	// but we'll to add it to the token if its already available.

	if typ == persist.TokenTypeERC721 && len(asset.Owners) == 1 {
		token := assetToAgnosticToken(asset, typ, persist.Address(asset.Owners[0].Address), 1)
		return []common.ChainAgnosticToken{token}, nil
	}

	if typ == persist.TokenTypeERC1155 && len(asset.Owners) > 0 {
		tokens := make([]common.ChainAgnosticToken, len(asset.Owners))
		for i, o := range asset.Owners {
			tokens[i] = assetToAgnosticToken(asset, typ, persist.Address(o.Address), o.Quantity)
		}
		return tokens, nil
	}

	token := assetToAgnosticToken(asset, typ, "", 1)
	return []common.ChainAgnosticToken{token}, nil
}

func fetchContractCollectionByAddress(ctx context.Context, r *retry.Retryer, chain persist.Chain, contractAddress persist.Address) (contractCollection, error) {
	contract, err := fetchContractByAddress(ctx, r, chain, contractAddress)
	if err != nil {
		return contractCollection{}, err
	}
	collection, err := fetchCollectionBySlug(ctx, r, chain, contract.Collection)
	if err != nil {
		return contractCollection{}, err
	}
	return contractCollection{contract, collection}, nil
}

func fetchContractByAddress(ctx context.Context, r *retry.Retryer, chain persist.Chain, contractAddress persist.Address) (contract Contract, err error) {
	endpoint := mustContractEndpoint(chain, contractAddress)
	resp, err := r.Do(mustAuthRequest(ctx, endpoint))
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

func fetchCollectionBySlug(ctx context.Context, r *retry.Retryer, chain persist.Chain, slug string) (collection Collection, err error) {
	endpoint := mustCollectionEndpoint(slug)
	resp, err := r.Do(mustAuthRequest(ctx, endpoint))
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

func mustNftEndpoint(chain persist.Chain, address persist.Address, tokenID persist.HexTokenID) *url.URL {
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

func streamAssetsForToken(ctx context.Context, r *retry.Retryer, chain persist.Chain, address persist.Address, tokenID persist.HexTokenID, outCh chan assetsReceived) {
	endpoint := mustNftEndpoint(chain, address, tokenID)
	setPagingParams(endpoint)
	paginateAssets(ctx, r, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForWallet(ctx context.Context, r *retry.Retryer, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint := mustNftsByWalletEndpoint(chain, address)
	setPagingParams(endpoint)
	paginateAssets(ctx, r, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForContract(ctx context.Context, r *retry.Retryer, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint := mustNftsByContractEndpoint(chain, address)
	setPagingParams(endpoint)
	paginateAssets(ctx, r, mustAuthRequest(ctx, endpoint), outCh)
}

func streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, r *retry.Retryer, chain persist.Chain, ownerAddress, contractAddress persist.Address, tokenID persist.HexTokenID, outCh chan assetsReceived) {
	endpoint := mustNftsByWalletEndpoint(chain, ownerAddress)
	setPagingParams(endpoint)
	paginateAssetsFilter(ctx, r, mustAuthRequest(ctx, endpoint), outCh, func(a Asset) bool {
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

func contractCollectionToChainAgnosticContract(contractCollection contractCollection) common.ChainAgnosticContract {
	desc := common.ChainAgnosticContractDescriptors{
		Symbol:       "", // OpenSea doesn't provide this, but it isn't exposed in the schema anyway
		Name:         contractCollection.Contract.Name,
		OwnerAddress: persist.Address(contractCollection.Collection.Owner),
	}
	return common.ChainAgnosticContract{
		Address:     contractCollection.Contract.Address,
		Descriptors: desc,
		IsSpam:      util.ToPointer(contractNameIsSpam(contractCollection.Contract.Name)),
	}
}

func assetToAgnosticToken(asset Asset, tokenType persist.TokenType, tokenOwner persist.Address, quantity int) common.ChainAgnosticToken {
	return common.ChainAgnosticToken{
		TokenType:       tokenType,
		Descriptors:     common.ChainAgnosticTokenDescriptors{Name: asset.Name, Description: asset.Description},
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

func wrapMissingContractErr(chain persist.Chain, err error) error {
	errMsg := err.Error()
	if strings.HasPrefix(errMsg, "Contract") && strings.HasSuffix(errMsg, "not found") {
		a := strings.TrimPrefix(errMsg, "Contract ")
		a = strings.TrimSuffix(a, " not found")
		a = strings.TrimSpace(a)
		return fmt.Errorf("%s not found: %s", persist.ContractIdentifiers{
			Chain:           chain,
			ContractAddress: persist.Address(a)}, err)
	}
	return err
}

func readErrBody(ctx context.Context, body io.Reader) error {
	b := new(bytes.Buffer)

	_, err := b.ReadFrom(body)
	if err != nil {
		return err
	}

	byt := b.Bytes()

	var errResp errorResult

	err = json.Unmarshal(byt, &errResp)
	if err != nil {
		return err
	}

	if len(errResp.Errors) > 0 {
		err = fmt.Errorf(errResp.Errors[0])
		return err
	}

	return fmt.Errorf(string(byt))
}

func paginateAssets(ctx context.Context, r *retry.Retryer, req *http.Request, outCh chan assetsReceived) {
	paginateAssetsFilter(ctx, r, req, outCh, nil)
}

// paginatesAssetsFilter fetches assets from OpenSea and sends them to outCh. An optional keepAssetFilter can be provided to filter out an asset
// after it is fetched if keepAssetFilter evaluates to false. This is useful for filtering out assets that can't be filtered natively by the API.
func paginateAssetsFilter(ctx context.Context, r *retry.Retryer, req *http.Request, outCh chan assetsReceived, keepAssetFilter func(a Asset) bool) {
	pages := 0
	for {
		resp, err := r.Do(req)
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
			err = readErrBody(ctx, resp.Body)
			logger.For(ctx).Errorf("unexpected status code (%d) from opensea: %s", resp.StatusCode, err)
			outCh <- assetsReceived{Err: err}
			return
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
					logger.For(ctx).Info("got target token from opensea")
					outCh <- assetsReceived{Assets: []Asset{page.NFT}}
				}
				return
			}
			logger.For(ctx).Info("got target token from opensea")
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
	if !errors.Is(err, retry.ErrOutOfRetries) {
		return err
	}
	err = ErrOpenseaRateLimited{err}
	logger.For(ctx).Error(err)
	sentryutil.ReportError(ctx, err)
	return err
}
