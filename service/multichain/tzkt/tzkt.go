package tzkt

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"

	"github.com/gammazero/workerpool"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/platform"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/multichain/objkt"
	"github.com/mikeydub/go-gallery/service/multichain/tezos"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

const pageSize = 1000

type tzMetadata struct {
	Date               string `json:"date"`
	Name               string `json:"name"`
	Tags               any    `json:"tags"`
	Image              string `json:"image"`
	Minter             string `json:"minter"`
	Rights             string `json:"rights"`
	Symbol             string `json:"symbol"`
	Formats            any
	Creators           any    `json:"creators"`
	Decimals           string `json:"decimals"`
	Attributes         any
	DisplayURI         string `json:"displayUri"`
	ArtifactURI        string `json:"artifactUri"`
	Description        string `json:"description"`
	MintingTool        string `json:"mintingTool"`
	ThumbnailURI       string `json:"thumbnailUri"`
	IsBooleanAmount    any    `json:"isBooleanAmount"`
	ShouldPreferSymbol any    `json:"shouldPreferSymbol"`
}

type tzktBalanceToken struct {
	ID      uint64 `json:"id"`
	Account struct {
		Alias   string          `json:"alias"`
		Address persist.Address `json:"address"`
	} `json:"account"`
	Token struct {
		ID       uint64 `json:"id"`
		Contract struct {
			Alias   string          `json:"alias"`
			Address persist.Address `json:"address"`
		} `json:"contract"`
		TokenID  string              `json:"tokenId"`
		Standard tezos.TokenStandard `json:"standard"`
		Metadata tzMetadata          `json:"metadata"`
	} `json:"token"`
	Balance    string `json:"balance"`
	FirstLevel uint64 `json:"firstLevel"`
	LastLevel  uint64 `json:"lastLevel"`
}

type tzktContract struct {
	ID           uint64 `json:"id"`
	Alias        string `json:"alias"`
	Address      string `json:"address"`
	LastActivity uint64 `json:"lastActivity"`
	Creator      struct {
		Alias   string          `json:"alias"`
		Address persist.Address `json:"address"`
	} `json:"creator"`
}

// Provider is an the struct for retrieving data from the Tezos blockchain
type Provider struct {
	apiURL     string
	httpClient *http.Client
	// Metadata tends to be better on objkt for certain tokens like fxhash
	// so we replace them with objkt ones where appropriate
	objktProvider *objkt.Provider
}

// NewProvider creates a new Tezos Provider
func NewProvider(httpClient *http.Client) *Provider {
	return &Provider{
		apiURL:        env.GetString("TEZOS_API_URL"),
		httpClient:    httpClient,
		objktProvider: objkt.NewProvider(),
	}
}

func (p *Provider) ProviderInfo() multichain.ProviderInfo {
	return multichain.ProviderInfo{
		Chain:      persist.ChainTezos,
		ChainID:    0,
		ProviderID: "tzkt",
	}
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Tezos Blockchain
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tzAddr, err := tezos.ToAddress(addr)
	if err != nil {
		return nil, nil, err
	}
	limit := pageSize
	offset := 0
	resultTokens := []tzktBalanceToken{}
	for {

		tzktBalances, err := p.fetchBalancesByAddress(ctx, tzAddr, limit, offset)
		if err != nil {
			return nil, nil, err
		}

		resultTokens = append(resultTokens, tzktBalances...)

		if len(tzktBalances) < limit {
			break
		}

		offset += limit

		logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), pageSize, offset)
	}

	return p.tzBalanceTokensToTokens(ctx, resultTokens, addr.String())
}

func (p *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	go func() {
		defer close(rec)

		tzAddr, err := tezos.ToAddress(addr)
		if err != nil {
			errChan <- err
			return
		}
		limit := 100
		offset := 0
		for {
			tzktBalances, err := p.fetchBalancesByAddress(ctx, tzAddr, limit, offset)
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(tzktBalances), tzAddr.String(), limit, offset)

			resultTokens, resultContracts, err := p.tzBalanceTokensToTokens(ctx, tzktBalances, addr.String())
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("converted %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), limit, offset)

			if len(resultTokens) > 0 || len(resultContracts) > 0 {
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    resultTokens,
					Contracts: resultContracts,
				}
			}

			if len(tzktBalances) < limit {
				break
			}

			offset += limit

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), limit, offset)
		}
	}()
	return rec, errChan
}

