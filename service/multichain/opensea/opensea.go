package opensea

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
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
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/retry"
	"github.com/sirupsen/logrus"
)

func init() {
	env.RegisterValidation("OPENSEA_API_KEY", "required")
}

var baseURL, _ = url.Parse("https://api.opensea.io/api/v1")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

var sharedStoreFrontAddresses = []persist.EthereumAddress{
	"0x09200b963c52d3297a93af71f919e7829c53cf9a",
	"0x37530eef6290a0be43d481e8feb5597c3a05f03e",
	"0x495f947276749ce646f68ac8c248420045cb7b5e",
	"0x4bea754665a141bf4837e743f8ca08507e8afb01",
	"0x5e30b1d6f920364c847512e2528efdadf72a97a9",
	"0x80fa4ab88378853d97e30a9f002b084b1a4efd00",
	"0x8798093380deea4af4007b8e874643c61ea2dae8",
	"0xfda7a5ecd561af6a17506a686ad0a648857dcc14",
}

type Provider struct {
	httpClient *http.Client
	ethClient  *ethclient.Client
	cache      *redis.Cache
}

// TokenID represents a token ID from Opensea. It is separate from persist.TokenID because opensea returns token IDs in base 10 format instead of what we want, base 16
type TokenID string

// Assets is a list of NFTs from OpenSea
type Assets struct {
	Next     string  `json:"next"`
	Previous string  `json:"previous"`
	Assets   []Asset `json:"assets"`
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
	Description           string                  `json:"description"`
	PayoutAddress         persist.EthereumAddress `json:"payout_address"`
	PrimaryAssetContracts []Contract              `json:"primary_asset_contracts"`
	Slug                  string                  `json:"slug"`
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
func NewProvider(ethClient *ethclient.Client, httpClient *http.Client, redisClient *redis.Cache) *Provider {
	return &Provider{
		httpClient: httpClient,
		ethClient:  ethClient,
		cache:      redisClient,
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
		streamAssetsForWallet(ctx, persist.EthereumAddress(address), WithResultCh(assetsChan))
	}()

	return assetsToTokens(ctx, address, assetsChan, p.ethClient)
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, address persist.Address, limit, offset int) ([]multichain.ChainAgnosticToken, multichain.ChainAgnosticContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForContract(ctx, persist.EthereumAddress(address), WithResultCh(assetsChan))
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
		streamAssetsForContractAddressAndOwner(ctx, persist.EthereumAddress(owner), persist.EthereumAddress(address), WithResultCh(assetsChan))
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
		streamAssetsForTokenIdentifiers(ctx, persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID.Base10String()), WithResultCh(assetsChan))
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
		streamAssetsForTokenIdentifiersAndOwner(ctx, persist.EthereumAddress(ownerAddress), persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID.Base10String()), WithResultCh(assetsChan))
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

