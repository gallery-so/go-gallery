package opensea

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

var openseaURL, _ = url.Parse("https://api.opensea.io/api/v1/assets")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

type Provider struct {
	httpClient *http.Client
	ethClient  *ethclient.Client
}

// TokenID represents a token ID from Opensea. It is separate from persist.TokenID becuase opensea returns token IDs in base 10 format instead of what we want, base 16
type TokenID string

// Assets is a list of NFTs from OpenSea
type Assets struct {
	Next   string  `json:"next"`
	Assets []Asset `json:"assets"`
}

// Asset is an NFT from OpenSea
type Asset struct {
	Version int64 `json:"version"` // schema version for this model
	ID      int   `json:"id"`

	Name        string `json:"name"`
	Description string `json:"description"`

	ExternalURL      string              `json:"external_link"`
	TokenMetadataURL string              `json:"token_metadata"`
	Creator          Account             `json:"creator"`
	Owner            Account             `json:"owner"`
	Contract         persist.NFTContract `json:"asset_contract"`
	Collection       Collection          `json:"collection"`

	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	TokenID TokenID `json:"token_id"`

	// IMAGES - OPENSEA
	ImageURL             string `json:"image_url"`
	ImageThumbnailURL    string `json:"image_thumbnail_url"`
	ImagePreviewURL      string `json:"image_preview_url"`
	ImageOriginalURL     string `json:"image_original_url"`
	AnimationURL         string `json:"animation_url"`
	AnimationOriginalURL string `json:"animation_original_url"`

	Orders []Order `json:"orders"`

	AcquisitionDateStr string `json:"acquisition_date"`
}

// Events is a list of events from OpenSea
type Events struct {
	Events []Event `json:"asset_events"`
}

// Event is an event from OpenSea
type Event struct {
	Asset       Asset   `json:"asset"`
	FromAccount Account `json:"from_account"`
	ToAccount   Account `json:"to_account"`
	CreatedDate string  `json:"created_date"`
}

// Account is a user account from OpenSea
type Account struct {
	User    User                    `json:"user"`
	Address persist.EthereumAddress `json:"address"`
}

// User is a user from OpenSea
type User struct {
	Username string `json:"username"`
}

// Order is an order from OpenSea representing an NFT's maker and buyer
type Order struct {
	Maker Account `json:"maker"`
	Taker Account `json:"taker"`
}

// Collection is a collection from OpenSea
type Collection struct {
	Name          string                  `json:"name"`
	PayoutAddress persist.EthereumAddress `json:"payout_address"`
}

// Contract represents an NFT contract from Opensea
type Contract struct {
	Collection Collection              `json:"collection"`
	Address    persist.EthereumAddress `json:"address"`
	Symbol     string                  `json:"symbol"`
}

type assetsReceieved struct {
	assets []Asset
	err    error
}

type errNoSingleNFTForOpenseaID struct {
	openseaID int
}

// ErrNoAssetsForWallets is returned when opensea returns an empty array of assets for a wallet address
type ErrNoAssetsForWallets struct {
	Wallets []persist.EthereumAddress
}

// NewProvider creates a new provider for opensea
func NewProvider(ethClient *ethclient.Client, httpClient *http.Client) *Provider {
	return &Provider{
		httpClient: httpClient,
		ethClient:  ethClient,
	}
}

// GetBlockchainInfo returns Ethereum blockchain info
func (p *Provider) GetBlockchainInfo(context.Context) (multichain.BlockchainInfo, error) {
	return multichain.BlockchainInfo{
		Chain:   persist.ChainETH,
		ChainID: 0,
	}, nil
}

// GetTokensByWalletAddress returns a list of tokens for a wallet address
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		FetchAssets(ctx, assetsChan, persist.EthereumAddress(address.String()), "", "", "", 0, nil)
	}()

	return assetsToTokens(ctx, address, assetsChan, p.ethClient)
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		FetchAssets(ctx, assetsChan, "", persist.EthereumAddress(address), "", "", 0, nil)
	}()
	// TODO: Fill in this address or change something else
	tokens, contracts, err := assetsToTokens(ctx, "", assetsChan, p.ethClient)
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
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		FetchAssets(ctx, assetsChan, "", persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID.Base10String()), "", 0, nil)
	}()
	// TODO: Fill in this address or change something else
	return assetsToTokens(ctx, "", assetsChan, p.ethClient)
}

