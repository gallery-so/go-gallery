package opensea

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sirupsen/logrus"
)

func init() {
	env.RegisterValidation("OPENSEA_API_KEY", []string{"required"})
}

var baseURL, _ = url.Parse("https://api.opensea.io/api/v1")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

type Provider struct {
	httpClient *http.Client
	ethClient  *ethclient.Client
}

// TokenID represents a token ID from Opensea. It is separate from persist.TokenID because opensea returns token IDs in base 10 format instead of what we want, base 16
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
	Name                  string                  `json:"name"`
	PayoutAddress         persist.EthereumAddress `json:"payout_address"`
	PrimaryAssetContracts []Contract              `json:"primary_asset_contracts"`
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
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForWallet(ctx, assetsChan, persist.EthereumAddress(address))
	}()

	return assetsToTokens(ctx, address, assetsChan, p.ethClient)
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForContract(ctx, assetsChan, persist.EthereumAddress(address))
	}()
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

// GetTokensByContractAddressAndOwner returns a list of tokens for a contract address and owner
func (p *Provider) GetTokensByContractAddressAndOwner(ctx context.Context, owner, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForContractAddressAndOwner(ctx, assetsChan, persist.EthereumAddress(owner), persist.EthereumAddress(address))
	}()
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
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForTokenIdentifiers(ctx, assetsChan, persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID.Base10String()))
	}()
	tokens, contracts, err := assetsToTokens(ctx, "", assetsChan, p.ethClient)
	if err != nil {
		return nil, multichain.ChainAgnosticContract{}, err
	}
	contract := multichain.ChainAgnosticContract{}
	if len(contracts) > 0 {
		contract = contracts[0]
	}
	return tokens, contract, nil
}

func (p *Provider) GetTokensByTokenIdentifiersAndOwner(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForTokenIdentifiersAndOwner(ctx, assetsChan, persist.EthereumAddress(ownerAddress), persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID.Base10String()))
	}()
	tokens, contracts, err := assetsToTokens(ctx, "", assetsChan, p.ethClient)
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
func (p *Provider) GetTokenMetadataByTokenIdentifiers(ctx context.Context, ti multichain.ChainAgnosticIdentifiers, ownerAddress persist.Address) (persist.TokenMetadata, error) {
	token, _, err := p.GetTokensByTokenIdentifiersAndOwner(ctx, ti, ownerAddress)
	if err != nil {
		return nil, err
	}
	return token.TokenMetadata, nil
}

// GetContractByAddress returns a contract for a contract address
func (p *Provider) GetContractByAddress(ctx context.Context, contract persist.Address) (multichain.ChainAgnosticContract, error) {
	c, err := FetchContractByAddress(ctx, persist.EthereumAddress(contract))
	if err != nil {
		return multichain.ChainAgnosticContract{}, err
	}
	return contractToContract(ctx, c, p.ethClient)
}
func (d *Provider) GetCommunityOwners(ctx context.Context, communityID persist.Address, limit, offset int) ([]multichain.ChainAgnosticCommunityOwner, error) {
	return []multichain.ChainAgnosticCommunityOwner{}, nil
}

func (d *Provider) GetOwnedTokensByContract(context.Context, persist.Address, persist.Address, int, int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	return []multichain.ChainAgnosticToken{}, multichain.ChainAgnosticContract{}, nil
}

func (d *Provider) GetDisplayNameByAddress(ctx context.Context, addr persist.Address) string {
	return addr.String()
}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (p *Provider) VerifySignature(pCtx context.Context,
	pAddressStr persist.PubKey, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

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
		log.Println("pubkeyAddressHexStr:", pubkeyAddressHexStr)
		log.Println("pAddress:", pAddress)
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
			logrus.WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}
}

func FetchAssetsForWallet(ctx context.Context, address persist.EthereumAddress) ([]Asset, error) {
	url := baseURL.JoinPath("assets")
	setPagingParams(url)
	setOwner(url, address)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	return paginateAssets(req)
}

