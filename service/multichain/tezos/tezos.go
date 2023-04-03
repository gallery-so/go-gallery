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
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/machinebox/graphql"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"golang.org/x/crypto/blake2b"
)

const pageSize = 1000

const tezDomainsApiURL = "https://api.tezos.domains/graphql"

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

type tzAccount struct {
	Address string `json:"address"`
	Alias   string `json:"alias"`
	Public  string `json:"publicKey"`
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
	apiURL         string
	mediaURL       string
	ipfsGatewayURL string
	httpClient     *http.Client
	ipfsClient     *shell.Shell
	arweaveClient  *goar.Client
	storageClient  *storage.Client
	graphQL        *graphql.Client
	tokenBucket    string
}

// NewProvider creates a new Tezos Provider
func NewProvider(tezosAPIUrl, mediaURL, ipfsGatewayURL string, httpClient *http.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client, tokenBucket string) *Provider {
	return &Provider{
		apiURL:         tezosAPIUrl,
		mediaURL:       mediaURL,
		ipfsGatewayURL: ipfsGatewayURL,
		httpClient:     httpClient,
		ipfsClient:     ipfsClient,
		arweaveClient:  arweaveClient,
		graphQL:        graphql.NewClient(tezDomainsApiURL, graphql.WithHTTPClient(httpClient)),
		storageClient:  storageClient,
		tokenBucket:    tokenBucket,
	}
}

// GetBlockchainInfo retrieves blockchain info for Tezos
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Tezos Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address, maxLimit, startingOffset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tzAddr, err := toTzAddress(addr)
	if err != nil {
		return nil, nil, err
	}
	limit := int(math.Min(float64(maxLimit), float64(pageSize)))
	if limit < 1 {
		limit = pageSize
	}
	offset := startingOffset
	resultTokens := []tzktBalanceToken{}
	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&limit=%d&sort.asc=id&offset=%d", d.apiURL, tzAddr.String(), limit, offset), nil)
		if err != nil {
			return nil, nil, err
		}
		resp, err := retry.RetryRequest(d.httpClient, req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, nil, util.GetErrFromResp(resp)
		}
		var tzktBalances []tzktBalanceToken
		if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
			return nil, nil, err
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

		logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), pageSize, offset)
	}

	return d.tzBalanceTokensToTokens(ctx, resultTokens, addr.String())
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
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.contract=%s&limit=%d&offset=%d", d.apiURL, contractAddress.String(), limit, offset), nil)
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
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for address: %s", contractAddress)
	}
	contract := contracts[0]

	return tokens, contract, nil
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
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for address: %s", contractAddress)
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

func (d *Provider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

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
	token := multichain.ChainAgnosticToken{}
	if len(tokens) > 0 {
		token = tokens[0]
	} else {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, ErrNoTokensFoundByIdentifiers{tokenIdentifiers}
	}

	return token, contract, nil
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

func (d *Provider) GetCommunityOwners(ctx context.Context, contractAddress persist.Address, maxLimit, maxOffset int) ([]multichain.ChainAgnosticCommunityOwner, error) {
	tokens, _, err := d.GetTokensByContractAddress(ctx, contractAddress, maxLimit, maxOffset)
	if err != nil {
		return nil, err
	}
	owners := make([]multichain.ChainAgnosticCommunityOwner, len(tokens))
	for i, token := range tokens {
		owners[i] = multichain.ChainAgnosticCommunityOwner{
			Address: token.OwnerAddress,
		}
	}
	return owners, nil

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
	req := graphql.NewRequest(fmt.Sprintf(`{
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
	err := d.graphQL.Run(ctx, req, &resp)
	if err != nil {
		return addr.String()
	}
	if len(resp.Data.Domains.Items) == 0 {
		return addr.String()
	}
	return resp.Data.Domains.Items[0].Name
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

// ValidateTokensForWallet validates tokens for a wallet address on the Tezos Blockchain
func (d *Provider) ValidateTokensForWallet(ctx context.Context, wallet persist.Address, all bool) error {
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
				TokenType:   persist.TokenTypeERC1155,
				Description: tzToken.Token.Metadata.Description,
				Name:        tzToken.Token.Metadata.Name,
				TokenID:     tid,

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
				contract.Symbol = tzToken.Token.Metadata.Symbol
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

func (d *Provider) getPublicKeyFromAddress(ctx context.Context, address persist.Address) (persist.Address, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/accounts/%s", d.apiURL, address), nil)
	if err != nil {
		return "", err
	}
	resp, err := retry.RetryRequest(d.httpClient, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", util.GetErrFromResp(resp)
	}
	var account tzAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return "", err
	}
	key, err := tezos.ParseKey(account.Public)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(key.Address().String(), address.String()) {
		return "", fmt.Errorf("public key hash %s does not match address %s", string(key.Hash()), address)
	}
	return persist.Address(account.Public), nil
}

func (d *Provider) tzContractToContract(ctx context.Context, tzContract tzktContract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address:        persist.Address(tzContract.Address),
		CreatorAddress: persist.Address(tzContract.Creator.Address),
		LatestBlock:    persist.BlockNumber(tzContract.LastActivity),
		Name:           tzContract.Alias,
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

// IsSigned returns false if the token is an unsigned FxHash token (metadata hasn't yet been uploaded to the FxHash contract).
// It's possible for tzkt to index a token before FxHash signs a token which results in the API returning placeholder metadata.
// This seems to happen infrequently, but there are a few cases where the token is never updated with the signed metadata
// (either because of tzkt failing to update the metadata, or FxHash never signing the token). If it's the former, we want to
// fallback to an alternative provider in case there might be usable metadata elsewhere.
func IsSigned(ctx context.Context, token multichain.ChainAgnosticToken) bool {
	return !media.IsFxHash(token.ContractAddress) || token.Name != "[WAITING TO BE SIGNED]"
}

// ContainsTezosKeywords returns true if the token's metadata has at least one non-empty Tezos keyword.
// The tzkt API sometimes returns completely empty metadata, in which case we want to fallback
// to an alternative provider.
func ContainsTezosKeywords(ctx context.Context, token multichain.ChainAgnosticToken) bool {
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