func (p *Provider) fetchBalancesByAddress(ctx context.Context, tzAddr persist.Address, limit, offset int) ([]tzktBalanceToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&limit=%d&sort.asc=id&offset=%d", p.apiURL, tzAddr.String(), limit, offset), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return nil, err
	}
	return tzktBalances, nil
}

// GetTokensByContractAddress retrieves tokens for a contract address on the Tezos Blockchain
func (p *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	offset := startOffset
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {

		tzktBalances, err := p.fetchBalancesByContract(ctx, contractAddress, limit, offset)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resultTokens = append(resultTokens, tzktBalances...)

		if len(tzktBalances) < limit || (maxLimit > 0 && len(resultTokens) >= maxLimit) {
			break
		}

		if maxLimit > 0 && len(resultTokens)+limit >= maxLimit {
			// this will ensure that we don't go over the max limit
			limit = maxLimit - len(resultTokens)
		}

		offset += limit
	}

	tokens, contracts, err := p.tzBalanceTokensToTokens(ctx, resultTokens, contractAddress.String())
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contractAddress) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tez contract found for address: %s", contractAddress)
	}
	contract := contracts[0]

	return tokens, contract, nil
}

func (p *Provider) fetchBalancesByContract(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]tzktBalanceToken, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.contract=%s&limit=%d&offset=%d", p.apiURL, contractAddress.String(), limit, offset), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return nil, err
	}

	return tzktBalances, nil
}

func (p *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, addr persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	go func() {
		defer close(rec)

		tzAddr, err := tezos.ToAddress(addr)
		if err != nil {
			errChan <- err
			return
		}
		limit := int(math.Min(float64(maxLimit), float64(pageSize)))
		if limit < 1 {
			limit = pageSize
		}
		offset := 0
		for {
			tzktBalances, err := p.fetchBalancesByContract(ctx, tzAddr, limit, offset)
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(tzktBalances), tzAddr.String(), limit, offset)

			resultTokens, resultContracts, err := p.tzBalanceTokensToTokens(ctx, tzktBalances, addr.String())
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("converted %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), limit, offset)

			if len(resultTokens) > 0 || len(resultContracts) > 0 {
				rec <- multichain.ChainAgnosticTokensAndContracts{
					Tokens:    resultTokens,
					Contracts: resultContracts,
				}
			}

			if len(tzktBalances) < limit || (maxLimit > 0 && len(resultTokens) >= maxLimit) {
				break
			}

			if maxLimit > 0 && len(resultTokens)+limit >= maxLimit {
				// this will ensure that we don't go over the max limit
				limit = maxLimit - len(resultTokens)
			}

			offset += limit

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), limit, offset)
		}
	}()
	return rec, errChan
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Tezos Blockchain
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := startOffset
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&limit=%d&offset=%d", p.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := retry.RetryRequest(p.httpClient, req)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
		}
		var tzktBalances []tzktBalanceToken
		if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resultTokens = append(resultTokens, tzktBalances...)

		if len(tzktBalances) < limit || (maxLimit > 0 && len(resultTokens) >= maxLimit) {
			break
		}

		if maxLimit > 0 && len(resultTokens)+limit >= maxLimit {
			// this will ensure that we don't go over the max limit
			limit = maxLimit - len(resultTokens)
		}

		offset += limit

	}
	logger.For(ctx).Info("tzktBalances: ", len(resultTokens))

	tokens, contracts, err := p.tzBalanceTokensToTokens(ctx, resultTokens, tokenIdentifiers.String())
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}

	return tokens, contract, nil

}

func (p *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&account=%s&limit=1", p.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, ownerAddress), nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}

	tokens, contracts, err := p.tzBalanceTokensToTokens(ctx, tzktBalances, tokenIdentifiers.String())
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	if len(tokens) > 0 {
		return tokens[0], contract, nil
	} else {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no token found for token identifiers: %s", tokenIdentifiers.String())
	}
}

