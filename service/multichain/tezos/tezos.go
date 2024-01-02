package tezos

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"blockwatch.cc/tzgo/tezos"
	"github.com/gammazero/workerpool"
	mgql "github.com/machinebox/graphql"
	sgql "github.com/shurcooL/graphql"
	"golang.org/x/crypto/blake2b"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
)

const (
	hicEtNunc = "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton"
	objktCom  = "KT19xbD2xn6A81an18S35oKtnkFNr9CVwY5m"
	fxHash    = "KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE"
	fxHash2   = "KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi"
	fxHash3   = "KT1EfsNuqwLAWDd3o4pvfUx1CAh5GMdTrRvr"
	fxHash4   = "KT1GtbuswcNMGhHF2TSuH1Yfaqn16do8Qtva"
)

var hicContracts = []persist.Address{
	persist.Address(hicEtNunc),
	persist.Address(objktCom),
}

var fxContracts = []persist.Address{
	persist.Address(fxHash),
	persist.Address(fxHash2),
	persist.Address(fxHash3),
	persist.Address(fxHash4),
}

func IsHicEtNunc(contract persist.Address) bool {
	return util.Contains(hicContracts, contract)
}

func IsFxHash(contract persist.Address) bool {
	return util.Contains(fxContracts, contract)
}

const pageSize = 1000

const tezDomainsApiURL = "https://api.tezos.domains/graphql"
const fxHashGQLApiURL = "https://api.fxhash.xyz/graphql"

type tokenStandard string

const (
	tokenStandardFa12 tokenStandard = "fa1.2"
	tokenStandardFa2  tokenStandard = "fa2"
)

const tezosNoncePrepend = "Tezos Signed Message: "

type ErrNoTokensFoundByIdentifiers struct {
	tokenIdentifiers multichain.ChainAgnosticIdentifiers
}

func (e ErrNoTokensFoundByIdentifiers) Error() string {
	return fmt.Sprintf("no token found for token identifiers: %s", e.tokenIdentifiers.String())
}

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

type tokenID string
type balance string

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
		TokenID  tokenID       `json:"tokenId"`
		Standard tokenStandard `json:"standard"`
		Metadata tzMetadata    `json:"metadata"`
	} `json:"token"`
	Balance    balance `json:"balance"`
	FirstLevel uint64  `json:"firstLevel"`
	LastLevel  uint64  `json:"lastLevel"`
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
	apiURL       string
	httpClient   *http.Client
	tzDomainsGQL *mgql.Client
	fxGQL        *sgql.Client
}

// NewProvider creates a new Tezos Provider
func NewProvider(httpClient *http.Client) *Provider {
	return &Provider{
		apiURL:       env.GetString("TEZOS_API_URL"),
		httpClient:   httpClient,
		tzDomainsGQL: mgql.NewClient(tezDomainsApiURL, mgql.WithHTTPClient(httpClient)),
		fxGQL:        sgql.NewClient(fxHashGQLApiURL, http.DefaultClient),
	}
}

// GetBlockchainInfo retrieves blockchain info for Tezos
func (d *Provider) GetBlockchainInfo() multichain.BlockchainInfo {
	return multichain.BlockchainInfo{
		Chain:      persist.ChainTezos,
		ChainID:    0,
		ProviderID: "tezos",
	}
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Tezos Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tzAddr, err := toTzAddress(addr)
	if err != nil {
		return nil, nil, err
	}
	limit := pageSize
	offset := 0
	resultTokens := []tzktBalanceToken{}
	for {

		tzktBalances, err := d.fetchBalancesByAddress(ctx, tzAddr, limit, offset)
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

	return d.tzBalanceTokensToTokens(ctx, resultTokens, addr.String())
}

func (d *Provider) GetTokensIncrementallyByWalletAddress(ctx context.Context, addr persist.Address) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	go func() {
		defer close(rec)

		tzAddr, err := toTzAddress(addr)
		if err != nil {
			errChan <- err
			return
		}
		limit := 100
		offset := 0
		for {
			tzktBalances, err := d.fetchBalancesByAddress(ctx, tzAddr, limit, offset)
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(tzktBalances), tzAddr.String(), limit, offset)

			resultTokens, resultContracts, err := d.tzBalanceTokensToTokens(ctx, tzktBalances, addr.String())
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

func (d *Provider) fetchBalancesByAddress(ctx context.Context, tzAddr persist.Address, limit, offset int) ([]tzktBalanceToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&limit=%d&sort.asc=id&offset=%d", d.apiURL, tzAddr.String(), limit, offset), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	offset := startOffset
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {

		tzktBalances, err := d.fetchBalancesByContract(ctx, contractAddress, limit, offset)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, resultTokens, contractAddress.String())
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contractAddress) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tez contract found for address: %s", contractAddress)
	}
	contract := contracts[0]

	return tokens, contract, nil
}

