package opensea

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sourcegraph/conc/pool"
)

func init() {
	env.RegisterValidation("OPENSEA_API_KEY", "required")
}

var ErrAPIKeyExpired = errors.New("opensea api key expired")

// TokenIdentifer represents a token identifer from Opensea. It is separate from persist.TokenID because opensea returns token IDs in base 10 format instead of what we want, base 16
type TokenIdentifer string

// Asset is an NFT from OpenSea
type Asset struct {
	Identifier    TokenIdentifer `json:"identifier"`
	Collection    string         `json:"collection"`
	Contract      string         `json:"contract"`
	TokenStandard string         `json:"token_standard"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ImageURL      string         `json:"image_url"`
	AnimationURL  string         `json:"animation_url"`
	MetadataURL   string         `json:"metadata_url"`
	Owners        []Owner        `json:"owners"`
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

type Provider struct {
	Chain      persist.Chain
	httpClient *http.Client
}

// NewProvider creates a new provider for OpenSea
func NewProvider(httpClient *http.Client, chain persist.Chain) *Provider {
	mustChainIdentifierFrom(chain) // validate that the chain is supported
	return &Provider{httpClient: httpClient, Chain: chain}
}

// GetBlockchainInfo returns Ethereum blockchain info
func (p *Provider) GetBlockchainInfo() multichain.BlockchainInfo {
	return multichain.BlockchainInfo{
		Chain:      p.Chain,
		ChainID:    0,
		ProviderID: "opensea",
	}
}

// GetTokensByWalletAddress returns a list of tokens for a wallet address
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, walletAddress persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.Chain, walletAddress, outCh)
	}()
	return p.assetsToTokens(ctx, walletAddress, outCh)
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for a wallet address
func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, walletAddress persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.Chain, walletAddress, outCh)
	}()
	go p.streamAssetsToTokens(ctx, walletAddress, outCh, recCh, errCh)
	return recCh, errCh
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForContract(ctx, p.httpClient, p.Chain, contractAddress, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	var contract multichain.ChainAgnosticContract
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

// GetTokensByContractAddressAndOwner returns a list of tokens for a contract address and owner
func (p *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, walletAddress, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForContractAddressAndOwner(ctx, p.httpClient, p.Chain, walletAddress, contractAddress, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, walletAddress, outCh)
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
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForToken(ctx, p.httpClient, p.GetBlockchainInfo().Chain, ti.ContractAddress, ti.TokenID, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, "", outCh)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, walletAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForTokenIdentifiersAndOwner(ctx, p.httpClient, p.Chain, walletAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()
	tokens, contracts, err := p.assetsToTokens(ctx, walletAddress, outCh)
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
	tokens, _, err := p.GetTokensByTokenIdentifiers(ctx, ti, 1, 0)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens found for %s", ti)
	}

	return tokens[0].TokenMetadata, nil
}

// GetContractByAddress returns a contract for a contract address
func (p *Provider) GetContractByAddress(ctx context.Context, contractAddress persist.Address) (multichain.ChainAgnosticContract, error) {
	cc, err := fetchContractCollectionByAddress(ctx, p.httpClient, p.Chain, contractAddress)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return contractToChainAgnosticContract(cc.Contract, cc.Collection), nil
}

func (p *Provider) assetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	resultTokens := make([]multichain.ChainAgnosticToken, 0, len(outCh))
	seenContracts := &sync.Map{} // Mapping of contract address to contractCollection
	resultContracts := make([]multichain.ChainAgnosticContract, 0, len(outCh))
	tokensChan := make(chan multichain.ChainAgnosticToken)
	contractsChan := make(chan multichain.ChainAgnosticContract)
	contractLocks := make(map[string]*sync.Mutex)
	mu := sync.RWMutex{}
	errChan := make(chan error)
	wp := pool.New().WithMaxGoroutines(10).WithContext(ctx)

	go func() {
		defer close(tokensChan)
		defer close(contractsChan)
		for a := range outCh {
			assetsReceived := a
			if assetsReceived.Err != nil {
				errChan <- assetsReceived.Err
				return
			}
			for _, n := range assetsReceived.Assets {
				nft := n
				wp.Go(func(ctx context.Context) error {
					return p.streamTokenAndContract(ctx, ownerAddress, nft, tokensChan, contractsChan, seenContracts, contractLocks, &mu)
				})
			}
		}
		err := wp.Wait()
		if err != nil {
			errChan <- err
		}
	}()

	var tokenOpen, contractOpen bool
	var token multichain.ChainAgnosticToken
	var contract multichain.ChainAgnosticContract

	for {
		select {
		case token, tokenOpen = <-tokensChan:
			if tokenOpen {
				resultTokens = append(resultTokens, token)
				continue
			}
			if !contractOpen {
				return resultTokens, resultContracts, nil
			}
		case contract, contractOpen = <-contractsChan:
			if contractOpen {
				resultContracts = append(resultContracts, contract)
				continue
			}
			if !tokenOpen {
				return resultTokens, resultContracts, nil
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case err := <-errChan:
			return nil, nil, err
		}
	}
}

func (p *Provider) streamAssetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived, rec chan<- multichain.ChainAgnosticTokensAndContracts, errChan chan<- error) {
	seenContracts := &sync.Map{}
	mu := sync.RWMutex{}
	contractLocks := make(map[string]*sync.Mutex)
	for a := range outCh {
		assetsReceived := a

		if assetsReceived.Err != nil {
			errChan <- assetsReceived.Err
			return
		}

		innerTokens := make([]multichain.ChainAgnosticToken, 0, len(assetsReceived.Assets))
		innerContracts := make([]multichain.ChainAgnosticContract, 0, len(assetsReceived.Assets))
		innerTokenReceived := make(chan multichain.ChainAgnosticToken)
		innerContractReceived := make(chan multichain.ChainAgnosticContract)
		innerErrChan := make(chan error)

		go func() {
			wp := pool.New().WithMaxGoroutines(10).WithContext(ctx)
			defer close(innerTokenReceived)
			defer close(innerContractReceived)
			for _, n := range assetsReceived.Assets {
				nft := n
				wp.Go(func(ctx context.Context) error {
					return p.streamTokenAndContract(ctx, ownerAddress, nft, innerTokenReceived, innerContractReceived, seenContracts, contractLocks, &mu)
				})
			}
			err := wp.Wait()
			if err != nil {
				innerErrChan <- err
			}
		}()

		var tokenOpen, contractOpen bool
		var token multichain.ChainAgnosticToken
		var contract multichain.ChainAgnosticContract

	outer:
		for {
			select {
			case token, tokenOpen = <-innerTokenReceived:
				if tokenOpen {
					innerTokens = append(innerTokens, token)
					continue
				}
				if !contractOpen {
					break outer
				}
			case contract, contractOpen = <-innerContractReceived:
				if contractOpen {
					innerContracts = append(innerContracts, contract)
					continue
				}
				if !tokenOpen {
					break outer
				}
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case err := <-innerErrChan:
				logger.For(ctx).Error(err)
				errChan <- err
				return
			}
		}

		logger.For(ctx).Infof("incrementally received %d tokens from opensea, sending to receiver", len(innerTokens))
		rec <- multichain.ChainAgnosticTokensAndContracts{
			Tokens:    innerTokens,
			Contracts: innerContracts,
		}
	}
}

type contractCollection struct {
	Contract   Contract
	Collection Collection
}

func (p *Provider) streamTokenAndContract(ctx context.Context, ownerAddress persist.Address, asset Asset, tokenCh chan<- multichain.ChainAgnosticToken, contractCh chan<- multichain.ChainAgnosticContract, seenContracts *sync.Map, contractLocks map[string]*sync.Mutex, mu *sync.RWMutex) error {
	// Haven't seen this contract before, so we need to process it
	if _, ok := seenContracts.Load(asset.Contract); !ok {
		// If we don't have a lock yet, create one
		mu.Lock()
		if _, hasJobLock := contractLocks[asset.Contract]; !hasJobLock {
			contractLocks[asset.Contract] = &sync.Mutex{}
		}
		mu.Unlock()

		// Acquire a lock for the job
		mu.RLock()
		jobMu := contractLocks[asset.Contract]
		mu.RUnlock()

		// Process the contract
		var err error
		func() {
			jobMu.Lock()
			defer jobMu.Unlock()
			// Check again if we've seen the contract, since another job may have processed it while we were waiting for the lock
			if _, ok := seenContracts.Load(asset.Contract); !ok {
				c, err := fetchContractCollectionByAddress(ctx, p.httpClient, p.Chain, persist.Address(asset.Contract))
				if err == nil {
					contractCh <- contractToChainAgnosticContract(c.Contract, c.Collection)
					seenContracts.Store(asset.Contract, c)
				}
			}
		}()

		if err != nil {
			return err
		}
	}

	cc, _ := seenContracts.Load(asset.Contract)
	collection := cc.(contractCollection).Collection

	typ, err := tokenTypeFromAsset(asset)
	if err != nil {
		return err
	}

	var tokens []multichain.ChainAgnosticToken

	if ownerAddress != "" {
		tokenCh <- assetToToken(asset, collection, typ, ownerAddress, 1)
		return nil
	}

	// There are some types of syncs like syncing tokens by contract address or syncing tokens by token identifiers
	// where we can't confirm the owner via the API. If there isn't an input ownerAddress, we expand the token to all owners of the token.
	switch typ {
	case persist.TokenTypeERC721:
		if numOwners := len(asset.Owners); numOwners != 1 {
			return fmt.Errorf("unexpected number of owners for ERC721 token %s:%s: expected 1, got %d", asset.Contract, asset.Identifier, numOwners)
		}
		tokens = []multichain.ChainAgnosticToken{assetToToken(asset, collection, typ, persist.Address(asset.Owners[0].Address), 1)}
	case persist.TokenTypeERC1155:
		tokens = util.MapWithoutError(asset.Owners, func(o Owner) multichain.ChainAgnosticToken {
			return assetToToken(asset, collection, typ, persist.Address(o.Address), o.Quantity)
		})
	default:
		return fmt.Errorf("don't know how to handle owners for token type %s", typ)
	}

	for _, token := range tokens {
		tokenCh <- token
	}

	return nil
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
	endpoint, err := getContractEndpoint(chain, contractAddress)
	if err != nil {
		return Contract{}, err
	}

	resp, err := retry.RetryRequest(client, authRequest(ctx, endpoint))
	if err != nil {
		return Contract{}, err
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
	endpoint, err := getCollectionEndpoint(slug)
	if err != nil {
		return Collection{}, err
	}

	resp, err := retry.RetryRequest(client, authRequest(ctx, endpoint))
	if err != nil {
		return Collection{}, err
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

var (
	baseURL, _                        = url.Parse("https://api.opensea.io/api/v2")
	getContractEndpointTemplate       = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s")
	getCollectionEndpointTemplate     = fmt.Sprintf("%s/%s", baseURL.String(), "collections/%s")
	getNftEndpointTemplate            = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s/nfts/%s")
	getNftsByWalletEndpointTemplate   = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/account/%s/nfts")
	getNftsByContractEndpointTemplate = fmt.Sprintf("%s/%s", baseURL.String(), "chain/%s/contract/%s/nfts")
)

func getContractEndpoint(chain persist.Chain, address persist.Address) (*url.URL, error) {
	s := fmt.Sprintf(getContractEndpointTemplate, mustChainIdentifierFrom(chain), address)
	return url.Parse(s)
}

func getCollectionEndpoint(slug string) (*url.URL, error) {
	s := fmt.Sprintf(getCollectionEndpointTemplate, slug)
	return url.Parse(s)
}

func getNftEndpoint(chain persist.Chain, address persist.Address, tokenID persist.TokenID) (*url.URL, error) {
	s := fmt.Sprintf(getNftEndpointTemplate, mustChainIdentifierFrom(chain), address, tokenID.Base10String())
	return url.Parse(s)
}

func getNftsByWalletEndpoint(chain persist.Chain, address persist.Address) (*url.URL, error) {
	s := fmt.Sprintf(getNftsByWalletEndpointTemplate, mustChainIdentifierFrom(persist.ChainETH), address)
	return url.Parse(s)
}

func getNftsByContractEndpoint(chain persist.Chain, address persist.Address) (*url.URL, error) {
	s := fmt.Sprintf(getNftsByContractEndpointTemplate, mustChainIdentifierFrom(chain), address)
	return url.Parse(s)
}

func streamAssetsForToken(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, tokenID persist.TokenID, outCh chan assetsReceived) {
	endpoint, err := getNftEndpoint(chain, address, tokenID)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	paginateAssets(ctx, client, authRequest(ctx, endpoint), outCh)
}

func streamAssetsForWallet(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint, err := getNftsByWalletEndpoint(chain, address)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	setPagingParams(endpoint)
	paginateAssets(ctx, client, authRequest(ctx, endpoint), outCh)
}

func streamAssetsForContract(ctx context.Context, client *http.Client, chain persist.Chain, address persist.Address, outCh chan assetsReceived) {
	endpoint, err := getNftsByContractEndpoint(chain, address)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	setPagingParams(endpoint)
	paginateAssets(ctx, client, authRequest(ctx, endpoint), outCh)
}

func streamAssetsForContractAddressAndOwner(ctx context.Context, client *http.Client, chain persist.Chain, ownerAddress, contractAddress persist.Address, outCh chan assetsReceived) {
	contract, err := fetchContractByAddress(ctx, client, chain, contractAddress)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	endpoint, err := getNftsByWalletEndpoint(chain, ownerAddress)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	setCollection(endpoint, contract.Collection)
	setPagingParams(endpoint)
	paginateAssets(ctx, client, authRequest(ctx, endpoint), outCh)
}

func streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, client *http.Client, chain persist.Chain, ownerAddress, contractAddress persist.Address, tokenID persist.TokenID, outCh chan assetsReceived) {
	contract, err := fetchContractByAddress(ctx, client, chain, contractAddress)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	endpoint, err := getNftsByWalletEndpoint(chain, ownerAddress)
	if err != nil {
		outCh <- assetsReceived{Err: err}
		return
	}
	setCollection(endpoint, contract.Collection)
	setPagingParams(endpoint)
	paginateAssetsFilter(ctx, client, authRequest(ctx, endpoint), outCh, func(a Asset) bool {
		// OS doesn't let you filter for a specific token ID, so we have exclude everything that isn't the token we want
		return a.Contract != contractAddress.String() && mustTokenID(a.Identifier) != tokenID
	})
}

type unknownTokenStandard struct {
	TokenStandard string
}

func (e unknownTokenStandard) Error() string {
	return fmt.Sprintf("unknown token standard: %s", e.TokenStandard)
}

func tokenTypeFromAsset(asset Asset) (persist.TokenType, error) {
	switch asset.TokenStandard {
	case "erc721", "cryptopunks":
		return persist.TokenTypeERC721, nil
	case "erc1155":
		return persist.TokenTypeERC1155, nil
	default:
		return "", unknownTokenStandard{asset.TokenStandard}
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

func assetToToken(asset Asset, collection Collection, tokenType persist.TokenType, tokenOwner persist.Address, quantity int) multichain.ChainAgnosticToken {
	return multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		Descriptors:     multichain.ChainAgnosticTokenDescriptors{Name: asset.Name, Description: asset.Description},
		TokenURI:        persist.TokenURI(asset.MetadataURL),
		TokenID:         mustTokenID(asset.Identifier),
		OwnerAddress:    tokenOwner,
		FallbackMedia:   persist.FallbackMedia{ImageURL: persist.NullString(asset.ImageURL)},
		ContractAddress: persist.Address(asset.Contract),
		ExternalURL:     collection.ProjectURL, // OpenSea doesn't return a token-specific external URL, so we use the collection's project URL
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

// authRequest returns a http.Request with authorization headers
func authRequest(ctx context.Context, url *url.URL) *http.Request {
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
	for {
		resp, err := retry.RetryRequest(client, req)
		if err != nil {
			outCh <- assetsReceived{Err: err}
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			outCh <- assetsReceived{Err: ErrAPIKeyExpired}
			return
		}

		if resp.StatusCode != http.StatusOK {
			outCh <- assetsReceived{Err: util.BodyAsError(resp)}
			return
		}

		page := pageResult{}
		if err := util.UnmarshallBody(&page, resp.Body); err != nil {
			outCh <- assetsReceived{Err: err}
			return
		}

		logger.For(ctx).Infof("received %d assets from opensea query %s", len(page.NFTs), req.URL.RawQuery)

		if keepAssetFilter == nil {
			outCh <- assetsReceived{Assets: page.NFTs}
		} else {
			outCh <- assetsReceived{Assets: util.Filter(page.NFTs, keepAssetFilter, true)}
		}

		if page.Next == "" {
			return
		}

		setNext(req.URL, page.Next)
	}
}

func setNext(url *url.URL, next string) {
	query := url.Query()
	query.Set("next", next)
	url.RawQuery = query.Encode()
}

func setPagingParams(url *url.URL) {
	query := url.Query()
	query.Set("limit", "200")
	url.RawQuery = query.Encode()
}

func setCollection(url *url.URL, slug string) {
	query := url.Query()
	query.Set("collection", slug)
	url.RawQuery = query.Encode()
}

func contractNameIsSpam(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".lens-follower")
}

var chainToIdentifier = map[persist.Chain]string{
	persist.ChainETH:      "ethereum",
	persist.ChainPolygon:  "matic",
	persist.ChainOptimism: "optimism",
	persist.ChainArbitrum: "arbitrum",
}

func mustChainIdentifierFrom(c persist.Chain) string {
	id, ok := chainToIdentifier[c]
	if !ok {
		panic(fmt.Sprintf("unknown chain identifier: %d", c))
	}
	return id
}

func mustTokenID(t TokenIdentifer) persist.TokenID {
	asBig, ok := new(big.Int).SetString(string(t), 10)
	if !ok {
		panic("failed to convert opensea token identifier to big int")
	}
	return persist.TokenID(asBig.Text(16))
}