func FetchAssetsForContract(ctx context.Context, address persist.EthereumAddress) ([]Asset, error) {
	url := baseURL.JoinPath("assets")
	setPagingParams(url)
	setContractAddress(url, address)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	return paginateAssets(req)
}

func FetchAssetsForContractAddressAndOwner(ctx context.Context, ownerAddress, contractAddress persist.EthereumAddress) ([]Asset, error) {
	url := baseURL.JoinPath("assets")
	setPagingParams(url)
	setOwner(url, ownerAddress)
	setContractAddress(url, contractAddress)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	return paginateAssets(req)
}

func FetchAssetsForTokenIdentifiers(ctx context.Context, contractAddress persist.EthereumAddress, tokenID TokenID) ([]Asset, error) {
	url := baseURL.JoinPath("assets")
	setPagingParams(url)
	setContractAddress(url, contractAddress)
	setTokenID(url, tokenID)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	return paginateAssets(req)
}

func FetchAssetsForTokenIdentifiersAndOwner(ctx context.Context, ownerAddress, contractAddress persist.EthereumAddress, tokenID TokenID) ([]Asset, error) {
	url := baseURL.JoinPath("assets")
	setPagingParams(url)
	setOwner(url, ownerAddress)
	setContractAddress(url, contractAddress)
	setTokenID(url, tokenID)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	return paginateAssets(req)
}

// FetchCollectionsForAddress returns all collections that `address` has at least one token for
func FetchCollectionsForAddress(ctx context.Context, address persist.EthereumAddress) ([]Collection, error) {
	url := baseURL.JoinPath("collections")
	setAssetOwner(url, address)

	req, err := authRequest(ctx, url.String())
	if err != nil {
		return nil, err
	}

	resp, err := retry.RetryRequest(http.DefaultClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, util.BodyAsError(resp)
	}

	collections := []Collection{}
	err = util.UnmarshallBody(&collections, resp.Body)
	if err != nil {
		return nil, err
	}

	return collections, nil
}

// FetchContractByAddress fetches a contract by address
func FetchContractByAddress(pCtx context.Context, pContract persist.EthereumAddress) (Contract, error) {
	url := baseURL.JoinPath("asset_contract")

	req, err := authRequest(pCtx, url.String())
	if err != nil {
		return Contract{}, err
	}

	resp, err := retry.RetryRequest(http.DefaultClient, req)
	if err != nil {
		return Contract{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Contract{}, util.BodyAsError(resp)
	}

	contract := Contract{}
	err = util.UnmarshallBody(&contract, resp.Body)
	if err != nil {
		return Contract{}, err
	}

	return contract, nil
}

func streamAssetsForWallet(ctx context.Context, assetsChan chan<- assetsReceieved, address persist.EthereumAddress) {
	assets, err := FetchAssetsForWallet(ctx, address)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
	}
	assetsChan <- assetsReceieved{assets: assets}
}

func streamAssetsForContract(ctx context.Context, assetsChan chan<- assetsReceieved, address persist.EthereumAddress) {
	assets, err := FetchAssetsForContract(ctx, address)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
	}
	assetsChan <- assetsReceieved{assets: assets}
}

func streamAssetsForContractAddressAndOwner(ctx context.Context, assetsChan chan<- assetsReceieved, ownerAddress, contractAddress persist.EthereumAddress) {
	assets, err := FetchAssetsForContractAddressAndOwner(ctx, ownerAddress, contractAddress)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
	}
	assetsChan <- assetsReceieved{assets: assets}
}

func streamAssetsForTokenIdentifiers(ctx context.Context, assetsChan chan<- assetsReceieved, contractAddress persist.EthereumAddress, tokenID TokenID) {
	assets, err := FetchAssetsForTokenIdentifiers(ctx, contractAddress, tokenID)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
	}
	assetsChan <- assetsReceieved{assets: assets}
}

func streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, assetsChan chan<- assetsReceieved, ownerAddress, contractAddress persist.EthereumAddress, tokenID TokenID) {
	assets, err := FetchAssetsForTokenIdentifiersAndOwner(ctx, ownerAddress, contractAddress, tokenID)
	if err != nil {
		assetsChan <- assetsReceieved{err: err}
	}
	assetsChan <- assetsReceieved{assets: assets}
}