func (d *Provider) fetchBalancesByContract(ctx context.Context, contractAddress persist.Address, limit, offset int) ([]tzktBalanceToken, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.contract=%s&limit=%d&offset=%d", d.apiURL, contractAddress.String(), limit, offset), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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

func (d *Provider) GetTokensIncrementallyByContractAddress(ctx context.Context, addr persist.Address, maxLimit int) (<-chan multichain.ChainAgnosticTokensAndContracts, <-chan error) {
	rec := make(chan multichain.ChainAgnosticTokensAndContracts)
	errChan := make(chan error)
	go func() {
		defer close(rec)

		tzAddr, err := toTzAddress(addr)
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
			tzktBalances, err := d.fetchBalancesByContract(ctx, tzAddr, limit, offset)
			if err != nil {
				errChan <- err
				return
			}

			logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(tzktBalances), tzAddr.String(), limit, offset)

			resultTokens, resultContracts, err := d.tzBalanceTokensToTokens(ctx, tzktBalances, addr.String())
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

// GetTokensByContractAddressAndOwner retrieves tokens for a contract address and owner on the Tezos Blockchain
func (d *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, owner, contractAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	offset := startOffset
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?account=%s&token.standard=fa2&token.contract=%s&limit=%d&offset=%d", d.apiURL, owner, contractAddress.String(), limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := retry.RetryRequest(d.httpClient, req)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, resultTokens, contractAddress.String())
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contractAddress) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no tez contract found for address: %s", contractAddress)
	}
	contract := contracts[0]

	return tokens, contract, nil
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Tezos Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := startOffset
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&limit=%d&offset=%d", d.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := retry.RetryRequest(d.httpClient, req)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, resultTokens, tokenIdentifiers.String())
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}

	return tokens, contract, nil

}

func (d *Provider) GetTokenByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&account=%s&limit=1", d.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, ownerAddress), nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, tzktBalances, tokenIdentifiers.String())
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
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrNoTokensFoundByIdentifiers{tokenIdentifiers}
	}
}

func (d *Provider) GetTokenDescriptorsByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) (multichain.ChainAgnosticTokenDescriptors, multichain.ChainAgnosticContractDescriptors, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&limit=1", d.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress), nil)
	if err != nil {
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, tzktBalances, tokenIdentifiers.String())
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
		return multichain.ChainAgnosticTokenDescriptors{}, multichain.ChainAgnosticContractDescriptors{}, ErrNoTokensFoundByIdentifiers{tokenIdentifiers}
	}

	return token.Descriptors, contract.Descriptors, nil
}

// GetContractByAddress retrieves an Tezos contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/contracts/%s?type=contract", d.apiURL, addr.String()), nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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

	return d.tzContractToContract(ctx, tzktContract), nil

}

