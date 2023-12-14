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

type Provider struct {
	httpClient *http.Client
	chain      persist.Chain
}

// TokenIdentifer represents a token identifer from Opensea. It is separate from persist.TokenID because opensea returns token IDs in base 10 format instead of what we want, base 16
type TokenIdentifer string

// TokenID coverts an OpenSea token identifier to a persist.TokenID and panics if it fails
func (t TokenIdentifer) MustTokenID() persist.TokenID {
	asBig, ok := new(big.Int).SetString(string(t), 10)
	if !ok {
		panic("failed to convert opensea token id to big int")
	}
	return persist.TokenID(asBig.Text(16))
}

// Asset is an NFT from OpenSea
type Asset struct {
	Identifier    TokenIdentifer `json:"identifier"`
	Collection    string         `json:"collection"`
	Contract      string         `json:"contract"`
	TokenStandard string         `json:"token_standard"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ImageURL      string         `json:"image_url"`
	MetadataURL   string         `json:"metadata_url"`
	OpenSeaURL    string         `json:"opensea_url"`
	IsDisabled    bool           `json:"is_disabled"`
	IsNSFW        bool           `json:"is_nsfw"`
}

// Collection is a collection from OpenSea
type Collection struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	ImageURL    string `json:"image_url"`
}

// Contract represents an NFT contract from Opensea
type Contract struct {
	Address         persist.Address `json:"address"`
	ChainIdentifier ChainIdentifier `json:"chain_identifier"`
	Collection      string          `json:"collection"`
	Name            string          `json:"name"`
	Supply          int             `json:"supply"`
}

type ChainIdentifier string

const (
	ChainIdentifierEthereum ChainIdentifier = "ethereum"
	ChainIdentifierPolygon  ChainIdentifier = "matic"
	ChainIdentifierOptimism ChainIdentifier = "optimism"
	ChainIdentifierArbitrum ChainIdentifier = "arbitrum"
)

func (c ChainIdentifier) String() string {
	return string(c)
}

func (c ChainIdentifier) Chain() persist.Chain {
	switch c {
	case ChainIdentifierEthereum:
		return persist.ChainETH
	case ChainIdentifierPolygon:
		return persist.ChainPolygon
	case ChainIdentifierOptimism:
		return persist.ChainOptimism
	case ChainIdentifierArbitrum:
		return persist.ChainArbitrum
	default:
		panic(fmt.Sprintf("unknown chain identifier: %s", c))
	}
}

var chainToChainIndentifier = map[persist.Chain]ChainIdentifier{
	persist.ChainETH: ChainIdentifierEthereum,
}

func mustChainIdentifierFrom(c persist.Chain) ChainIdentifier {
	id, ok := chainToChainIndentifier[c]
	if !ok {
		panic(fmt.Sprintf("unknown chain identifier: %s", c))
	}
	return id
}

// NewProvider creates a new provider for OpenSea
func NewProvider(httpClient *http.Client, chain persist.Chain) *Provider {
	mustChainIdentifierFrom(chain) // validate that the chain is supported
	return &Provider{httpClient: httpClient, chain: chain}
}

// GetBlockchainInfo returns Ethereum blockchain info
func (p *Provider) GetBlockchainInfo() multichain.BlockchainInfo {
	return multichain.BlockchainInfo{
		Chain:      p.chain,
		ChainID:    0,
		ProviderID: "opensea",
	}
}

// GetTokensByWalletAddress returns a list of tokens for a wallet address
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, walletAddress persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.chain, walletAddress, outCh)
	}()
	return assetsToTokens(ctx, walletAddress, outCh)
}

// GetTokensIncrementallyByWalletAddress returns a list of tokens for a wallet address
func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, walletAddress persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	recCh := make(chan multichain.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForWallet(ctx, p.httpClient, p.chain, walletAddress, outCh)
	}()
	go streamAssetsToTokens(ctx, walletAddress, outCh, recCh, errCh)
	return recCh, errCh
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	outCh := make(chan assetsReceived)
	go func() {
		defer close(outCh)
		streamAssetsForContract(ctx, p.httpClient, p.chain, contractAddress, outCh)
	}()
	tokens, contracts, err := assetsToTokens(ctx, "", outCh)
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
		streamAssetsForContractAddressAndOwner(ctx, p.httpClient, p.chain, walletAddress, contractAddress, outCh)
	}()
	tokens, contracts, err := assetsToTokens(ctx, walletAddress, outCh)
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
	tokens, contracts, err := assetsToTokens(ctx, "", outCh)
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
		streamAssetsForTokenIdentifiersAndOwner(ctx, p.httpClient, p.chain, walletAddress, ti.ContractAddress, ti.TokenID, outCh)
	}()
	tokens, contracts, err := assetsToTokens(ctx, walletAddress, outCh)
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
	contract, err := fetchContractByAddress(ctx, p.httpClient, p.chain, contractAddress)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	collection, err := fetchCollectionBySlug(ctx, p.httpClient, p.chain, contract.Collection)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	desc := multichain.ChainAgnosticContractDescriptors{
		Symbol:       "TODO",
		Name:         contract.Name,
		Description:  collection.Description,
		OwnerAddress: persist.Address(collection.Owner),
	}

	return multichain.ChainAgnosticContract{
		Address:     persist.Address(contract.Address.String()),
		Descriptors: desc,
		IsSpam:      util.ToPointer(contractNameIsSpam(contract.Name)),
	}, nil
}

func (d *Provider) GetOwnedTokensByContract(context.Context, persist.Address, persist.Address, int, int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return []multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, nil
}

func (d *Provider) GetDisplayNameByAddress(ctx context.Context, addr persist.Address) string {
	return addr.String()
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

func fetchContractByAddress(ctx context.Context, client *http.Client, chain persist.Chain, contractAddress persist.Address) (Contract, error) {
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
		return Contract{}, util.ErrHTTP{
			URL:    endpoint.String(),
			Status: resp.StatusCode,
			Err:    util.BodyAsError(resp),
		}
	}

	contract := Contract{}
	err = util.UnmarshallBody(&contract, resp.Body)
	if err != nil {
		return Contract{}, err
	}

	return contract, nil
}

func fetchCollectionBySlug(ctx context.Context, client *http.Client, chain persist.Chain, slug string) (Collection, error) {
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
		return Collection{}, util.ErrHTTP{
			URL:    endpoint.String(),
			Status: resp.StatusCode,
			Err:    util.BodyAsError(resp),
		}
	}

	collection := Collection{}
	err = util.UnmarshallBody(&collection, resp.Body)
	if err != nil {
		return Collection{}, err
	}

	return collection, nil
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
	// OS doesn't let you filter for a specific token ID, so we have to filter it ourselves
	setCollection(endpoint, contract.Collection)
	setPagingParams(endpoint)
	paginateAssetsFilter(ctx, client, authRequest(ctx, endpoint), outCh, func(a Asset) bool {
		return a.Contract != contractAddress.String() && a.Identifier.MustTokenID() != tokenID
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

func assetToToken(asset Asset, tokenOwner persist.Address, externalURL string) (multichain.ChainAgnosticToken, error) {
	tokenType, err := tokenTypeFromAsset(asset)
	if err != nil {
		return multichain.ChainAgnosticToken{}, err
	}
	return multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		Descriptors:     multichain.ChainAgnosticTokenDescriptors{Name: asset.Name, Description: asset.Description},
		TokenURI:        persist.TokenURI(asset.MetadataURL),
		TokenID:         asset.Identifier.MustTokenID(),
		OwnerAddress:    tokenOwner,
		FallbackMedia:   persist.FallbackMedia{ImageURL: persist.NullString(asset.ImageURL)},
		ContractAddress: persist.Address(asset.Contract),
		ExternalURL:     externalURL,
		TokenMetadata:   metadataFromAsset(asset),
		Quantity:        "1",
	}, nil
}

func metadataFromAsset(asset Asset) persist.TokenMetadata {
	m := persist.TokenMetadata{
		"name":        asset.Name,
		"description": asset.Description,
		"image_url":   asset.ImageURL,
	}
	// ENS
	if persist.Address(asset.Contract) == eth.EnsAddress {
		m["profile_image"] = fmt.Sprintf("https://metadata.ens.domains/mainnet/avatar/%s", asset.Name)
	}
	return m
}

func contractFromAddress(contractAddress persist.Address) multichain.ChainAgnosticContract {
	isSpam := contractNameIsSpam(asset.Contract.Name)
	return multichain.ChainAgnosticContract{
		Address: persist.Address(asset.Contract.Address.String()),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Symbol:          asset.Contract.Symbol,
			Name:            asset.Contract.Name,
			OwnerAddress:    persist.Address(asset.Collection.PayoutAddress),
			Description:     asset.Collection.Description,
			ProfileImageURL: asset.Collection.ImageURL,
		},
		IsSpam: &isSpam,
	}
}

func assetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	resultTokens := make([]multichain.ChainAgnosticToken, 0, len(outCh))
	seenContracts := &sync.Map{}
	resultContracts := make([]multichain.ChainAgnosticContract, 0, len(outCh))
	tokensChan := make(chan multichain.ChainAgnosticToken)
	contractsChan := make(chan multichain.ChainAgnosticContract)
	errChan := make(chan error)
	wp := pool.New().WithMaxGoroutines(10).WithContext(ctx)

	go func() {
		defer close(tokensChan)
		for a := range outCh {
			assetsReceived := a
			if assetsReceived.Err != nil {
				errChan <- assetsReceived.Err
				return
			}
			for _, n := range assetsReceived.Assets {
				nft := n
				wp.Go(func(ctx context.Context) error {
					return streamTokenAndContract(ctx, ownerAddress, nft, tokensChan, contractsChan, seenContracts)
				})
			}
		}
		err := wp.Wait()
		if err != nil {
			errChan <- err
		}
	}()

	for {
		select {
		case token, ok := <-tokensChan:
			if !ok {
				return resultTokens, resultContracts, nil
			}
			resultTokens = append(resultTokens, token)
		case contract, ok := <-contractsChan:
			if !ok {
				return resultTokens, resultContracts, nil
			}
			if contract.Address.String() != "" {
				resultContracts = append(resultContracts, contract)
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case err := <-errChan:
			return nil, nil, err
		}
	}
}

func streamAssetsToTokens(ctx context.Context, ownerAddress persist.Address, outCh <-chan assetsReceived, rec chan<- multichain.ChainAgnosticTokensAndContracts, errChan chan<- error) {
	seenContracts := &sync.Map{}
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
					return streamTokenAndContract(ctx, ownerAddress, nft, innerTokenReceived, innerContractReceived, seenContracts)
				})
			}
			err := wp.Wait()
			if err != nil {
				innerErrChan <- err
			}
		}()

	outer:
		for {
			select {
			case token, ok := <-innerTokenReceived:
				if !ok {
					break outer
				}
				innerTokens = append(innerTokens, token)
			case contract, ok := <-innerContractReceived:
				if !ok {
					break outer
				}
				if contract.Address.String() != "" {
					innerContracts = append(innerContracts, contract)
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

func streamTokenAndContract(ctx context.Context, ownerAddress persist.Address, asset Asset, innerTokenReceived chan<- multichain.ChainAgnosticToken, innerContractReceived chan<- multichain.ChainAgnosticContract, seenContracts *sync.Map) error {
	contract, ok := seenContracts.LoadOrStore(asset.Contract, contractFromAddress(asset.Contract))
	if !ok {
		innerContractReceived <- contract.(multichain.ChainAgnosticContract)
	} else {
		innerContractReceived <- multichain.ChainAgnosticContract{}
	}

	tokenOwner := ownerAddress
	if tokenOwner == "" {
		tokenOwner = persist.Address(asset.Owner.Address)
	}

	token, err := assetToToken(asset, tokenOwner)
	if err != nil {
		if util.ErrorIs[unknownTokenStandard](err) {
			logger.For(ctx).Error(err)
			return nil
		}
		return err
	}

	innerTokenReceived <- token
	return nil
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

type assetPage struct {
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

		page := assetPage{}
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
