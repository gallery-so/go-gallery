package reservoir

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	mc "github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
)

const (
	ethMainnetBaseURL         = "https://api.reservoir.tools"
	optimismBaseURL           = "https://api-optimism.reservoir.tools"
	polygonBaseURL            = "https://api-polygon.reservoir.tools"
	arbitrumBaseURL           = "https://api-arbitrum.reservoir.tools"
	zoraBaseURL               = "https://api-zora.reservoir.tools"
	baseBaseURL               = "https://api-base.reservoir.tools"
	getTokensEndpointTemplate = "%s/tokens/v7"
	getCollectionsEndpoint    = "%s/collections/v7"
	tokenBatchLimit           = 20 // The docs say 50 is the max, but the API returns 20 at most
	collectionBatchLimit      = 20
)

type Provider struct {
	chain           persist.Chain
	apiURL          string
	apiKey          string
	httpClient      *http.Client
	batchToken      *batchToken
	batchCollection *batchCollection
}

// NewProvider creates a new Reservoir provider
func NewProvider(ctx context.Context, httpClient *http.Client, chain persist.Chain) *Provider {
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

	c := *httpClient
	c.Transport = &authMiddleware{t: c.Transport, apiKey: env.GetString("RESERVOIR_API_KEY")}

	p := &Provider{
		apiURL:     apiURL,
		chain:      chain,
		httpClient: &c,
	}

	batchToken := &batchToken{
		provider: p,
		ctx:      ctx,
		maxBatch: tokenBatchLimit,
		wait:     200 * time.Millisecond,
	}

	batchCollection := &batchCollection{
		provider: p,
		ctx:      ctx,
		maxBatch: collectionBatchLimit,
		wait:     200 * time.Millisecond,
	}

	p.batchToken = batchToken
	p.batchCollection = batchCollection

	return p
}

type authMiddleware struct {
	t      http.RoundTripper
	apiKey string
}

func (a *authMiddleware) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("x-api-key", a.apiKey)
	t := a.t
	if t == nil {
		t = http.DefaultTransport
	}
	return t.RoundTrip(r)
}

func checkURL(s string) url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return *u
}

type mintStage struct {
	Stage   string `json:"stage"`
	Kind    string `json:"kind"`
	TokenID string `json:"tokenId"`
	Price   struct {
		Currency struct {
			Contract string `json:"contract"`
			Name     string `json:"name"`
			Symbol   string `json:"symbol"`
			Decimals int    `json:"decimals"`
		}
		Amount struct {
			Raw     string  `json:"raw"`
			Decimal float64 `json:"decimal"`
			USD     float64 `json:"usd"`
			Native  string  `json:"native"`
		}
	}
	StartTime         int64 `json:"startTime"`
	EndTime           int64 `json:"endTime"`
	MaxMintsPerWallet int   `json:"maxMintsPerWallet"`
}

type reservoirCollection struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	IsMinting  bool        `json:"isMinting"`  // Only present in collection response, not present in the token response
	MintStages []mintStage `json:"mintStages"` // Always present in collection response, only present in the token response for the queried tokens
}

type reservoirToken struct {
	TokenID    string              `json:"tokenId"`
	Kind       string              `json:"kind"`
	Collection reservoirCollection `json:"collection"`
	MintStages []mintStage         `json:"mintStages"`
}

type getTokensResponse struct {
	Tokens []struct {
		Token reservoirToken `json:"token"`
	}
	Continuation string `json:"continuation"`
}

type getCollectionResponse struct {
	Collections  []reservoirCollection `json:"collections"`
	Continuation string                `json:"continuation"`
}

func setToken(u url.URL, contractAddress persist.Address, tokenID persist.HexTokenID) url.URL {
	return setTokens(u, []mc.ChainAgnosticIdentifiers{{ContractAddress: contractAddress, TokenID: tokenID}})
}

func setTokens(u url.URL, tIDs []mc.ChainAgnosticIdentifiers) url.URL {
	query := u.Query()
	for _, t := range tIDs {
		query.Add("tokens", fmt.Sprintf("%s:%s", t.ContractAddress, t.TokenID.ToDecimalTokenID()))
	}
	u.RawQuery = query.Encode()
	return u
}

func setContracts(u url.URL, contractIDs []string) url.URL {
	query := u.Query()
	for _, c := range contractIDs {
		query.Add("contract", c)
	}
	u.RawQuery = query.Encode()
	return u
}