/*
	{
	      "type":"origination",
	      "id":105425282269184,
	      "level":1810181,
	      "timestamp":"2021-10-26T14:03:54Z",
	      "block":"BM6K7gb9sc2mX5MFyEYBb1usiMVxKgb4nHWDBew33NKehXxGLZo",
	      "hash":"oo9UKfbsWiAyB6ju8HvjLeistxRNYirVQ6Lf1ypdoFW2aBn4pM2",
	      "counter":32520298,
	      "sender":{
	         "alias":"FXHASH Admin",
	         "address":"tz1fepn7jZsCYBqCDhpM63hzh9g2Ytqk4Tpv"
	      },
	      "gasLimit":2456,
	      "gasUsed":2356,
	      "storageLimit":4275,
	      "storageUsed":4018,
	      "bakerFee":4358,
	      "storageFee":1004500,
	      "allocationFee":64250,
	      "contractBalance":0,
	      "status":"applied",
	      "originatedContract":{
	         "kind":"asset",
	         "address":"KT1LhC3ZcG8bnDbqvyDv3F7TPWkBSQ5fCqCs",
	         "typeHash":-266181292,
	         "codeHash":956470470,
	         "tzips":[
	            "fa2"
	         ]
	      }
	   }
*/
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
func (d *Provider) GetContractsByOwnerAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticContract, error) {

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/operations/originations?sender=%s", d.apiURL, addr.String()), nil)
	if err != nil {
		return nil, err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
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

/*
fxHash owned collections query:
query account($usernameOrAddress: String ) {
  account(usernameOrAddress: $usernameOrAddress) {
    username
    generativeTokens {
      gentkContractAddress
      issuerContractAddress
      name
      slug
      thumbnailUri

      author {
        account {
          wallets {
            address
          }
        }
      }
    }
  }
}


*/

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address, maxLimit, startOffset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := 0
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&token.contract=%s&limit=%d&offset=%d", d.apiURL, ownerAddress, contractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := retry.RetryRequest(d.httpClient, req)
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

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, resultTokens, fmt.Sprintf("%s:%s", contractAddress, ownerAddress))
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	var contract multichain.ChainAgnosticContract
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

/*
gql example
{
  reverseRecords(
    where: {
      address: {
        in: [
          "KT1Mqx5meQbhufngJnUAGEGpa4ZRxhPSiCgB"
          "KT1GBZmSxmnKJXGMdMLbugPfLyUPmuLSMwKS"
        ]
      }
    }
  ) {
    items {
      address
      owner
      domain {
        name
      }
    }
  }
}

example response
{
  "data": {
    "domains": {
      "items": [
        {
          "address": "tz1VxMudmADssPp6FPDGRsvJXE41DD6i9g6n",
          "name": "aaa.tez",
          "owner": "tz1VxMudmADssPp6FPDGRsvJXE41DD6i9g6n",
          "level": 2
        },
        {
          "address": null,
          "name": "a.aaa.tez",
          "owner": "tz1VxMudmADssPp6FPDGRsvJXE41DD6i9g6n",
          "level": 3
        },
        {
          "address": null,
          "name": "alice.tez",
          "owner": "tz1Q4vimV3wsfp21o7Annt64X7Hs6MXg9Wix",
          "level": 2
        }
      ]
    }
  },
  "extensions": {}
}
*/

type tezDomainResponse struct {
	Data struct {
		Domains struct {
			Items []struct {
				Address string `json:"address"`
				Name    string `json:"name"`
				Owner   string `json:"owner"`
				Level   int    `json:"level"`
			} `json:"items"`
		} `json:"domains"`
	} `json:"data"`
}

func (d *Provider) GetDisplayNameByAddress(ctx context.Context, addr persist.Address) string {
	req := mgql.NewRequest(fmt.Sprintf(`{
	  "query": "query ($addresses: [String!]) {
		reverseRecords(
			where: {
				address: {
					in: $addresses
				}
			}
		) {
			items {
				domain {
					name
				}
			}
		}
	}",
	  "variables": {
		"addresses": [
			%s
		]
	  }
	}`, addr.String()))

	resp := tezDomainResponse{}
	err := d.tzDomainsGQL.Run(ctx, req, &resp)
	if err != nil {
		return addr.String()
	}
	if len(resp.Data.Domains.Items) == 0 {
		return addr.String()
	}
	return resp.Data.Domains.Items[0].Name
}

func (d *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) (persist.TokenMetadata, error) {
	t, _, err := d.GetTokensByTokenIdentifiers(ctx, ti, 1, 0)
	if err != nil {
		return persist.TokenMetadata{}, err
	}
	if len(t) == 0 {
		return persist.TokenMetadata{}, fmt.Errorf("no token found for %s", ti)
	}
	return t[0].TokenMetadata, nil
}

// RefreshToken refreshes the metadata for a given token.
func (d *Provider) RefreshToken(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, owner persist.Address) error {
	return nil
}

// UpdateMediaForWallet updates media for the tokens owned by a wallet on the Tezos Blockchain
func (d *Provider) UpdateMediaForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	return nil
}

// RefreshContract refreshes the metadata for a contract
func (d *Provider) RefreshContract(ctx context.Context, addr persist.Address) error {
	return nil
}