// GetContractsCreatedOnSharedContract returns a tokens created by the address under the Shared Storefront contract
func (p *Provider) GetContractsCreatedOnSharedContract(ctx context.Context, walletAddress persist.Address) ([]multichain.ChildContract, error) {
	assetsChan := make(chan assetsReceieved)
	go func() {
		defer close(assetsChan)
		streamAssetsForCollectionEditor(ctx, persist.EthereumAddress(walletAddress),
			WithResultCh(assetsChan),
			WithSortAsc(),
			WithCaching(p.cache),
			// WithFromLastCursor(),
		)
	}()
	return assetsToGroups(ctx, assetsChan, p.ethClient)
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

type QueryOptions struct {
	outCh    chan assetsReceieved
	sortAsc  bool
	pageSize int
	cache    *redis.Cache
	useLast  bool
}

type OptionFunc func(o *QueryOptions)

func DefaultArgs() QueryOptions {
	return QueryOptions{
		outCh:    make(chan assetsReceieved),
		sortAsc:  false,
		pageSize: 200,
	}
}

// WithResultCh allows you to specify a channel to receive the assets on
func WithResultCh(outCh chan assetsReceieved) OptionFunc {
	return func(o *QueryOptions) {
		o.outCh = outCh
	}
}

// WithSortAsc will sort the assets in ascending order
func WithSortAsc() OptionFunc {
	return func(o *QueryOptions) {
		o.sortAsc = true
	}
}

// WithCaching will enable cursor caching
func WithCaching(cache *redis.Cache) OptionFunc {
	return func(o *QueryOptions) {
		o.cache = cache
	}
}

// WithFromLastCursor will start the query from the last cursor
func WithFromLastCursor() OptionFunc {
	return func(o *QueryOptions) {
		o.useLast = true
	}
}

func FetchAssetsForWallet(ctx context.Context, address persist.EthereumAddress) ([]Asset, error) {
	args := DefaultArgs()
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setOwner(url, address)

	outCh := make(chan assetsReceieved)

	go func() {
		defer close(args.outCh)
		streamAssetsForWallet(ctx, address, WithResultCh(outCh))
	}()

	assets := make([]Asset, 0)
	for a := range args.outCh {
		if a.err != nil {
			return nil, a.err
		}
		assets = append(assets, a.assets...)
	}

	return assets, nil
}

// FetchCollectionsForAddress returns all collections that `address` has at least one token for
func FetchCollectionsForAddress(ctx context.Context, address persist.EthereumAddress) ([]Collection, error) {
	url := baseURL.JoinPath("collections")
	setAssetOwner(url, address)

	req := authRequest(ctx, url.String())
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

	req := authRequest(pCtx, url.String())

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

func streamAssetsForWallet(ctx context.Context, address persist.EthereumAddress, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setOwner(url, address)
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

func streamAssetsForContract(ctx context.Context, address persist.EthereumAddress, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setContractAddress(url, address)
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

func streamAssetsForContractAddressAndOwner(ctx context.Context, ownerAddress, contractAddress persist.EthereumAddress, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setOwner(url, ownerAddress)
	setContractAddress(url, contractAddress)
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

func streamAssetsForTokenIdentifiers(ctx context.Context, contractAddress persist.EthereumAddress, tokenID TokenID, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setContractAddress(url, contractAddress)
	setTokenID(url, tokenID)
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

func streamAssetsForTokenIdentifiersAndOwner(ctx context.Context, ownerAddress, contractAddress persist.EthereumAddress, tokenID TokenID, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setContractAddress(url, contractAddress)
	setTokenID(url, tokenID)
	if ownerAddress != "" {
		setOwner(url, ownerAddress)
	}
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

func streamAssetsForCollectionEditor(ctx context.Context, editorAddress persist.EthereumAddress, opts ...OptionFunc) {
	args := DefaultArgs()
	for _, opt := range opts {
		opt(&args)
	}
	url := baseURL.JoinPath("assets")
	setPagingParams(url, args.sortAsc, args.pageSize)
	setCollectionEditor(url, editorAddress)
	// Filter for only the Shared Storefront Address
	for _, address := range sharedStoreFrontAddresses {
		addContractAddress(url, address)
	}
	// setCursor should be called last when all the query params have been added
	setCursor(ctx, url, args.cache)
	paginateAssets(ctx, authRequest(ctx, url.String()), args.sortAsc, args.outCh, args.cache)
}

// assetsToGroups converts a channel of assets to a slice of sub-contract groups
func assetsToGroups(ctx context.Context, assetsChan <-chan assetsReceieved, ethClient *ethclient.Client) ([]multichain.ChildContract, error) {
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	childContracts := make(map[string]multichain.ChildContract)
	parentContracts := make(map[string]multichain.ChainAgnosticContract)

	for page := range assetsChan {
		if page.err != nil {
			return nil, err
		}
		for _, asset := range page.assets {
			contractAsStr := asset.Contract.ContractAddress.String()

			// Add the parent contract if we haven't seen it before
			if _, seen := parentContracts[contractAsStr]; !seen {
				parentContracts[contractAsStr] = contractFromAsset(asset, persist.BlockNumber(block))
			}

			childID := asset.Collection.Slug

			// Found a new child
			if _, seen := childContracts[childID]; !seen {
				childContracts[childID] = multichain.ChildContract{
					ChildID:        childID,
					Name:           asset.Collection.Name,
					Description:    asset.Collection.Description,
					ParentContract: parentContracts[contractAsStr],
					Tokens:         make([]multichain.ChainAgnosticToken, 0),
				}
			}

			token, err := assetToToken(asset, persist.BlockNumber(block), persist.Address(asset.Owner.Address))
			var unknownSchemaErr unknownContractSchemaError

			if errors.As(err, &unknownSchemaErr) {
				logger.For(ctx).Error(err)
				continue
			}

			if err != nil {
				return nil, err
			}

			// Work-around because OS doesn't return the owner address for some reason
			// All OS shared contracts are ERC1155 so this would involve find all owners
			// that have a balance and creating a token with the correct balance per user
			if token.OwnerAddress == "" {
				var ownerAddress persist.EthereumAddress
				var err error

				switch token.TokenType {
				case persist.TokenTypeERC721:
					ownerAddress, err = rpc.RetryGetOwnerOfERC721Token(ctx, persist.EthereumAddress(token.ContractAddress), token.TokenID, ethClient)
				case persist.TokenTypeERC1155:
					panic("not implemented")
				}

				if err != nil {
					logger.For(ctx).Error(err)
				}

				token.OwnerAddress = persist.Address(ownerAddress)
			}

			if token.OwnerAddress == "" {
				continue
			}

			// Add the token
			childContract := childContracts[childID]
			childContract.Tokens = append(childContract.Tokens, token)
			childContracts[childID] = childContract
		}
	}

	// Convert the map to a slice
	children := make([]multichain.ChildContract, 0)
	for _, child := range childContracts {
		children = append(children, child)
	}

	return children, nil
}

type unknownContractSchemaError struct {
	schema string
}

func (e unknownContractSchemaError) Error() string {
	return fmt.Sprintf("unknown token type: %s", e.schema)
}

func tokenTypeFromAsset(asset Asset) (persist.TokenType, error) {
	switch asset.Contract.ContractSchemaName {
	case "ERC721", "CRYPTOPUNKS":
		return persist.TokenTypeERC721, nil
	case "ERC1155":
		return persist.TokenTypeERC1155, nil
	default:
		return "", unknownContractSchemaError{asset.Contract.ContractSchemaName.String()}
	}
}

func assetToToken(asset Asset, block persist.BlockNumber, tokenOwner persist.Address) (multichain.ChainAgnosticToken, error) {
	tokenType, err := tokenTypeFromAsset(asset)
	if err != nil {
		return multichain.ChainAgnosticToken{}, err
	}
	return multichain.ChainAgnosticToken{
		TokenType:       tokenType,
		Name:            asset.Name,
		Description:     asset.Description,
		TokenURI:        persist.TokenURI(asset.TokenMetadataURL),
		TokenID:         persist.TokenID(asset.TokenID.ToBase16()),
		OwnerAddress:    tokenOwner,
		ContractAddress: persist.Address(asset.Contract.ContractAddress.String()),
		ExternalURL:     asset.ExternalURL,
		BlockNumber:     block,
		TokenMetadata: persist.TokenMetadata{
			"name":          asset.Name,
			"description":   asset.Description,
			"image_url":     asset.ImageOriginalURL,
			"animation_url": asset.AnimationOriginalURL,
		},
		Quantity: "1",
		IsSpam:   util.ToPointer(false), // OpenSea filters spam on their side
	}, nil
}

func contractFromAsset(asset Asset, block persist.BlockNumber) multichain.ChainAgnosticContract {
	return multichain.ChainAgnosticContract{
		Address:        persist.Address(asset.Contract.ContractAddress.String()),
		Symbol:         asset.Contract.ContractSymbol.String(),
		Name:           asset.Contract.ContractName.String(),
		CreatorAddress: persist.Address(asset.Creator.Address),
		LatestBlock:    block,
	}
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
					contract, ok := seenContracts.LoadOrStore(nft.Contract.ContractAddress.String(), contractFromAsset(nft, persist.BlockNumber(block)))
					if !ok {
						contractsChan <- contract.(multichain.ChainAgnosticContract)
					} else {
						contractsChan <- multichain.ChainAgnosticContract{}
					}

					tokenOwner := ownerAddress
					if tokenOwner == "" {
						tokenOwner = persist.Address(nft.Owner.Address)
					}

					token, err := assetToToken(nft, persist.BlockNumber(block), tokenOwner)

					var unknownSchemaErr unknownContractSchemaError

					if errors.As(err, &unknownSchemaErr) {
						logger.For(ctx).Error(err)
						return
					}

					if err != nil {
						errChan <- err
						return
					}

					tokensChan <- token
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
func authRequest(ctx context.Context, url string) *http.Request {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("X-API-KEY", env.GetString("OPENSEA_API_KEY"))
	return req
}

func paginateAssets(ctx context.Context, req *http.Request, sortAsc bool, outCh chan assetsReceieved, cache *redis.Cache) {
	var lastCursor string
	for {
		resp, err := retry.RetryRequest(http.DefaultClient, req)
		if err != nil {
			outCh <- assetsReceieved{err: err}
			return
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			outCh <- assetsReceieved{err: util.BodyAsError(resp)}
			return
		}

		assets := Assets{}
		if err := util.UnmarshallBody(&assets, resp.Body); err != nil {
			outCh <- assetsReceieved{err: err}
			return
		}

		outCh <- assetsReceieved{assets: assets.Assets}

		cursor := assets.Next
		if sortAsc {
			cursor = assets.Previous
		}

		// No more pages to paginate
		if cursor == "" {
			if cache != nil && lastCursor != "" {
				storeCursor(ctx, req.URL, cache, lastCursor)
			}
			return
		}

		lastCursor = cursor
		query := req.URL.Query()
		query.Set("cursor", cursor)
		req.URL.RawQuery = query.Encode()
	}
}

func setPagingParams(url *url.URL, sortAsc bool, pageSize int) {
	sortOrder := "asc"
	if !sortAsc {
		sortOrder = "desc"
	}
	query := url.Query()
	query.Set("order_direction", sortOrder)
	query.Set("limit", strconv.Itoa(pageSize))
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

func setCollectionEditor(url *url.URL, address persist.EthereumAddress) {
	query := url.Query()
	query.Add("collection_editor", address.String())
	url.RawQuery = query.Encode()
}

func setTokenID(url *url.URL, tokenID TokenID) {
	query := url.Query()
	query.Add("token_ids", string(tokenID))
	url.RawQuery = query.Encode()
}

func setCursor(ctx context.Context, url *url.URL, cache *redis.Cache) {
	key := fmt.Sprintf("provider:opensea:cursor:%s", url.String())
	byt, err := cache.Get(ctx, key)

	var notFoundErr redis.ErrKeyNotFound
	if errors.As(err, &notFoundErr) {
		return
	}

	query := url.Query()
	query.Set("cursor", string(byt))
	url.RawQuery = query.Encode()
}

func storeCursor(ctx context.Context, url *url.URL, cache *redis.Cache, cursor string) error {
	// Remove cursor from the url because the url is used as the cache key
	urlCopy := *url
	query := urlCopy.Query()
	query.Del("cursor")
	urlCopy.RawQuery = query.Encode()
	key := fmt.Sprintf("provider:opensea:cursor:%s", urlCopy.String())
	return cache.Set(ctx, key, []byte(cursor), 0)
}