func (p *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&limit=1", p.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress), nil)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, util.GetErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}

	tokens, contracts, err := p.tzBalanceTokensToTokens(ctx, tzktBalances, tokenIdentifiers.String())
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	token := multichain.ChainAgnosticToken{}
	if len(tokens) > 0 {
		token = tokens[0]
	} else {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, fmt.Errorf("no token found for token identifiers: %s", tokenIdentifiers.String())
	}

	return token.Descriptors, contract.Descriptors, nil
}

// GetContractByAddress retrieves an Tezos contract by address
func (p *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/contracts/%s?type=contract", p.apiURL, addr.String()), nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var tzktContract tzktContract
	if err := json.NewDecoder(resp.Body).Decode(&tzktContract); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return p.tzContractToContract(ctx, tzktContract), nil

}

type tzktOrigination struct {
	Type      string `json:"type"`
	ID        uint64 `json:"id"`
	Level     uint64 `json:"level"`
	Timestamp string `json:"timestamp"`
	Block     string `json:"block"`
	Hash      string `json:"hash"`
	Sender    struct {
		Alias   string          `json:"alias"`
		Address persist.Address `json:"address"`
	} `json:"sender"`
	OriginatedContract struct {
		Kind     string `json:"kind"`
		Address  string `json:"address"`
		Alias    string `json:"alias"`
		TypeHash int    `json:"typeHash"`
		CodeHash int    `json:"codeHash"`
		Tzips    []string
	} `json:"originatedContract"`
}

// GetContractsByOwnerAddress retrieves ethereum contracts by their owner address
func (p *Provider) GetContractsByOwnerAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/operations/originations?sender=%s", p.apiURL, addr.String()), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(p.httpClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, util.GetErrFromResp(resp)
	}
	var originations []tzktOrigination
	if err := json.NewDecoder(resp.Body).Decode(&originations); err != nil {
		return nil, err
	}

	filtered := util.Filter(originations, func(o tzktOrigination) bool {
		return o.OriginatedContract.Kind == "asset" && (util.ContainsString(o.OriginatedContract.Tzips, "fa2") || util.ContainsString(o.OriginatedContract.Tzips, "fa1.2"))
	}, false)

	contracts := make([]multichain.ChainAgnosticContract, 0, len(filtered))
	for _, o := range filtered {
		contracts = append(contracts, multichain.ChainAgnosticContract{
			Address: persist.Address(o.OriginatedContract.Address),
			Descriptors: multichain.ChainAgnosticContractDescriptors{
				Name:         o.OriginatedContract.Alias,
				OwnerAddress: addr,
			},
		})
	}

	return contracts, nil
}

func (p *Provider) GetOwnedTokensByContract(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := 0
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&token.contract=%s&limit=%d&offset=%d", p.apiURL, ownerAddress, contractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := retry.RetryRequest(p.httpClient, req)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
		}
		var tzktBalances []tzktBalanceToken
		if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resultTokens = append(resultTokens, tzktBalances...)

		if len(tzktBalances) < limit || (maxLimit > 0 && len(resultTokens) >= maxLimit) {
			break
		}

		if maxLimit > 0 && len(resultTokens)+limit >= maxLimit {
			// this will ensure that we don't go over the max limit
			limit = maxLimit - len(resultTokens)
		}
		offset += limit
	}

	logger.For(ctx).Info("tzktBalances: ", len(resultTokens))

	tokens, contracts, err := p.tzBalanceTokensToTokens(ctx, resultTokens, fmt.Sprintf("%s:%s", contractAddress, ownerAddress))
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	var contract multichain.ChainAgnosticContract
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	t, _, err := p.GetTokensByTokenIdentifiers(ctx, ti, 1, 0)
	if err != nil {
		return persist.TokenMetadata{}, err
	}
	if len(t) == 0 {
		return persist.TokenMetadata{}, fmt.Errorf("no token found for %s", ti)
	}
	return t[0].TokenMetadata, nil
}