// VerifySignature will verify a signature using the ed25519 algorithm
// the address provided must be the tezos public key, not the hashed address
func (d *Provider) VerifySignature(pCtx context.Context, pPubKey persist.PubKey, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {
	key, err := tezos.ParseKey(pPubKey.String())
	if err != nil {
		return false, err
	}
	sig, err := tezos.ParseSignature(pSignatureStr)
	if err != nil {
		return false, err
	}
	nonce := tezosNoncePrepend + auth.NewNoncePrepend + pNonce
	asBytes := []byte(nonce)
	asHex := hex.EncodeToString(asBytes)
	lenHexBytes := []byte(fmt.Sprintf("%d", len(asHex)))

	asBytes = append(lenHexBytes, asBytes...)
	// these three bytes will always be at the front of a hashed signed message according to the tezos standard
	// https://tezostaquito.io/docs/signing/
	asBytes = append([]byte{0x05, 0x01, 0x00}, asBytes...)

	hash, err := blake2b.New256(nil)
	if err != nil {
		return false, err
	}
	_, err = hash.Write(asBytes)
	if err != nil {
		return false, err
	}

	return key.Verify(hash.Sum(nil), sig) == nil, nil
}

func (d *Provider) tzBalanceTokensToTokens(pCtx context.Context, tzTokens []tzktBalanceToken, mediaKey string) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
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
			if tzToken.Token.Standard == tokenStandardFa12 {
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
			tid := persist.TokenID(tzToken.Token.TokenID.toBase16String())

			agnostic := multichain.ChainAgnosticToken{
				TokenType: persist.TokenTypeERC1155,
				Descriptors: multichain.ChainAgnosticTokenDescriptors{
					Description: tzToken.Token.Metadata.Description,
					Name:        tzToken.Token.Metadata.Name,
				},
				TokenID: tid,
				FallbackMedia: persist.FallbackMedia{
					ImageURL: persist.NullString(tzToken.Token.Metadata.Image),
				},
				ContractAddress: tzToken.Token.Contract.Address,
				Quantity:        persist.HexString(tzToken.Balance.toBase16String()),
				TokenMetadata:   agnosticMetadata,
				OwnerAddress:    tzToken.Account.Address,
				BlockNumber:     persist.BlockNumber(tzToken.LastLevel),
			}

			tokenChan <- agnostic
			contractsLock.Lock()
			if !seenContracts[normalizedContractAddress] {
				seenContracts[normalizedContractAddress] = true
				contractsLock.Unlock()
				contract, err := d.GetContractByAddress(ctx, persist.Address(normalizedContractAddress))
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

func (d *Provider) tzContractToContract(ctx context.Context, tzContract tzktContract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address: persist.Address(tzContract.Address),

		LatestBlock: persist.BlockNumber(tzContract.LastActivity),
		Descriptors: multichain.ChainAgnosticContractDescriptors{
			Name:         tzContract.Alias,
			OwnerAddress: persist.Address(tzContract.Creator.Address),
		},
	}
}

func toTzAddress(address persist.Address) (persist.Address, error) {
	if strings.HasPrefix(address.String(), "tz") {
		return address, nil
	}
	key, err := tezos.ParseKey(address.String())
	if err != nil {
		return "", err
	}
	return persist.Address(key.Address().String()), nil
}

func (t tokenID) String() string {
	return string(t)
}
func (t tokenID) toBase16String() string {
	asInt, ok := big.NewInt(0).SetString(t.String(), 10)
	if !ok {
		panic(fmt.Sprintf("failed to convert tokenID to int: %s", t))
	}
	return asInt.Text(16)
}

func (b balance) String() string {
	return string(b)
}
func (b balance) toBase16String() string {
	asInt, ok := big.NewInt(0).SetString(b.String(), 10)
	if !ok {
		panic(fmt.Sprintf("failed to convert tokenID to int: %s", b))
	}
	return asInt.Text(16)
}

func (b balance) ToBigInt() *big.Int {
	asInt, ok := big.NewInt(0).SetString(b.String(), 10)
	if !ok {
		panic(fmt.Sprintf("failed to convert tokenID to int: %s", b))
	}
	return asInt
}

// IsFxHashSigned returns false if the token is an unsigned FxHash token (metadata hasn't yet been uploaded to the FxHash contract).
// It's possible for tzkt to index a token before FxHash signs a token which results in the API returning placeholder metadata.
// This seems to happen infrequently, but there are a few cases where the token is never updated with the signed metadata
// (either because of tzkt failing to update the metadata, or FxHash never signing the token). If it's the former, we want to
// fallback to an alternative provider in case there might be usable metadata elsewhere.
func IsFxHashSigned(contract persist.Address, tokenName string) bool {
	return !IsFxHash(contract) || tokenName != "[WAITING TO BE SIGNED]"
}

// ContainsTezosKeywords returns true if the token's metadata has at least one non-empty Tezos keyword.
// The tzkt API sometimes returns completely empty metadata, in which case we want to fallback
// to an alternative provider.
func ContainsTezosKeywords(token multichain.ChainAgnosticToken) bool {
	imageKeywords, animationKeywords := persist.ChainTezos.BaseKeywords()
	for field, val := range token.TokenMetadata {

		for _, keyword := range imageKeywords {
			if field == keyword && (val != nil && val != "") {
				return true
			}
		}

		for _, keyword := range animationKeywords {
			if field == keyword && (val != nil && val != "") {
				return true
			}
		}
	}
	return false
}