// GetContractByAddress returns a contract for a contract address
func (p *Provider) GetContractByAddress(ctx context.Context, contract persist.Address) (multichain.ChainAgnosticContract, error) {
	c, err := FetchContractByAddress(ctx, persist.EthereumAddress(contract), 0)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return contractToContract(ctx, c, p.ethClient)
}

// UpdateMediaForWallet updates media for a wallet address
// bool is whether or not to update all media content, including the tokens that already have media content
func (p *Provider) UpdateMediaForWallet(context.Context, persist.Address, bool) error {
	return nil
}

// ValidateTokensForWallet validates tokens for a wallet address
// bool is whether or not to update all of the tokens regardless of whether we know they exist already
func (p *Provider) ValidateTokensForWallet(context.Context, persist.Address, bool) error {
	return nil
}

// FetchAssetsForWallet recursively fetches all assets for a wallet
func FetchAssetsForWallet(pCtx context.Context, pWalletAddress persist.EthereumAddress, pCursor string, retry int, alreadyReceived map[int]string) ([]Asset, error) {

	if alreadyReceived == nil {
		alreadyReceived = make(map[int]string)
	}

	result := []Asset{}

	dir := "desc"

	logger.For(pCtx).Debugf("Fetching assets for wallet %s with cursor %s, retry %d, dir %s,and alreadyReceived %d", pWalletAddress, pCursor, retry, dir, len(alreadyReceived))

	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/assets?owner=%s&order_direction=%s&limit=%d", pWalletAddress, dir, 50)
	if pCursor != "" {
		urlStr = fmt.Sprintf("%s&cursor=%s", urlStr, pCursor)
	}

	req, err := http.NewRequestWithContext(pCtx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", viper.GetString("OPENSEA_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if retry < 3 {
				time.Sleep(time.Second * 3 * time.Duration(retry+1))
				return FetchAssetsForWallet(pCtx, pWalletAddress, pCursor, retry+1, alreadyReceived)
			}
			return nil, fmt.Errorf("opensea api rate limit exceeded")
		}
		bs := new(bytes.Buffer)
		_, err := bs.ReadFrom(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, bs.String())
	}
	response := Assets{}
	err = util.UnmarshallBody(&response, resp.Body)
	if err != nil {
		return nil, err
	}

	nextCursor := response.Next

	doneReceiving := false
	for _, asset := range response.Assets {
		if it, ok := alreadyReceived[asset.ID]; ok {
			logger.For(pCtx).Debugf("response already received asset: %s", it)
			doneReceiving = true
			continue
		}
		result = append(result, asset)
		alreadyReceived[asset.ID] = fmt.Sprintf("asset-%d|offset-%s|len-%d|dir-%s", asset.ID, pCursor, len(result), dir)
	}
	if doneReceiving {
		return result, nil
	}
	if len(response.Assets) == 50 {
		next, err := FetchAssetsForWallet(pCtx, pWalletAddress, nextCursor, 0, alreadyReceived)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

// FetchAssets fetches assets by its token identifiers
func FetchAssets(pCtx context.Context, assetsChan chan<- assetsReceieved, pWalletAddress, pContractAddress persist.EthereumAddress, pTokenID TokenID, pCursor string, retry int, alreadyReceived map[int]string) {

	if alreadyReceived == nil {
		alreadyReceived = make(map[int]string)
	}
	result := []Asset{}

	dir := "desc"

	url := *openseaURL
	query := url.Query()
	if pTokenID != "" {
		query.Set("token_ids", string(pTokenID))
	}
	if pContractAddress != "" {
		query.Set("asset_contract_address", string(pContractAddress))
	}
	if pWalletAddress != "" {
		query.Set("owner", string(pWalletAddress))
	}
	query.Set("order_direction", dir)
	query.Set("limit", "50")
	if pCursor != "" {
		query.Set("cursor", pCursor)
	}

	url.RawQuery = query.Encode()

	urlStr := url.String()

	req, err := http.NewRequestWithContext(pCtx, "GET", urlStr, nil)
	if err != nil {
		assetsChan <- assetsReceieved{err: fmt.Errorf("failed to create request for url: %s - %s", urlStr, err)}
	}
	req.Header.Set("X-API-KEY", viper.GetString("OPENSEA_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		assetsChan <- assetsReceieved{err: fmt.Errorf("failed to fetch assets for wallet %s: %s - url %s", pWalletAddress, err, urlStr)}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if retry < 3 {
				time.Sleep(time.Second * 3 * time.Duration(retry+1))
				FetchAssets(pCtx, assetsChan, pWalletAddress, pContractAddress, pTokenID, pCursor, retry+1, alreadyReceived)
				return
			}
			assetsChan <- assetsReceieved{err: fmt.Errorf("opensea api rate limit exceeded - url %s", urlStr)}
			return
		}
		bs := new(bytes.Buffer)
		_, err := bs.ReadFrom(resp.Body)
		if err != nil {
			assetsChan <- assetsReceieved{err: err}
			return
		}
		assetsChan <- assetsReceieved{err: fmt.Errorf("unexpected status code for url %s: %d - %s", urlStr, resp.StatusCode, bs.String())}
		return
	}
	response := Assets{}
	err = util.UnmarshallBody(&response, resp.Body)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
		return
	}

	nextCursor := response.Next

	doneReceiving := false
	for _, asset := range response.Assets {
		if it, ok := alreadyReceived[asset.ID]; ok {
			logger.For(pCtx).Debugf("response already received asset: %s", it)
			doneReceiving = true
			continue
		}
		result = append(result, asset)
		alreadyReceived[asset.ID] = fmt.Sprintf("asset-%d|offset-%s|len-%d|dir-%s", asset.ID, pCursor, len(result), dir)
	}

	assetsChan <- assetsReceieved{assets: result}

	if doneReceiving {
		return
	}

	if len(response.Assets) == 50 {
		FetchAssets(pCtx, assetsChan, pWalletAddress, pContractAddress, pTokenID, nextCursor, 0, alreadyReceived)
		if err != nil {
			assetsChan <- assetsReceieved{err: err}
			return
		}
	}
}

