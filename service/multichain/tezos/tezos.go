package tezos

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
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
	"github.com/mikeydub/go-gallery/util"
	"golang.org/x/crypto/blake2b"
)

const limit = 1000

type tokenStandard string

const (
	tokenStandardFa12 tokenStandard = "fa1.2"
	tokenStandardFa2  tokenStandard = "fa2"
)

const tezosNoncePrepend = "Tezos Signed Message: "

type tzMetadata struct {
	Date               string      `json:"date"`
	Name               string      `json:"name"`
	Tags               interface{} `json:"tags"`
	Image              string      `json:"image"`
	Minter             string      `json:"minter"`
	Rights             string      `json:"rights"`
	Symbol             string      `json:"symbol"`
	Formats            interface{}
	Creators           interface{} `json:"creators"`
	Decimals           string      `json:"decimals"`
	Attributes         interface{}
	DisplayURI         string      `json:"displayUri"`
	ArtifactURI        string      `json:"artifactUri"`
	Description        string      `json:"description"`
	MintingTool        string      `json:"mintingTool"`
	ThumbnailURI       string      `json:"thumbnailUri"`
	IsBooleanAmount    interface{} `json:"isBooleanAmount"`
	ShouldPreferSymbol interface{} `json:"shouldPreferSymbol"`
}

type tzAccount struct {
	Address string `json:"address"`
	Alias   string `json:"alias"`
	Public  string `json:"publicKey"`
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
		storageClient:  storageClient,
		tokenBucket:    tokenBucket,
	}
}

// GetBlockchainInfo retrieves blockchain info for ETH
func (d *Provider) GetBlockchainInfo(ctx context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainTezos,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress retrieves tokens for a wallet address on the Tezos Blockchain
func (d *Provider) GetTokensByWalletAddress(ctx context.Context, addr persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	tzAddr, err := toTzAddress(addr)
	if err != nil {
		return nil, nil, err
	}
	offset := 0
	resultTokens := []tzktBalanceToken{}
	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&limit=%d&sort.asc=id&offset=%d", d.apiURL, tzAddr.String(), limit, offset), nil)
		if err != nil {
			return nil, nil, err
		}
		resp, err := d.httpClient.Do(req)
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

		if len(tzktBalances) < limit {
			break
		}

		offset += limit

		logger.For(ctx).Debugf("retrieved %d tokens for address %s (limit %d offset %d)", len(resultTokens), tzAddr.String(), limit, offset)
	}

	return d.tzBalanceTokensToTokens(ctx, resultTokens, addr.String())

}

// GetTokensByContractAddress retrieves tokens for a contract address on the Tezos Blockchain
func (d *Provider) GetTokensByContractAddress(ctx context.Context, contractAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {

	offset := 0
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.contract=%s&limit=%d&offset=%d", d.apiURL, contractAddress.String(), limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := d.httpClient.Do(req)
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

		if len(tzktBalances) < limit {
			break
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
func (d *Provider) GetTokensByTokenIdentifiers(ctx context.Context, tokenIdentifiers multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := 0
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&limit=%d&offset=%d", d.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := d.httpClient.Do(req)
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

		if len(tzktBalances) < limit {
			break
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

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&token.tokenId=%s&token.contract=%s&account=%d&limit=1", d.apiURL, tokenIdentifiers.TokenID.Base10String(), tokenIdentifiers.ContractAddress, ownerAddress), nil)
	if err != nil {
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, err
	}
	resp, err := d.httpClient.Do(req)
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
		return multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, fmt.Errorf("no token found for token identifiers: %s", tokenIdentifiers.String())
	}

	return token, contract, nil
}

// GetContractByAddress retrieves an Tezos contract by address
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
		return multichain.ChainAgnosticContract{}, util.GetErrFromResp(resp)
	}
	var tzktContract tzktContract
	if err := json.NewDecoder(resp.Body).Decode(&tzktContract); err != nil {
		return multichain.ChainAgnosticContract{}, err
	}

	return d.tzContractToContract(ctx, tzktContract), nil

}

func (d *Provider) GetOwnedTokensByContract(ctx context.Context, contractAddress persist.Address, ownerAddress persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	offset := 0
	resultTokens := []tzktBalanceToken{}

	for {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v1/tokens/balances?token.standard=fa2&account=%s&token.contract=%s&limit=%d&offset=%d", d.apiURL, ownerAddress, contractAddress, limit, offset), nil)
		if err != nil {
			return nil, multichain.ChainAgnosticContract{}, err
		}
		resp, err := d.httpClient.Do(req)
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

		if len(tzktBalances) < limit {
			break
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

func (d *Provider) GetCommunityOwners(ctx context.Context, contractAddress persist.Address) ([]multichain.ChainAgnosticCommunityOwner, error) {
	tokens, _, err := d.GetTokensByContractAddress(ctx, contractAddress)
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
			med := d.makeTempMedia(tid, tzToken.Token.Contract.Address, agnosticMetadata, fmt.Sprintf("%s/%s-%s", mediaKey, tzToken.Token.Contract.Address, tzToken.Token.TokenID))

			agnostic := multichain.ChainAgnosticToken{
				TokenType:       persist.TokenTypeERC1155,
				Description:     tzToken.Token.Metadata.Description,
				Name:            tzToken.Token.Metadata.Name,
				TokenID:         tid,
				Media:           med,
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

func (d *Provider) makeTempMedia(tokenID persist.TokenID, contract persist.Address, agnosticMetadata persist.TokenMetadata, name string) persist.Media {
	med := persist.Media{
		MediaType: persist.MediaTypeSyncing,
	}
	img, anim := media.FindImageAndAnimationURLs(tokenID, contract, agnosticMetadata, "", media.TezAnimationKeywords(multichain.TezAnimationKeywords), media.TezImageKeywords(multichain.TezImageKeywords), name)
	if persist.TokenURI(anim).Type() == persist.URITypeIPFS {
		removedIPFS := strings.Replace(anim, "ipfs://", "", 1)
		removedIPFS = strings.Replace(removedIPFS, "ipfs/", "", 1)
		anim = fmt.Sprintf("%s/ipfs/%s", d.ipfsGatewayURL, removedIPFS)
	}
	if persist.TokenURI(img).Type() == persist.URITypeIPFS {
		removedIPFS := strings.Replace(img, "ipfs://", "", 1)
		removedIPFS = strings.Replace(removedIPFS, "ipfs/", "", 1)
		img = fmt.Sprintf("%s/ipfs/%s", d.ipfsGatewayURL, removedIPFS)
	}
	if anim != "" {
		if persist.TokenURI(anim).Type() == persist.URITypeIPFS {
			removedIPFS := strings.Replace(anim, "ipfs://", "", 1)
			removedIPFS = strings.Replace(removedIPFS, "ipfs/", "", 1)
			anim = fmt.Sprintf("%s/ipfs/%s", d.ipfsGatewayURL, removedIPFS)
		}
		med.MediaURL = persist.NullString(anim)
		if img != "" {
			med.ThumbnailURL = persist.NullString(img)
		}
	} else if img != "" {
		med.MediaURL = persist.NullString(img)
	}
	return med
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
	resp, err := d.httpClient.Do(req)
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
