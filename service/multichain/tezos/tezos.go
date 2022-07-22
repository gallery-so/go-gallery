package tezos

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"

	"blockwatch.cc/tzgo/tezos"
	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"golang.org/x/crypto/blake2b"
)

type tokenStandard string

const (
	tokenStandardFa12 tokenStandard = "fa1.2"
	tokenStandardFa2  tokenStandard = "fa2"
)

const tezosNoncePrepend = "Tezos Signed Message: "

type tzMetadata struct {
	Date    string   `json:"date"`
	Name    string   `json:"name"`
	Tags    []string `json:"tags"`
	Image   string   `json:"image"`
	Minter  string   `json:"minter"`
	Rights  string   `json:"rights"`
	Symbol  string   `json:"symbol"`
	Formats []struct {
		URI        string `json:"uri"`
		FileName   string `json:"fileName"`
		FileSize   string `json:"fileSize"`
		MimeType   string `json:"mimeType"`
		Dimensions struct {
			Unit  string `json:"unit"`
			Value string `json:"value"`
		} `json:"dimensions"`
	} `json:"formats"`
	Creators  []string `json:"creators"`
	Decimals  string   `json:"decimals"`
	Royalties struct {
		Shares   map[string]string `json:"shares"`
		Decimals string            `json:"decimals"`
	} `json:"royalties"`
	Attributes []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"attributes"`
	DisplayURI         string `json:"displayUri"`
	ArtifactURI        string `json:"artifactUri"`
	Description        string `json:"description"`
	MintingTool        string `json:"mintingTool"`
	ThumbnailURI       string `json:"thumbnailUri"`
	IsBooleanAmount    bool   `json:"isBooleanAmount"`
	ShouldPreferSymbol bool   `json:"shouldPreferSymbol"`
}

type tokenID string
type balance string

type tzktToken struct {
	ID       uint64 `json:"id"`
	Contract struct {
		Alias   string          `json:"alias"`
		Address persist.Address `json:"address"`
	} `json:"contract"`
	TokenID    tokenID       `json:"tokenId"`
	Standard   tokenStandard `json:"standard"`
	Metadata   tzMetadata    `json:"metadata"`
	FirstLevel uint64        `json:"firstLevel"`
	LastLevel  uint64        `json:"lastLevel"`
}

type tzktBalanceToken struct {
	ID      string `json:"id"`
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

// Provider is an the struct for retrieving data from the Ethereum blockchain
type Provider struct {
	apiURL        string
	httpClient    *http.Client
	ipfsClient    *shell.Shell
	arweaveClient *goar.Client
	storageClient *storage.Client
	tokenBucket   string
}

// NewProvider creates a new ethereum Provider
func NewProvider(indexerBaseURL string, httpClient *http.Client, ipfsClient *shell.Shell, arweaveCleint *goar.Client, storageClient *storage.Client, tokenBucket string) *Provider {
	return &Provider{
		apiURL:        indexerBaseURL,
		httpClient:    httpClient,
		ipfsClient:    ipfsClient,
		arweaveClient: arweaveCleint,
		storageClient: storageClient,
		tokenBucket:   tokenBucket,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s", d.apiURL, addr.String()), nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, getErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return nil, nil, err
	}

	return d.tzBalanceTokensToTokens(ctx, tzktBalances)

}

// GetTokensByContractAddress retrieves tokens for a contract address on the Ethereum Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.contract=%s", d.apiURL, contractAddress.String()), nil)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, multichain.ChainAgnosticContract{}, getErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}

	tokens, contracts, err := d.tzBalanceTokensToTokens(ctx, tzktBalances)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	if len(contractAddress) == 0 {
		return nil, multichain.ChainAgnosticContract{}, fmt.Errorf("no contract found for address: %s", contractAddress)
	}
	contract := contracts[0]
	return tokens, contract, nil
}

// GetTokensByTokenIdentifiers retrieves tokens for a token identifiers on the Ethereum Blockchain
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s", d.apiURL, tokenIdentifiers.TokenID, tokenIdentifiers.ContractAddress), nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, getErrFromResp(resp)
	}
	var tzktBalances []tzktBalanceToken
	if err := json.NewDecoder(resp.Body).Decode(&tzktBalances); err != nil {
		return nil, nil, err
	}

	return d.tzBalanceTokensToTokens(ctx, tzktBalances)
}

// GetContractByAddress retrieves an ethereum contract by address
func (d *Provider) GetContractByAddress(ctx context.Context, addr persist.Address) (multichain.ChainAgnosticContract, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/contracts/%s?type=contract", d.apiURL, addr.String()), nil)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return multichain.ChainAgnosticContract{}, getErrFromResp(resp)
	}
	var tzktContract tzktContract
	if err := json.NewDecoder(resp.Body).Decode(&tzktContract); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return d.tzContractToContract(ctx, tzktContract), nil

}

// RefreshToken refreshes the metadata for a given token.
func (d *Provider) RefreshToken(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) error {
	return nil
}

// UpdateMediaForWallet updates media for the tokens owned by a wallet on the Ethereum Blockchain
func (d *Provider) UpdateMediaForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	return nil
}

// RefreshContract refreshes the metadata for a contract
func (d *Provider) RefreshContract(ctx context.Context, addr persist.Address) error {
	return nil
}

// ValidateTokensForWallet validates tokens for a wallet address on the Ethereum Blockchain
func (d *Provider) ValidateTokensForWallet(ctx context.Context, wallet persist.Address, all bool) error {
	return nil

}

// VerifySignature will verify a signature using the ed25519 algorithm
// the address provided must be the tezos public key, not the hashed address
func (d *Provider) VerifySignature(pCtx context.Context, pAddressStr persist.Address, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {
	key, err := tezos.ParseKey(pAddressStr.String())
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
	asBytes = append([]byte{0x05, 0x01, 0x00}, asBytes...)

	hash, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}
	hash.Write(asBytes)

	return key.Verify(hash.Sum(nil), sig) == nil, nil
}

func (d *Provider) tzBalanceTokensToTokens(pCtx context.Context, tzTokens []tzktBalanceToken) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
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
			med, err := media.MakePreviewsForMetadata(ctx, agnosticMetadata, normalizedContractAddress, tid, "", persist.ChainTezos, d.ipfsClient, d.arweaveClient, d.storageClient, d.tokenBucket, []string{"image", "displayUri", "thumbnailUri", "artifactUri", "uri"}, []string{"artifactUri", "displayUri", "uri", "image"})
			if err != nil {
				logger.For(ctx).Errorf("Failed to make previews for tezos token %s: %s", tid, err)
			}

			tokenType := persist.TokenTypeERC721
			if tzToken.Balance.ToBigInt().Cmp(big.NewInt(1)) > 0 {
				tokenType = persist.TokenTypeERC1155
			} else {
				err := func() error {
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances/count?id=%d", d.apiURL, tzToken.Token.ID), nil)
					if err != nil {
						return err
					}
					resp, err := d.httpClient.Do(req)
					if err != nil {
						return err
					}
					defer resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						return getErrFromResp(resp)
					}
					// body will just be an integer so decode it onto an integer
					var count int
					if err := json.NewDecoder(resp.Body).Decode(&count); err != nil {
						return err
					}

					if count > 1 {
						tokenType = persist.TokenTypeERC1155
					}

					return nil
				}()
				if err != nil {
					errChan <- err
					return
				}

			}
			agnostic := multichain.ChainAgnosticToken{
				Media:           med,
				TokenType:       tokenType,
				Description:     tzToken.Token.Metadata.Description,
				Name:            tzToken.Token.Metadata.Name,
				TokenID:         tid,
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

func (d *Provider) tzContractToContract(ctx context.Context, tzContract tzktContract) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address:        persist.Address(tzContract.Address),
		CreatorAddress: persist.Address(tzContract.Creator.Address),
		LatestBlock:    persist.BlockNumber(tzContract.LastActivity),
		Name:           tzContract.Alias,
	}
}

func getErrFromResp(res *http.Response) error {
	errResp := map[string]interface{}{}
	json.NewDecoder(res.Body).Decode(&errResp)
	return fmt.Errorf("unexpected status: %s | err: %v ", res.Status, errResp)
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