func setIncludeMintStages(u url.URL) url.URL {
	query := u.Query()
	query.Set("includeMintStages", "true")
	u.RawQuery = query.Encode()
	return u
}

func setCollectionID(u url.URL, collectionID string) url.URL {
	query := u.Query()
	query.Set("id", collectionID)
	u.RawQuery = query.Encode()
	return u
}

func readResponseBodyInto(ctx context.Context, httpClient *http.Client, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errMsg struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}

		err = util.UnmarshallBody(&errMsg, resp.Body)
		if err != nil {
			return err
		}

		msg := fmt.Errorf("%s: %s", errMsg.Error, errMsg.Message)

		return fmt.Errorf("unexpected statusCode(%d): %s", resp.StatusCode, msg)
	}

	return util.UnmarshallBody(into, resp.Body)
}

func (p *Provider) getToken(ctx context.Context, tIDs mc.ChainAgnosticIdentifiers) (reservoirToken, error) {
	u := checkURL(fmt.Sprintf(getTokensEndpointTemplate, p.apiURL))
	u = setToken(u, tIDs.ContractAddress, tIDs.TokenID)
	u = setIncludeMintStages(u)

	var body getTokensResponse

	err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
	if err != nil {
		return reservoirToken{}, err
	}

	if len(body.Tokens) == 0 {
		err := fmt.Errorf("not found: %s", persist.TokenIdentifiers{
			TokenID:         tIDs.TokenID,
			ContractAddress: tIDs.ContractAddress,
			Chain:           p.chain,
		})
		return reservoirToken{}, err
	}

	return body.Tokens[0].Token, nil
}

func (p *Provider) getTokenBatch(ctx context.Context, tIDs []mc.ChainAgnosticIdentifiers) ([]reservoirToken, []error) {
	u := checkURL(fmt.Sprintf(getTokensEndpointTemplate, p.apiURL))
	u = setTokens(u, tIDs)
	u = setIncludeMintStages(u)

	var body getTokensResponse

	batchResult := make([]reservoirToken, len(tIDs))
	batchErrs := make([]error, len(tIDs))

	err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
	if err != nil {
		// Fill with the same error
		for i := range tIDs {
			batchErrs[i] = err
		}
		return nil, batchErrs
	}

	tokenIDToToken := make(map[persist.TokenIdentifiers]reservoirToken)

	for i, t := range body.Tokens {
		tID := persist.TokenIdentifiers{
			TokenID:         persist.DecimalTokenID(t.Token.TokenID).ToHexTokenID(),
			ContractAddress: persist.Address(p.chain.NormalizeAddress(tIDs[i].ContractAddress)),
			Chain:           p.chain,
		}
		tokenIDToToken[tID] = t.Token
	}

	for i, tID := range tIDs {
		tID := persist.TokenIdentifiers{
			TokenID:         tID.TokenID,
			ContractAddress: persist.Address(p.chain.NormalizeAddress(tIDs[i].ContractAddress)),
			Chain:           p.chain,
		}
		token, ok := tokenIDToToken[tID]
		if !ok {
			batchErrs[i] = fmt.Errorf("%s not found", tID)
		} else {
			batchResult[i] = token
		}
	}

	return batchResult, batchErrs
}

// getCollectionBatch should only be called for single contract collections where the collection ID is the contract address
func (p *Provider) getCollectionBatch(ctx context.Context, collectionIDs []string) ([]reservoirCollection, []error) {
	u := checkURL(fmt.Sprintf(getCollectionsEndpoint, p.apiURL))
	u = setContracts(u, collectionIDs)
	u = setIncludeMintStages(u)

	var body getCollectionResponse

	batchResult := make([]reservoirCollection, len(collectionIDs))
	batchErrs := make([]error, len(collectionIDs))

	err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
	if err != nil {
		// Fill with the same error
		for i := range collectionIDs {
			batchErrs[i] = err
		}
		return nil, batchErrs
	}

	addressToCollection := make(map[persist.ContractIdentifiers]reservoirCollection)

	for _, c := range body.Collections {
		cID := persist.ContractIdentifiers{
			ContractAddress: persist.Address(p.chain.NormalizeAddress(persist.Address(c.ID))),
			Chain:           p.chain,
		}
		addressToCollection[cID] = c
	}

	for i, id := range collectionIDs {
		cID := persist.ContractIdentifiers{
			ContractAddress: persist.Address(p.chain.NormalizeAddress(persist.Address(id))),
			Chain:           p.chain,
		}
		collection, ok := addressToCollection[cID]
		if !ok {
			batchErrs[i] = fmt.Errorf("%s not found", cID)
		} else {
			batchResult[i] = collection
		}
	}

	return batchResult, batchErrs
}