func assetsToTokens(ctx context.Context, ownerAddress persist.Address, assetsChan <-chan assetsReceieved, ethClient *ethclient.Client) ([]multichain.ChainAgnosticToken, []multichain.ChainAgnosticContract, error) {

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
						logger.For(ctx).Warnf("unknown token type: %s", nft.Contract.ContractSchemaName)
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
						med.MediaType, _, _, err = media.PredictMediaType(innerCtx, nft.AnimationURL)

					case nft.AnimationOriginalURL != "":
						med.MediaURL = persist.NullString(nft.AnimationOriginalURL)
						med.MediaType, _, _, err = media.PredictMediaType(innerCtx, nft.AnimationOriginalURL)

					case nft.ImageURL != "":
						med.MediaURL = persist.NullString(nft.ImageURL)
						med.MediaType, _, _, err = media.PredictMediaType(innerCtx, nft.ImageURL)

					case nft.ImageOriginalURL != "":
						med.MediaURL = persist.NullString(nft.ImageOriginalURL)
						med.MediaType, _, _, err = media.PredictMediaType(innerCtx, nft.ImageOriginalURL)

					default:
						med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
						med.MediaType, _, _, err = media.PredictMediaType(innerCtx, nft.ImageThumbnailURL)
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

					tokenOwner := ownerAddress
					if tokenOwner == "" {
						tokenOwner = persist.Address(nft.Owner.Address)
					}

					tokensChan <- multichain.ChainAgnosticToken{
						TokenType:       tokenType,
						Name:            nft.Name,
						Description:     nft.Description,
						TokenURI:        persist.TokenURI(nft.TokenMetadataURL),
						TokenID:         persist.TokenID(nft.TokenID.ToBase16()),
						OwnerAddress:    tokenOwner,
						ContractAddress: persist.Address(nft.Contract.ContractAddress.String()),
						ExternalURL:     nft.ExternalURL,
						BlockNumber:     persist.BlockNumber(block),
						TokenMetadata:   metadata,
						Media:           med,
						Quantity:        "1",
						IsSpam:          util.ToPointer(false), // OpenSea filters spam on their side
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

// authRequest returns a http.Request with authorization headers
func authRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", env.GetString(ctx, "OPENSEA_API_KEY"))
	return req, nil
}

func paginateAssets(req *http.Request) ([]Asset, error) {
	result := make([]Asset, 0)
	for {
		resp, err := retry.RetryRequest(http.DefaultClient, req)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, util.BodyAsError(resp)
		}

		assets := Assets{}

		err = util.UnmarshallBody(&assets, resp.Body)
		if err != nil {
			return nil, err
		}

		for _, asset := range assets.Assets {
			result = append(result, asset)
		}

		// No more pages to paginate
		if assets.Next == "" {
			return result, nil
		}

		query := req.URL.Query()
		query.Set("cursor", assets.Next)
		req.URL.RawQuery = query.Encode()
	}
}

func setPagingParams(url *url.URL) {
	query := url.Query()
	query.Set("order_direction", "desc")
	query.Set("limit", "50")
	url.RawQuery = query.Encode()
}

func setOwner(url *url.URL, address persist.EthereumAddress) {
	query := url.Query()
	query.Set("owner", address.String())
	url.RawQuery = query.Encode()
}

func setAssetOwner(url *url.URL, address persist.EthereumAddress) {
	query := url.Query()
	query.Set("asset_owner", address.String())
	url.RawQuery = query.Encode()
}

func setContractAddress(url *url.URL, address persist.EthereumAddress) {
	query := url.Query()
	query.Set("asset_contract_address", address.String())
	url.RawQuery = query.Encode()
}

func addContractAddress(url *url.URL, address persist.EthereumAddress) {
	query := url.Query()
	query.Add("asset_contract_addresses", address.String())
	url.RawQuery = query.Encode()
}

func setTokenID(url *url.URL, tokenID TokenID) {
	query := url.Query()
	query.Add("token_ids", string(tokenID))
	url.RawQuery = query.Encode()
}
