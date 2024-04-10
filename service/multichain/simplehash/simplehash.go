package simplehash

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	// "sort"
	"strconv"
	"strings"
	// "time"

	"github.com/sourcegraph/conc/pool"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	mc "github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var (
	getNftsByWalletEndpoint      = checkURL(fmt.Sprintf(getNftsByWalletEndpointTemplate, baseURL))
	getNftsByTokenListEndpoint   = checkURL(fmt.Sprintf(getNftsByTokenListEndpointTemplate, baseURL))
	getContractsByWalletEndpoint = checkURL(fmt.Sprintf(getContractsByWalletEndpointTemplate, baseURL))
)

const (
	baseURL                              = "https://api.simplehash.com"
	getNftsByWalletEndpointTemplate      = "%s/api/v0/nfts/owners_v2"
	getNftsByTokenListEndpointTemplate   = "%s/api/v0/nfts/assets"
	getContractsByWalletEndpointTemplate = "%s/api/v0/nfts/contracts_by_wallets"
	getNftByTokenIDEndpointTemplate      = "%s/api/v0/nfts/%s/%s/%s"
	getNftsByContractEndpointTemplate    = "%s/api/v0/nfts/%s/%s"
	spamScoreThreshold                   = 90
	tokenBatchLimit                      = 50
	contractBatchLimit                   = 40
	incrementalSyncPoolSize              = 24
)

type Provider struct {
	chain      persist.Chain
	httpClient *http.Client
}

func NewProvider(chain persist.Chain, httpClient *http.Client) *Provider {
	if _, ok := chainToSimpleHashChain[chain]; !ok {
		panic(fmt.Sprintf("simplehash is not configured for chain=%d", chain))
	}
	c := *httpClient
	c.Transport = &authMiddleware{t: c.Transport, apiKey: env.GetString("SIMPLEHASH_API_KEY")}
	return &Provider{httpClient: &c, chain: chain}
}

type authMiddleware struct {
	t      http.RoundTripper
	apiKey string
}

func (a *authMiddleware) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Add("X-API-KEY", a.apiKey)
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

var chainToSimpleHashChain = map[persist.Chain]string{
	persist.ChainETH:  "ethereum",
	persist.ChainBase: "base",
	persist.ChainZora: "zora",
}

func setChain(u url.URL, chain persist.Chain) url.URL {
	query := u.Query()
	query.Set("chains", chainToSimpleHashChain[chain])
	u.RawQuery = query.Encode()
	return u
}

func setWallet(u url.URL, wallet persist.Address) url.URL {
	query := u.Query()
	query.Set("wallet_addresses", wallet.String())
	u.RawQuery = query.Encode()
	return u
}

func setContractAddress(u url.URL, chain persist.Chain, contract persist.Address) url.URL {
	query := u.Query()
	query.Set("contract_ids", fmtContractID(chain, contract))
	u.RawQuery = query.Encode()
	return u
}

func setContractID(u url.URL, contractID string) url.URL {
	return setContractIDs(u, []string{contractID})
}

func setContractIDs(u url.URL, contractIDs []string) url.URL {
	query := u.Query()
	query.Set("contract_ids", strings.Join(contractIDs, ","))
	u.RawQuery = query.Encode()
	return u
}

func setQueriedWalletBalances(u url.URL) url.URL {
	query := u.Query()
	query.Set("queried_wallet_balances", "1")
	u.RawQuery = query.Encode()
	return u
}

func setLimit(u url.URL, limit int) url.URL {
	query := u.Query()
	query.Set("limit", strconv.Itoa(limit))
	u.RawQuery = query.Encode()
	return u
}

func setSpamThreshold(u url.URL, threshold int) url.URL {
	query := u.Query()
	query.Set("spam_score__lt", strconv.Itoa(threshold))
	u.RawQuery = query.Encode()
	return u
}

func setSpamFilter(u url.URL, threshold int) url.URL {
	query := u.Query()
	query.Set("filters", "spam_score__lt="+strconv.Itoa(threshold))
	u.RawQuery = query.Encode()
	return u
}

func setNftIDs(u url.URL, chain persist.Chain, ids []mc.ChainAgnosticIdentifiers) url.URL {
	query := u.Query()
	nftIDs := make([]string, len(ids))
	for i, id := range ids {
		nftIDs[i] = fmtNftID(chain, id.ContractAddress, id.TokenID.ToDecimalTokenID())
	}
	query.Set("nft_ids", strings.Join(nftIDs, ","))
	u.RawQuery = query.Encode()
	return u
}

func fmtContractID(chain persist.Chain, contract persist.Address) string {
	return fmt.Sprintf("%s.%s", chainToSimpleHashChain[chain], contract)
}