// FetchContractByAddress fetches a contract by address
func FetchContractByAddress(pCtx context.Context, pContract persist.EthereumAddress, retry int) (Contract, error) {

	logger.For(pCtx).Debugf("Fetching contract for address %s", pContract)

	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/asset_contract/%s", pContract)

	req, err := http.NewRequestWithContext(pCtx, "GET", urlStr, nil)
	if err != nil {
		return Contract{}, err
	}
	req.Header.Set("X-API-KEY", viper.GetString("OPENSEA_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Contract{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if retry < 3 {
				time.Sleep(time.Second * 3 * time.Duration(retry+1))
				return FetchContractByAddress(pCtx, pContract, retry+1)
			}
			return Contract{}, fmt.Errorf("opensea api rate limit exceeded")
		}
		bs := new(bytes.Buffer)
		_, err := bs.ReadFrom(resp.Body)
		if err != nil {
			return Contract{}, err
		}
		return Contract{}, fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, bs.String())
	}
	response := Contract{}
	err = util.UnmarshallBody(&response, resp.Body)
	if err != nil {
		return Contract{}, err
	}

	return response, nil
}

func assetsToTokens(ctx context.Context, address persist.Address, assetsChan <-chan assetsReceieved, ethClient *ethclient.Client) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, nil, err
	}
	resultTokens := make([]multichain.ChainAgnosticToken, 0, len(assetsChan))
	seenContracts := &sync.Map{}
	resultContracts := make([]multichain.ChainAgnosticContract, 0, len(assetsChan))
	tokensChan := make(chan multichain.ChainAgnosticToken)
	contractsChan := make(chan multichain.ChainAgnosticContract)
	errChan := make(chan error)
	wp := workerpool.New(10)

	go func() {
		defer close(tokensChan)
		for a := range assetsChan {
			assetsReceived := a
			if assetsReceived.err != nil {
				errChan <- assetsReceived.err
				return
			}
			for _, n := range assetsReceived.assets {
				nft := n
				wp.Submit(func() {
					innerCtx, cancel := context.WithTimeout(ctx, time.Second*10)
					defer cancel()
					var tokenType persist.TokenType
					switch nft.Contract.ContractSchemaName {
					case "ERC721", "CRYPTOPUNKS":
						tokenType = persist.TokenTypeERC721
					case "ERC1155":
						tokenType = persist.TokenTypeERC1155
					default:
						errChan <- fmt.Errorf("unknown token type: %s", nft.Contract.ContractSchemaName)
						return
					}

					metadata := persist.TokenMetadata{
						"name":          nft.Name,
						"description":   nft.Description,
						"image_url":     nft.ImageOriginalURL,
						"animation_url": nft.AnimationOriginalURL,
					}

					med := persist.Media{ThumbnailURL: persist.NullString(firstNonEmptyString(nft.ImageURL, nft.ImagePreviewURL, nft.ImageThumbnailURL))}
					switch {
					case nft.AnimationURL != "":
						med.MediaURL = persist.NullString(nft.AnimationURL)
						med.MediaType, err = media.PredictMediaType(innerCtx, nft.AnimationURL)

					case nft.AnimationOriginalURL != "":
						med.MediaURL = persist.NullString(nft.AnimationOriginalURL)
						med.MediaType, err = media.PredictMediaType(innerCtx, nft.AnimationOriginalURL)

					case nft.ImageURL != "":
						med.MediaURL = persist.NullString(nft.ImageURL)
						med.MediaType, err = media.PredictMediaType(innerCtx, nft.ImageURL)

					case nft.ImageOriginalURL != "":
						med.MediaURL = persist.NullString(nft.ImageOriginalURL)
						med.MediaType, err = media.PredictMediaType(innerCtx, nft.ImageOriginalURL)

					default:
						med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
						med.MediaType, err = media.PredictMediaType(innerCtx, nft.ImageThumbnailURL)
					}

					if err != nil {
						logger.For(ctx).Errorf("failed to predict media type for %s: %s", nft.ImageThumbnailURL, err)
					}

					contract, ok := seenContracts.LoadOrStore(nft.Contract.ContractAddress.String(), multichain.ChainAgnosticContract{
						Address:        persist.Address(nft.Contract.ContractAddress.String()),
						Symbol:         nft.Contract.ContractSymbol.String(),
						Name:           nft.Contract.ContractName.String(),
						CreatorAddress: persist.Address(nft.Creator.Address),
						LatestBlock:    persist.BlockNumber(block),
					})
					if !ok {
						contractsChan <- contract.(multichain.ChainAgnosticContract)
					} else {
						contractsChan <- multichain.ChainAgnosticContract{}
					}

					tokensChan <- multichain.ChainAgnosticToken{
						TokenType:       tokenType,
						Name:            nft.Name,
						Description:     nft.Description,
						TokenURI:        persist.TokenURI(nft.TokenMetadataURL),
						TokenID:         persist.TokenID(nft.TokenID.ToBase16()),
						OwnerAddress:    address,
						ContractAddress: persist.Address(nft.Contract.ContractAddress.String()),
						ExternalURL:     nft.ExternalURL,
						BlockNumber:     persist.BlockNumber(block),
						TokenMetadata:   metadata,
						Media:           med,
						Quantity:        "1",
						IsSpam:          util.BoolToPointer(false), // Tokens returned by OS can be considered legit
					}
				})
			}
		}
		wp.StopWait()
	}()

	for {
		select {
		case token, ok := <-tokensChan:
			if !ok {
				return resultTokens, resultContracts, nil
			}
			resultTokens = append(resultTokens, token)
		case contract := <-contractsChan:
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

func contractToContract(ctx context.Context, openseaContract Contract, ethClient *ethclient.Client) (multichain.ChainAgnosticContract, error) {
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return multichain.ChainAgnosticContract{
		Address:        persist.Address(openseaContract.Address.String()),
		Symbol:         openseaContract.Symbol,
		Name:           openseaContract.Collection.Name,
		CreatorAddress: persist.Address(openseaContract.Collection.PayoutAddress),
		LatestBlock:    persist.BlockNumber(block),
	}, nil
}

// RefreshToken refreshes the metadata for a given token.
func (p *Provider) RefreshToken(context.Context, multichain.ChainAgnosticIdentifiers, persist.Address) error {
	return nil
}

// RefreshContract refreshses the metadata for a contract
func (p *Provider) RefreshContract(context.Context, persist.Address) error {
	return nil
}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (p *Provider) VerifySignature(pCtx context.Context,
	pAddressStr persist.Address, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

	nonce := auth.NewNoncePrepend + pNonce
	// personal_sign
	validBool, err := verifySignature(pSignatureStr,
		nonce,
		pAddressStr.String(), pWalletType,
		true, p.ethClient)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = verifySignature(pSignatureStr,
			nonce,
			pAddressStr.String(), pWalletType,
			false, p.ethClient)
		if err != nil || !validBool {
			nonce = auth.NoncePrepend + pNonce
			validBool, err = verifySignature(pSignatureStr,
				nonce,
				pAddressStr.String(), pWalletType,
				true, p.ethClient)
			if err != nil || !validBool {
				validBool, err = verifySignature(pSignatureStr,
					nonce,
					pAddressStr.String(), pWalletType,
					false, p.ethClient)
			}
		}
	}

	if err != nil {
		return false, err
	}

	return validBool, nil
}

func verifySignature(pSignatureStr string,
	pData string,
	pAddress string, pWalletType persist.WalletType,
	pUseDataHeaderBool bool, ec *ethclient.Client) (bool, error) {

	// eth_sign:
	// - https://goethereumbook.org/signature-verify/
	// - http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// - sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))

	var data string
	if pUseDataHeaderBool {
		data = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(pData), pData)
	} else {
		data = pData
	}

	switch pWalletType {
	case persist.WalletTypeEOA:
		dataHash := crypto.Keccak256Hash([]byte(data))

		sig, err := hexutil.Decode(pSignatureStr)
		if err != nil {
			return false, err
		}
		// Ledger-produced signatures have v = 0 or 1
		if sig[64] == 0 || sig[64] == 1 {
			sig[64] += 27
		}
		v := sig[64]
		if v != 27 && v != 28 {
			return false, errors.New("invalid signature (V is not 27 or 28)")
		}
		sig[64] -= 27

		sigPublicKeyECDSA, err := crypto.SigToPub(dataHash.Bytes(), sig)
		if err != nil {
			return false, err
		}

		pubkeyAddressHexStr := crypto.PubkeyToAddress(*sigPublicKeyECDSA).Hex()
		logger.For(nil).Infof("pubkeyAddressHexStr: %s", pubkeyAddressHexStr)
		logger.For(nil).Infof("pAddress: %s", pAddress)
		if !strings.EqualFold(pubkeyAddressHexStr, pAddress) {
			return false, auth.ErrAddressSignatureMismatch
		}

		publicKeyBytes := crypto.CompressPubkey(sigPublicKeyECDSA)

		signatureNoRecoverID := sig[:len(sig)-1]

		return crypto.VerifySignature(publicKeyBytes, dataHash.Bytes(), signatureNoRecoverID), nil
	case persist.WalletTypeGnosis:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sigValidator, err := contracts.NewISignatureValidator(common.HexToAddress(pAddress), ec)
		if err != nil {
			return false, err
		}

		hashedData := crypto.Keccak256([]byte(data))
		var input [32]byte
		copy(input[:], hashedData)

		result, err := sigValidator.IsValidSignature(&bind.CallOpts{Context: ctx}, input, []byte{})
		if err != nil {
			logger.For(nil).WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}

}

func (e errNoSingleNFTForOpenseaID) Error() string {
	return fmt.Sprintf("no single NFT found for opensea id %d", e.openseaID)
}

func (e ErrNoAssetsForWallets) Error() string {
	return fmt.Sprintf("no assets found for wallet: %v", e.Wallets)
}

func (t TokenID) String() string {
	return string(t)
}

// ToBase16 coverts a base 10 tokenID to base 16
func (t TokenID) ToBase16() string {
	asBig, ok := new(big.Int).SetString(string(t), 10)
	if !ok {
		panic("failed to convert opensea token id to big int")
	}
	return asBig.Text(16)
}

func firstNonEmptyString(strs ...string) string {
	for _, str := range strs {
		if str != "" {
			return str
		}
	}
	return ""
}
