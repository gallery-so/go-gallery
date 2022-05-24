package opensea

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
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
	TokenMetadataURL string              `json:"token_metadata_url"`
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
func (p *Provider) GetTokensByWalletAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, error) {
	assets, err := FetchAssetsForWallet(ctx, persist.EthereumAddress(address.String()), "", 0, nil)
	if err != nil {
		return nil, err
	}
	return assetsToTokens(ctx, address, assets, p.ethClient)
}

// GetTokensByContractAddress returns a list of tokens for a contract address
func (p *Provider) GetTokensByContractAddress(ctx context.Context, address persist.Address) ([]multichain.ChainAgnosticToken, error) {
	assets, err := FetchAssets(ctx, "", persist.EthereumAddress(address), "", "", 0, nil)
	if err != nil {
		return nil, err
	}
	// TODO: Fill in this address or change something else
	return assetsToTokens(ctx, "", assets, p.ethClient)
}

// GetTokensByTokenIdentifiers returns a list of tokens for a list of token identifiers
func (p *Provider) GetTokensByTokenIdentifiers(ctx context.Context, ti persist.TokenIdentifiers) ([]multichain.ChainAgnosticToken, error) {
	assets, err := FetchAssets(ctx, "", persist.EthereumAddress(ti.ContractAddress), TokenID(ti.TokenID), "", 0, nil)
	if err != nil {
		return nil, err
	}
	// TODO: Fill in this address or change something else
	return assetsToTokens(ctx, "", assets, p.ethClient)
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

// UpdateAssetsForAcc is a pipeline for getting assets for an account
func UpdateAssetsForAcc(pCtx context.Context, pUserID persist.DBID, pOwnerWalletAddresses []persist.EthereumAddress,
	nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository, backupRepo persist.BackupRepository) error {

	err := galleryRepo.RefreshCache(pCtx, pUserID)
	if err != nil {
		return err
	}

	galleries, err := galleryRepo.GetByUserID(pCtx, pUserID)
	if err != nil {
		return err
	}

	for _, gallery := range galleries {
		err = backupRepo.Insert(pCtx, gallery)
		if err != nil {
			return err
		}
	}

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return fmt.Errorf("failed to get user by id %s: %w", pUserID, err)
	}
	if len(pOwnerWalletAddresses) == 0 {
		pOwnerWalletAddresses = persist.WalletsToEthereumAddresses(user.Wallets)
	}

	ids, err := UpdateAssetsForWallet(pCtx, pOwnerWalletAddresses, nftRepo)
	if err != nil {
		if e, ok := err.(ErrNoAssetsForWallets); ok {
			logrus.Debugf("no assets found for wallets %v", e.Wallets)
		} else {
			return err
		}
	}

	// ensure NFTs that a user used to own are no longer in their gallery
	if err := collRepo.ClaimNFTs(pCtx, pUserID, pOwnerWalletAddresses, persist.CollectionUpdateNftsInput{NFTs: ids}); err != nil {
		return fmt.Errorf("failed to claim NFTs: %w", err)
	}
	if err := collRepo.RemoveNFTsOfOldAddresses(pCtx, pUserID); err != nil {
		return fmt.Errorf("failed to remove NFTs of old addresses: %w", err)
	}

	return nil
}

// UpdateAssetsForWallet is a pipeline for getting assets for a wallet
func UpdateAssetsForWallet(pCtx context.Context, pOwnerWalletAddresses []persist.EthereumAddress, nftRepo persist.NFTRepository) ([]persist.DBID, error) {
	var returnErr error
	asDBNfts, err := fetchAssetsForWallets(pCtx, pOwnerWalletAddresses, nftRepo)
	if err != nil {
		if e, ok := err.(ErrNoAssetsForWallets); ok {
			returnErr = e
		} else {
			return nil, fmt.Errorf("failed to fetch assets for wallets %v: %w", pOwnerWalletAddresses, err)
		}
	}
	logrus.Debugf("found %d assets for wallets %v", len(asDBNfts), pOwnerWalletAddresses)

	ids, err := nftRepo.BulkUpsert(pCtx, asDBNfts)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk upsert NFTs: %w", err)
	}
	logrus.Debugf("bulk upserted %d NFTs", len(ids))
	return ids, returnErr
}