func fmtNftID(chain persist.Chain, contract persist.Address, tokenID persist.DecimalTokenID) string {
	return fmt.Sprintf("%s.%s.%s", chainToSimpleHashChain[chain], contract, tokenID)
}

type simplehashPreviews struct {
	ImageSmallURL     string `json:"image_small_url"`
	ImageMediumURL    string `json:"image_medium_url"`
	ImageLargeURL     string `json:"image_large_url"`
	ImageOpenGraphURL string `json:"image_opengraph_url"`
	Blurhash          string `json:"blurhash"`
	PredominantColor  string `json:"predominant_color"`
}

type simplehashImageProps struct {
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Size     int    `json:"size"`
	MimeType string `json:"mime_type"`
}

type simplehashOwners struct {
	OwnerAddress      string `json:"owner_address"`
	Quantity          int    `json:"quantity"`
	QuantityString    string `json:"quantity_string"`
	FirstAcquiredDate string `json:"first_acquired_date"`
	LastAcquiredDate  string `json:"last_acquired_date"`
}

type simplehashContract struct {
	Type                   string `json:"type"`
	Name                   string `json:"name"`
	Symbol                 string `json:"symbol"`
	DeployedBy             string `json:"deployed_by"`
	OwnedBy                string `json:"owned_by"`
	DeployedViaContract    string `json:"deployed_via_contract"`
	HasMultipleCollections bool   `json:"has_multiple_collections"`
}

type simplehashMetadata struct {
	ImageOriginalURL     string `json:"image_original_url"`
	AnimationOriginalURL string `json:"animation_original_url"`
	MetadataOriginalURL  string `json:"metadata_original_url"`
}