func (p *Provider) tzBalanceTokensToTokens(pCtx context.Context, tzTokens []tzktBalanceToken, mediaKey string) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tzTokens = dedupeBalances(tzTokens)
	seenContracts := map[string]bool{}
	contractsLock := &sync.Mutex{}
	tokenChan := make(chan multichain.ChainAgnosticToken)
	contractChan := make(chan multichain.ChainAgnosticContract)

	errChan := make(chan error)
	ctx, cancel := context.WithCancel(pCtx)
	wp := workerpool.New(10)
	for _, t := range tzTokens {
		tzToken := t
		wp.Submit(func() {
			if tzToken.Token.Standard == tezos.TokenStandardFa12 {
				errChan <- nil
				return
			}

			normalizedContractAddress := persist.ChainTezos.NormalizeAddress(tzToken.Token.Contract.Address)
			metadata, err := json.Marshal(tzToken.Token.Metadata)
			if err != nil {
				errChan <- err
				return
			}

			var agnosticMetadata persist.TokenMetadata
			if err := json.Unmarshal(metadata, &agnosticMetadata); err != nil {
				errChan <- err
				return
			}

			token := multichain.ChainAgnosticToken{
				TokenType: persist.TokenTypeERC1155,
				Descriptors: multichain.ChainAgnosticTokenDescriptors{
					Description: tzToken.Token.Metadata.Description,
					Name:        tzToken.Token.Metadata.Name,
				},
				TokenID: persist.MustTokenID(tzToken.Token.TokenID),
				FallbackMedia: persist.FallbackMedia{
					ImageURL: persist.NullString(tzToken.Token.Metadata.Image),
				},
				ContractAddress: tzToken.Token.Contract.Address,
				Quantity:        persist.MustHexString(tzToken.Balance),
				TokenMetadata:   agnosticMetadata,
				OwnerAddress:    tzToken.Account.Address,
				BlockNumber:     persist.BlockNumber(tzToken.LastLevel),
			}

			// Try objkt if token isn't signed yet
			if !platform.IsFxhashSignedTezos(persist.ChainTezos, token.ContractAddress, token.Descriptors.Name) {
				tIDs := multichain.ChainAgnosticIdentifiers{ContractAddress: tzToken.Token.Contract.Address, TokenID: persist.MustTokenID(tzToken.Token.TokenID)}
				objktToken, objktContract, err := p.objktProvider.GetTokenByTokenIdentifiersAndOwner(ctx, tIDs, tzToken.Account.Address)
				if err == nil {
					token = objktToken
					contractsLock.Lock()
					seenContracts[normalizedContractAddress] = true
					contractChan <- objktContract
					contractsLock.Unlock()
				} else {
					logger.For(ctx).Errorf("could not fetch %s from objkt: %s", tIDs, err)
					sentryutil.ReportError(ctx, err)
				}
			}

			tokenChan <- token
			contractsLock.Lock()
			if !seenContracts[normalizedContractAddress] {
				seenContracts[normalizedContractAddress] = true
				contractsLock.Unlock()
				contract, err := p.GetContractByAddress(ctx, persist.Address(normalizedContractAddress))
				if err != nil {
					errChan <- err
					return
				}
				contract.Descriptors.Symbol = tzToken.Token.Metadata.Symbol
				contractChan <- contract
			} else {
				contractsLock.Unlock()
			}
		})
	}
	go func() {
		defer cancel()
		wp.StopWait()
	}()

	resultTokens := make([]multichain.ChainAgnosticToken, 0, len(tzTokens))
	resultContracts := make([]multichain.ChainAgnosticContract, 0, len(tzTokens))
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return resultTokens, resultContracts, nil
			}
			return nil, nil, ctx.Err()
		case err := <-errChan:
			if err != nil {
				return nil, nil, err
			}
		case token := <-tokenChan:
			resultTokens = append(resultTokens, token)
		case contract := <-contractChan:
			resultContracts = append(resultContracts, contract)
		}
	}
}

func dedupeBalances(tzTokens []tzktBalanceToken) []tzktBalanceToken {
	seen := map[string]tzktBalanceToken{}
	result := make([]tzktBalanceToken, 0, len(tzTokens))
	for _, t := range tzTokens {
		id := multichain.ChainAgnosticIdentifiers{ContractAddress: t.Token.Contract.Address, TokenID: persist.TokenID(t.Token.TokenID)}
		seen[id.String()] = t
	}
	for _, t := range seen {
		result = append(result, t)
	}
	return result
}

func (p *Provider) tzContractToContract(ctx context.Context, tzContract tzktContract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address: persist.Address(tzContract.Address),

		LatestBlock: persist.BlockNumber(tzContract.LastActivity),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Name:         tzContract.Alias,
			OwnerAddress: persist.Address(tzContract.Creator.Address),
		},
	}
}