func fetchAssetsForWallets(pCtx context.Context, pWalletAddresses []persist.EthereumAddress, nftRepo persist.NFTRepository) ([]persist.NFT, error) {
	result := []persist.NFT{}
	nftsChan := make(chan []persist.NFT)
	errChan := make(chan error)
	for _, walletAddress := range pWalletAddresses {
		go func(wa persist.EthereumAddress) {
			assets, err := FetchAssetsForWallet(pCtx, wa, "", 0, nil)
			if err != nil {
				errChan <- fmt.Errorf("failed to fetch assets for wallet %s: %s", wa, err)
				return
			}
			if len(assets) == 0 {
				time.Sleep(time.Second)
				assets, err = FetchAssetsForWallet(pCtx, wa, "", 0, nil)
				if err != nil {
					errChan <- fmt.Errorf("failed to fetch assets for wallet %s: %s", wa, err)
					return
				}
				if len(assets) == 0 {
					errChan <- ErrNoAssetsForWallets{Wallets: []persist.EthereumAddress{wa}}
					return
				}
			}
			asGlry, err := assetsToNFTs(pCtx, wa, assets, nftRepo)
			if err != nil {
				errChan <- fmt.Errorf("failed to convert opensea assets to db nfts: %s", err)
				return
			}
			nftsChan <- asGlry
		}(walletAddress)
	}

	var notFoundError error
	for i := 0; i < len(pWalletAddresses); i++ {
		select {
		case nfts := <-nftsChan:
			result = append(result, nfts...)
		case err := <-errChan:
			if e, ok := err.(ErrNoAssetsForWallets); ok {
				if notFoundError == nil {
					notFoundError = e
				} else {
					it := notFoundError.(ErrNoAssetsForWallets)
					it.Wallets = append(it.Wallets, e.Wallets...)
					notFoundError = it
				}
			} else {
				return nil, err
			}
		}
	}

	return result, notFoundError
}