type simplehashCollection struct {
	CollectionID string   `json:"collection_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	ImageURL     string   `json:"image_url"`
	ExternalURL  string   `json:"external_url"`
	IsNSFW       bool     `json:"is_nsfw"`
	SpamScore    int      `json:"spam_score"`
	Chains       []string `json:"chains"`
	TopContracts []string `json:"top_contracts"`
}

type queriedWalletBalance struct {
	Address           string `json:"address"`
	Quantity          int    `json:"quantity"`
	QuantityString    string `json:"quantity_string"`
	FirstAcquiredDate string `json:"first_acquired_date"`
	LastAcquiredDate  string `json:"last_acquired_date"`
}

type simplehashNFT struct {
	NftID                 string                 `json:"nft_id"`
	Chain                 string                 `json:"chain"`
	ContractAddress       string                 `json:"contract_address"`
	TokenID               string                 `json:"token_id"`
	Name                  string                 `json:"name"`
	Description           string                 `json:"description"`
	Previews              simplehashPreviews     `json:"previews"`
	ImageURL              string                 `json:"image_url"`
	ImageProperties       simplehashImageProps   `json:"image_properties"`
	ExternalURL           string                 `json:"external_url"`
	Owners                []simplehashOwners     `json:"owners"`
	Contract              simplehashContract     `json:"contract"`
	Collection            simplehashCollection   `json:"collection"`
	ExtraMetadata         simplehashMetadata     `json:"extra_metadata"`
	QueriedWalletBalances []queriedWalletBalance `json:"queried_wallet_balances"`
}

type getNftsByWalletResponse struct {
	NextCursor string          `json:"next_cursor"`
	Next       string          `json:"next"`
	NFTs       []simplehashNFT `json:"nfts"`
}

type getNftsByContractResponse struct {
	NextCursor string          `json:"next_cursor"`
	Next       string          `json:"next"`
	NFTs       []simplehashNFT `json:"nfts"`
}

type getNftsByTokenListResponse struct {
	NFTs []simplehashNFT `json:"nfts"`
}

type simplehashContractOwnership struct {
	PrimaryKey        string `json:"primary_key"`
	DistinctNftsOwned int    `json:"distinct_nfts_owned"`
}

type getContractsByWalletResponse struct {
	NextCursor string                        `json:"next_cursor"`
	Next       string                        `json:"next"`
	Contracts  []simplehashContractOwnership `json:"contracts"`
}

type getCollectionByContractResponse struct {
	NextCursor  string                 `json:"next_cursor"`
	Next        string                 `json:"next"`
	Collections []simplehashCollection `json:"collections"`
}

func isSpamCollection(c simplehashCollection) bool {
	return c.SpamScore > spamScoreThreshold
}

func translateToChainAgnosticToken(t simplehashNFT, ownerAddress persist.Address, isSpam *bool) mc.ChainAgnosticToken {
	var tokenType persist.TokenType

	if t.Contract.Type == "ERC721" {
		tokenType = persist.TokenTypeERC721
	} else if t.Contract.Type == "ERC1155" {
		tokenType = persist.TokenTypeERC1155
	} else {
		tID := mc.ChainAgnosticIdentifiers{ContractAddress: persist.Address(t.ContractAddress), TokenID: persist.HexTokenID(t.TokenID)}
		logger.For(context.Background()).Warnf("%s has unknown token type: %s", tID, t.Contract.Type)
	}

	var quantity persist.HexString
	if len(t.QueriedWalletBalances) > 0 {
		quantity = persist.MustHexString(t.QueriedWalletBalances[0].QuantityString)
	}

	return mc.ChainAgnosticToken{
		Descriptors: mc.ChainAgnosticTokenDescriptors{
			Name:        t.Name,
			Description: t.Description,
		},
		TokenType:    tokenType,
		TokenURI:     persist.TokenURI(t.ExtraMetadata.MetadataOriginalURL),
		TokenID:      persist.MustTokenID(t.TokenID),
		OwnerAddress: ownerAddress,
		TokenMetadata: persist.TokenMetadata{
			"name":          t.Name,
			"description":   t.Description,
			"image_url":     t.ExtraMetadata.ImageOriginalURL,
			"animation_url": t.ExtraMetadata.AnimationOriginalURL,
			"original_url":  t.ExtraMetadata.MetadataOriginalURL,
		},
		ContractAddress: persist.Address(t.ContractAddress),
		ExternalURL:     t.ExternalURL,
		Quantity:        quantity,
		IsSpam:          isSpam,
		FallbackMedia: persist.FallbackMedia{
			ImageURL: persist.NullString(t.ImageURL),
			Dimensions: persist.Dimensions{
				Width:  t.ImageProperties.Width,
				Height: t.ImageProperties.Height,
			},
		},
	}
}

func translateToChainAgnosticContract(address string, contract simplehashContract, collection simplehashCollection) mc.ChainAgnosticContract {
	c := mc.ChainAgnosticContract{
		Descriptors: mc.ChainAgnosticContractDescriptors{
			Symbol:         contract.Symbol,
			Name:           contract.Name,
			OwnerAddress:   persist.Address(contract.OwnedBy),
			CreatorAddress: persist.Address(contract.DeployedBy),
		},
		Address: persist.Address(address),
		IsSpam:  util.ToPointer(isSpamCollection(collection)),
	}
	if !contract.HasMultipleCollections {
		c.Descriptors.Description = collection.Description
		c.Descriptors.ProfileImageURL = collection.ImageURL
	}
	return c
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
			Message string `json:"message"`
		}

		err = util.UnmarshallBody(&errMsg, resp.Body)
		if err != nil {
			return err
		}

		return fmt.Errorf("unexpected statusCode(%d): %s", resp.StatusCode, errMsg.Message)
	}

	return util.UnmarshallBody(into, resp.Body)
}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tIDs mc.ChainAgnosticIdentifiers, owner persist.Address) (mc.ChainAgnosticToken, mc.ChainAgnosticContract, error) {
	u := setChain(getNftsByWalletEndpoint, p.chain)
	u = setWallet(u, owner)
	u = setContractAddress(u, p.chain, tIDs.ContractAddress)
	u = setQueriedWalletBalances(u)
	next := u.String()

	var token simplehashNFT

outer:
	for next != "" && token.TokenID == "" {
		var body getNftsByWalletResponse

		err := readResponseBodyInto(ctx, p.httpClient, next, &body)
		if err != nil {
			return mc.ChainAgnosticToken{}, mc.ChainAgnosticContract{}, err
		}

		for _, nft := range body.NFTs {
			if nft.TokenID == tIDs.TokenID.ToDecimalTokenID().String() {
				token = nft
				break outer
			}
		}

		next = body.Next
	}

	if token.TokenID == "" {
		err := fmt.Errorf("token not found: %s", persist.TokenUniqueIdentifiers{
			Chain:           p.chain,
			ContractAddress: tIDs.ContractAddress,
			TokenID:         tIDs.TokenID,
			OwnerAddress:    owner,
		})
		logger.For(ctx).Error(err)
		return mc.ChainAgnosticToken{}, mc.ChainAgnosticContract{}, err
	}

	aContract := translateToChainAgnosticContract(token.ContractAddress, token.Contract, token.Collection)
	aToken := translateToChainAgnosticToken(token, owner, aContract.IsSpam)
	return aToken, aContract, nil
}

func (p *Provider) binRequestsByContract(ctx context.Context, address persist.Address) (<-chan []string, <-chan error) {
	outCh := make(chan []string, incrementalSyncPoolSize)
	errCh := make(chan error)

	go func() {
		defer close(outCh)
		defer close(errCh)

		requestBins := [][]string{}
		requestBinTotals := []int{0}

		u := setChain(getContractsByWalletEndpoint, p.chain)
		u = setWallet(u, address)
		u = setLimit(u, tokenBatchLimit)
		u = setSpamThreshold(u, spamScoreThreshold)
		next := u.String()

		for next != "" {
			var body getContractsByWalletResponse

			err := readResponseBodyInto(ctx, p.httpClient, next, &body)
			if err != nil {
				errCh <- err
				return
			}

			// Use first-fit binning to find the first request that can fit the contract
			for _, contract := range body.Contracts {
				var placed bool
				var addedToBinIdx int

				for binIdx, bin := range requestBins {
					if requestBinTotals[binIdx]+contract.DistinctNftsOwned <= tokenBatchLimit && len(bin)+1 <= contractBatchLimit {
						requestBins[binIdx] = append(requestBins[binIdx], contract.PrimaryKey)
						requestBinTotals[binIdx] += contract.DistinctNftsOwned
						placed = true
						addedToBinIdx = binIdx
						break
					}
				}

				if !placed {
					newBin := []string{contract.PrimaryKey}
					addedToBinIdx = len(requestBins)
					requestBins = append(requestBins, newBin)
					requestBinTotals = append(requestBinTotals, contract.DistinctNftsOwned)
				}

				// Send a bin once its full
				if requestBinTotals[addedToBinIdx] >= tokenBatchLimit || len(requestBins[addedToBinIdx]) >= contractBatchLimit {
					outCh <- requestBins[addedToBinIdx]
					requestBins = append(requestBins[:addedToBinIdx], requestBins[addedToBinIdx+1:]...)
					requestBinTotals = append(requestBinTotals[:addedToBinIdx], requestBinTotals[addedToBinIdx+1:]...)
				}
			}

			next = body.Next
		}

		// Send the remaining bins
		for _, bin := range requestBins {
			outCh <- bin
		}

	}()

	return outCh, errCh
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, address persist.Address) (<-chan mc.ChainAgnosticTokensAndContracts, <-chan error) {
	batchRequestCh, batchRequestErrCh := p.binRequestsByContract(ctx, address)

	outCh := make(chan mc.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)

	go func() {
		defer close(outCh)
		defer close(errCh)

		workers := pool.New().WithMaxGoroutines(incrementalSyncPoolSize)

	outer:
		for {
			select {
			case err, ok := <-batchRequestErrCh:
				if ok {
					errCh <- err
					return
				}
			case contractBin, ok := <-batchRequestCh:
				if !ok {
					break outer
				}

				workers.Go(func() {
					logger.For(ctx).Infof("simplehash batch fetching tokens from %d contracts for address=%s\n", len(contractBin), address)
					u := setChain(getNftsByWalletEndpoint, p.chain)
					u = setWallet(u, address)
					u = setQueriedWalletBalances(u)
					u = setLimit(u, tokenBatchLimit)
					u = setSpamFilter(u, spamScoreThreshold)
					u = setContractIDs(u, contractBin)

					next := u.String()

					for next != "" {
						var body getNftsByWalletResponse

						err := readResponseBodyInto(ctx, p.httpClient, next, &body)
						if err != nil {
							errCh <- err
							return
						}

						var page mc.ChainAgnosticTokensAndContracts

						for _, nft := range body.NFTs {
							contract := translateToChainAgnosticContract(nft.ContractAddress, nft.Contract, nft.Collection)
							token := translateToChainAgnosticToken(nft, persist.Address(address), contract.IsSpam)
							page.Contracts = append(page.Contracts, contract)
							page.Tokens = append(page.Tokens, token)
						}

						outCh <- page

						next = body.Next
					}
				})
			}
		}

		workers.Wait()
	}()

	return outCh, errCh
}

func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, address persist.Address, maxLimit int) (<-chan mc.ChainAgnosticTokensAndContracts, <-chan error) {
	outCh := make(chan mc.ChainAgnosticTokensAndContracts)
	errCh := make(chan error)

	go func() {
		defer close(outCh)
		defer close(errCh)

		u := checkURL(fmt.Sprintf(getNftsByContractEndpointTemplate, baseURL, chainToSimpleHashChain[p.chain], address))
		u = setLimit(u, tokenBatchLimit)

		next := u.String()
		var tokensReceived int

		for next != "" && tokensReceived < maxLimit {
			var body getNftsByContractResponse

			err := readResponseBodyInto(ctx, p.httpClient, next, &body)
			if err != nil {
				errCh <- err
				return
			}

			var page mc.ChainAgnosticTokensAndContracts

			for i := 0; i < len(body.NFTs) && tokensReceived < maxLimit; i++ {
				nft := body.NFTs[i]
				contract := translateToChainAgnosticContract(nft.ContractAddress, nft.Contract, nft.Collection)
				token := translateToChainAgnosticToken(nft, persist.Address(address), contract.IsSpam)
				page.Tokens = append(page.Tokens, token)
				page.Contracts = append(page.Contracts, contract)
				tokensReceived++
			}

			outCh <- page

			next = body.Next
		}
	}()

	return outCh, errCh
}

func (p *Provider) GetTokenMetadataByTokenIdentifiersBatch(ctx context.Context, tIDs []mc.ChainAgnosticIdentifiers) ([]persist.TokenMetadata, error) {
	if len(tIDs) == 0 {
		return []persist.TokenMetadata{}, nil
	}

	chunks := util.ChunkBy(tIDs, tokenBatchLimit)
	metadata := make([]persist.TokenMetadata, len(tIDs))
	lookup := make(map[mc.ChainAgnosticIdentifiers]persist.TokenMetadata)

	for i, c := range chunks {
		batchID := i + 1
		u := setNftIDs(getNftsByTokenListEndpoint, p.chain, c)

		logger.For(ctx).Infof("handling metadata batch=%d of %d", batchID, len(chunks))

		var body getNftsByTokenListResponse

		err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
		if err != nil {
			logger.For(ctx).Errorf("failed to handle metadata batch=%d: %s", batchID, err)
			continue
		}

		for _, t := range body.NFTs {
			token := translateToChainAgnosticToken(t, "", nil)
			tID := mc.ChainAgnosticIdentifiers{ContractAddress: persist.Address(p.chain.NormalizeAddress(token.ContractAddress)), TokenID: token.TokenID}
			lookup[tID] = token.TokenMetadata
		}
	}

	for i, tID := range tIDs {
		metadata[i] = lookup[tID]
	}

	return metadata, nil
}

func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tID mc.ChainAgnosticIdentifiers) ([]mc.ChainAgnosticToken, mc.ChainAgnosticContract, error) {
	u := checkURL(fmt.Sprintf(getNftByTokenIDEndpointTemplate, baseURL, chainToSimpleHashChain[p.chain], tID.ContractAddress, tID.TokenID.ToDecimalTokenID()))

	var body simplehashNFT

	err := readResponseBodyInto(ctx, p.httpClient, u.String(), &body)
	if err != nil {
		return nil, mc.ChainAgnosticContract{}, err
	}

	if body.NftID == "" {
		err := fmt.Errorf("token not found: %s", persist.TokenIdentifiers{
			Chain:           p.chain,
			ContractAddress: tID.ContractAddress,
			TokenID:         tID.TokenID,
		})
		logger.For(ctx).Error(err)
		return nil, mc.ChainAgnosticContract{}, err
	}

	contract := translateToChainAgnosticContract(body.ContractAddress, body.Contract, body.Collection)
	token := translateToChainAgnosticToken(body, "", contract.IsSpam)
	return []mc.ChainAgnosticToken{token}, contract, nil
}

func (p *Provider) GetContractByAddress(ctx context.Context, address persist.Address) (mc.ChainAgnosticContract, error) {
	// Needs at least one mint in order to fetch the contract, because the contract object is only available in the token response
	outCh, errCh := p.GetTokensIncrementallyByContractAddress(ctx, address, 1)
	for {
		select {
		case page := <-outCh:
			if len(page.Contracts) == 0 {
				return mc.ChainAgnosticContract{}, fmt.Errorf("%s not found", persist.NewContractIdentifiers(address, p.chain))
			}
			return page.Contracts[0], nil
		case err := <-errCh:
			if err != nil {
				return mc.ChainAgnosticContract{}, err
			}
		}
	}
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, tID mc.ChainAgnosticIdentifiers) (mc.ChainAgnosticTokenDescriptors, mc.ChainAgnosticContractDescriptors, error) {
	tokens, contract, err := p.GetTokensByTokenIdentifiers(ctx, tID)
	if err != nil {
		return mc.ChainAgnosticTokenDescriptors{}, mc.ChainAgnosticContractDescriptors{}, err
	}
	return tokens[0].Descriptors, contract.Descriptors, nil
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, tID mc.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	tokens, _, err := p.GetTokensByTokenIdentifiers(ctx, tID)
	if err != nil {
		return persist.TokenMetadata{}, err
	}
	return tokens[0].TokenMetadata, nil
}