func (p *Provider) getCollection(ctx context.Context, collectionID string) (reservoirCollection, error) {
	u := checkURL(fmt.Sprintf(getCollectionsEndpoint, p.apiURL))
	u = setCollectionID(u, collectionID)
	u = setIncludeMintStages(u)

	var body getCollectionResponse

	err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
	if err != nil {
		return reservoirCollection{}, err
	}

	if len(body.Collections) == 0 {
		err := fmt.Errorf("collection not found: %s", collectionID)
		return reservoirCollection{}, err
	}

	return body.Collections[0], nil
}

func translateCurrency(symbol string) (persist.Currency, error) {
	switch symbol {
	case "ETH":
		return persist.CurrencyEther, nil
	case "ENJOY":
		return persist.CurrencyEnjoy, nil
	default:
		return "", fmt.Errorf("unknown currency symbol: %s", symbol)
	}
}

func isMultiCollection(collectionID string) bool {
	// multi collections follow the format: <contract-address>:<namespace>
	return strings.Index(collectionID, ":") != -1
}

func (p *Provider) GetMintingStatusByTokenIdentifiers(ctx context.Context, tID mc.ChainAgnosticIdentifiers) (bool, persist.Currency, float64, error) {
	t, err := p.batchToken.Get(ctx, tID)
	if err != nil {
		return false, "", 0, err
	}

	if t.Kind == "erc1155" {
		if len(t.MintStages) == 0 {
			return false, "", 0, nil
		}

		// The response should only include the mint info for the specific token, but search for the
		// matching mint stage to be safe. TODO: There may be other kinds of minting other than time-based
		// that we should account for.
		for _, stage := range t.MintStages {
			if persist.DecimalTokenID(stage.TokenID) != tID.TokenID.ToDecimalTokenID() {
				continue
			}

			currency, err := translateCurrency(stage.Price.Currency.Symbol)
			if err != nil {
				return false, "", 0, err
			}

			n := time.Now().Unix()

			if stage.StartTime != 0 && stage.EndTime != 0 {
				return n >= stage.StartTime && n <= stage.EndTime, currency, stage.Price.Amount.Decimal, nil
			}

			if stage.StartTime != 0 {
				return n >= stage.StartTime, currency, stage.Price.Amount.Decimal, nil
			}

			err = fmt.Errorf("no start time found for %s", persist.TokenIdentifiers{
				TokenID:         tID.TokenID,
				ContractAddress: tID.ContractAddress,
				Chain:           p.chain,
			})

			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
			return false, "", 0, err
		}

		err = fmt.Errorf("no mint stage found for %s", persist.TokenIdentifiers{
			TokenID:         tID.TokenID,
			ContractAddress: tID.ContractAddress,
			Chain:           p.chain,
		})
		sentryutil.ReportError(ctx, err)
		return false, "", 0, err
	}

	var collection reservoirCollection

	// Reservoir doesn't offer a way to batch multi-collections, so fetch them separately
	if isMultiCollection(t.Collection.ID) {
		collection, err = p.getCollection(ctx, t.Collection.ID)
	} else {
		// The majority of collections are single contract, and Reservoir supports batch fetching of them
		collection, err = p.batchCollection.Get(ctx, t.Collection.ID)
	}

	if !collection.IsMinting {
		return false, "", 0, nil
	}

	if collection.IsMinting && len(collection.MintStages) == 0 {
		err = fmt.Errorf("no mint stage found for collection: %s, but collection is minting", t.Collection.ID)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		return false, "", 0, err
	}

	var currency persist.Currency

	// There are other denominations it could be in, just use the first one supportted for now
	for _, stage := range collection.MintStages {
		currency, err = translateCurrency(collection.MintStages[0].Price.Currency.Symbol)
		if err != nil {
			logger.For(ctx).Error(err)
			continue
		}
		return collection.IsMinting, currency, stage.Price.Amount.Decimal, nil
	}

	return false, "", 0, err
}