// FetchAssetsForWallet recursively fetches all assets for a wallet
func FetchAssetsForWallet(pCtx context.Context, pWalletAddress persist.EthereumAddress, pCursor string, retry int, alreadyReceived map[int]string) ([]Asset, error) {

	if alreadyReceived == nil {
		alreadyReceived = make(map[int]string)
	}

	result := []Asset{}

	dir := "desc"

	logrus.Debugf("Fetching assets for wallet %s with cursor %s, retry %d, dir %s,and alreadyReceived %d", pWalletAddress, pCursor, retry, dir, len(alreadyReceived))

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
			logrus.Debugf("response already received asset: %s", it)
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
func FetchAssets(pCtx context.Context, pWalletAddress, pContractAddress persist.EthereumAddress, pTokenID TokenID, pCursor string, retry int, alreadyReceived map[int]string) ([]Asset, error) {

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
		return nil, fmt.Errorf("failed to create request for url: %s - %s", urlStr, err)
	}
	req.Header.Set("X-API-KEY", viper.GetString("OPENSEA_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch assets for wallet %s: %s - url %s", pWalletAddress, err, urlStr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if retry < 3 {
				time.Sleep(time.Second * 3 * time.Duration(retry+1))
				return FetchAssets(pCtx, pWalletAddress, pContractAddress, pTokenID, pCursor, retry+1, alreadyReceived)
			}
			return nil, fmt.Errorf("opensea api rate limit exceeded - url %s", urlStr)
		}
		bs := new(bytes.Buffer)
		_, err := bs.ReadFrom(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected status code for url %s: %d - %s", urlStr, resp.StatusCode, bs.String())
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
			logrus.Debugf("response already received asset: %s", it)
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
		next, err := FetchAssets(pCtx, pWalletAddress, pContractAddress, pTokenID, nextCursor, 0, alreadyReceived)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

// FetchContractByAddress fetches a contract by address
func FetchContractByAddress(pCtx context.Context, pContract persist.EthereumAddress, retry int) (Contract, error) {

	logrus.Debugf("Fetching contract for address %s", pContract)

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

func assetsToTokens(ctx context.Context, address persist.Address, openseaNfts []Asset, ethClient *ethclient.Client) ([]multichain.ChainAgnosticToken, error) {
	block, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]multichain.ChainAgnosticToken, len(openseaNfts))
	for i, nft := range openseaNfts {
		var tokenType persist.TokenType
		switch nft.Contract.ContractSchemaName {
		case "ERC721", "CRYPTOPUNKS":
			tokenType = persist.TokenTypeERC721
		case "ERC1155":
			tokenType = persist.TokenTypeERC1155
		default:
			return nil, fmt.Errorf("unknown token type: %s", nft.Contract.ContractSchemaName)
		}

		metadata := persist.TokenMetadata{
			"name":          nft.Name,
			"description":   nft.Description,
			"image_url":     nft.ImageOriginalURL,
			"animation_url": nft.AnimationOriginalURL,
		}

		med := persist.Media{ThumbnailURL: persist.NullString(nft.ImageThumbnailURL)}
		switch {
		case nft.AnimationURL != "":
			med.MediaURL = persist.NullString(nft.AnimationURL)
			med.MediaType = media.PredictMediaType(ctx, nft.AnimationURL)
		case nft.ImageURL != "":
			med.MediaURL = persist.NullString(nft.ImageURL)
			med.MediaType = media.PredictMediaType(ctx, nft.ImageURL)
		default:
			med.MediaURL = persist.NullString(nft.ImageThumbnailURL)
			med.MediaType = media.PredictMediaType(ctx, nft.ImageThumbnailURL)
		}

		token := multichain.ChainAgnosticToken{
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
		}
		result[i] = token
	}
	return result, nil
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

func assetsToNFTs(pCtx context.Context, pWalletAddress persist.EthereumAddress, openseaNfts []Asset, nftRepo persist.NFTRepository) ([]persist.NFT, error) {

	nfts := make([]persist.NFT, len(openseaNfts))
	nftChan := make(chan persist.NFT)
	for _, openseaNft := range openseaNfts {
		go func(nft Asset) {
			nftChan <- openseaToDBNft(pCtx, pWalletAddress, nft, nftRepo)
		}(openseaNft)
	}
	for i := 0; i < len(openseaNfts); i++ {
		nfts[i] = <-nftChan
	}
	return nfts, nil
}

func openseaToDBNft(pCtx context.Context, pWalletAddress persist.EthereumAddress, nft Asset, nftRepo persist.NFTRepository) persist.NFT {

	result := persist.NFT{
		OwnerAddress:         pWalletAddress,
		MultipleOwners:       nft.Owner.Address == persist.ZeroAddress,
		Name:                 persist.NullString(nft.Name),
		Description:          persist.NullString(nft.Description),
		ExternalURL:          persist.NullString(nft.ExternalURL),
		ImageURL:             persist.NullString(nft.ImageURL),
		CreatorAddress:       nft.Creator.Address,
		AnimationURL:         persist.NullString(nft.AnimationURL),
		OpenseaTokenID:       persist.TokenID(nft.TokenID.ToBase16()),
		OpenseaID:            persist.NullInt64(nft.ID),
		TokenCollectionName:  persist.NullString(nft.Collection.Name),
		ImageThumbnailURL:    persist.NullString(nft.ImageThumbnailURL),
		ImagePreviewURL:      persist.NullString(nft.ImagePreviewURL),
		ImageOriginalURL:     persist.NullString(nft.ImageOriginalURL),
		TokenMetadataURL:     persist.NullString(nft.TokenMetadataURL),
		Contract:             nft.Contract,
		AcquisitionDateStr:   persist.NullString(nft.AcquisitionDateStr),
		CreatorName:          persist.NullString(nft.Creator.User.Username),
		AnimationOriginalURL: persist.NullString(nft.AnimationOriginalURL),
	}

	dbNFT, _ := nftRepo.GetByOpenseaID(pCtx, persist.NullInt64(nft.ID), pWalletAddress)
	if dbNFT.ID != "" {
		result.ID = dbNFT.ID
	}

	return result

}

// VerifySignature will verify a signature using all available methods (eth_sign and personal_sign)
func (d *Provider) VerifySignature(pCtx context.Context,
	pAddressStr persist.Address, pWalletType persist.WalletType, pNonce string, pSignatureStr string) (bool, error) {

	nonce := auth.NewNoncePrepend + pNonce
	// personal_sign
	validBool, err := verifySignature(pSignatureStr,
		nonce,
		pAddressStr.String(), pWalletType,
		true, d.ethClient)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = verifySignature(pSignatureStr,
			nonce,
			pAddressStr.String(), pWalletType,
			false, d.ethClient)
		if err != nil || !validBool {
			nonce = auth.NoncePrepend + pNonce
			validBool, err = verifySignature(pSignatureStr,
				nonce,
				pAddressStr.String(), pWalletType,
				true, d.ethClient)
			if err != nil || !validBool {
				validBool, err = verifySignature(pSignatureStr,
					nonce,
					pAddressStr.String(), pWalletType,
					false, d.ethClient)
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
