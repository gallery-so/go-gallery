package opensea

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

// Assets is a list of NFTs from OpenSea
type Assets struct {
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
	TokenID persist.TokenID `json:"token_id"`

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
	User    User            `json:"user"`
	Address persist.Address `json:"address"`
}

// User is a user from OpenSea
type User struct {
	Username string `json:"username"`
}

// Collection is a collection from OpenSea
type Collection struct {
	Name string `json:"name"`
}

// Order is an order from OpenSea representing an NFT's maker and buyer
type Order struct {
	Maker Account `json:"maker"`
	Taker Account `json:"taker"`
}

type errNoNFTForTokenIdentifiers struct {
	tokenID         persist.TokenID
	contractAddress persist.Address
}

type errNoSingleNFTForOpenseaID struct {
	openseaID int
}

// UpdateAssetsForAcc is a pipeline for getting assets for an account
func UpdateAssetsForAcc(pCtx context.Context, pUserID persist.DBID, pOwnerWalletAddresses []persist.Address,
	nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, historyRepo persist.OwnershipHistoryRepository) error {

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return fmt.Errorf("failed to get user by id %s: %w", pUserID, err)
	}
	if len(pOwnerWalletAddresses) == 0 {
		pOwnerWalletAddresses = user.Addresses
	}

	asDBNfts, err := fetchAssetsForWallets(pCtx, pOwnerWalletAddresses, nftRepo)
	if err != nil {
		return fmt.Errorf("failed to fetch assets for user %s: %w", pUserID, err)
	}

	ids, err := nftRepo.BulkUpsert(pCtx, pUserID, asDBNfts)
	if err != nil {
		return fmt.Errorf("failed to bulk upsert NFTs: %w", err)
	}

	// update other user's collections and this user's collection so that they and ONLY they can display these
	// specific NFTs while also ensuring that NFTs they don't own don't list them as the owner
	if err := collRepo.ClaimNFTs(pCtx, pUserID, pOwnerWalletAddresses, persist.CollectionUpdateNftsInput{NFTs: ids}); err != nil {
		return fmt.Errorf("failed to claim NFTs: %w", err)
	}

	return nil
}

func fetchAssetsForWallets(pCtx context.Context, pWalletAddresses []persist.Address, nftRepo persist.NFTRepository) ([]persist.NFT, error) {
	result := []persist.NFT{}
	nftsChan := make(chan []persist.NFT)
	errChan := make(chan error)
	for _, walletAddress := range pWalletAddresses {
		go func(wa persist.Address) {
			assets, err := FetchAssetsForWallet(wa, 0, 0, nil)
			if err != nil {
				errChan <- fmt.Errorf("failed to fetch assets for wallet %s: %s", wa, err)
				return
			}
			if len(assets) == 0 {
				time.Sleep(time.Second)
				assets, err = FetchAssetsForWallet(wa, 0, 0, nil)
				if err != nil {
					errChan <- fmt.Errorf("failed to fetch assets for wallet %s: %s", wa, err)
					return
				}
				if len(assets) == 0 {
					errChan <- fmt.Errorf("no assets found for wallet address: %s", wa)
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

	for i := 0; i < len(pWalletAddresses); i++ {
		select {
		case nfts := <-nftsChan:
			result = append(result, nfts...)
		case err := <-errChan:
			return nil, err
		}
	}

	return result, nil
}

// FetchAssetsForWallet recursively fetches all assets for a wallet
func FetchAssetsForWallet(pWalletAddress persist.Address, pOffset int, retry int, alreadyReceived map[int]string) ([]Asset, error) {

	if alreadyReceived == nil {
		alreadyReceived = make(map[int]string)
	}

	result := []Asset{}

	if pOffset > 20000 {
		return result, fmt.Errorf("failed to fetch assets for wallet %s: too many results", pWalletAddress)
	}

	dir := "desc"
	offset := pOffset
	if pOffset > 10000 {
		dir = "asc"
		offset = pOffset - 10050
	}

	logrus.Debugf("Fetching assets for wallet %s with offset %d, retry %d, dir %s,and alreadyReceived %d", pWalletAddress, offset, retry, dir, len(alreadyReceived))

	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/assets?owner=%s&order_direction=%s&offset=%d&limit=%d", pWalletAddress, dir, offset, 50)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if retry < 3 {
				time.Sleep(time.Second * 2)
				return FetchAssetsForWallet(pWalletAddress, pOffset, retry+1, alreadyReceived)
			}
			return nil, fmt.Errorf("opensea api rate limit exceeded")
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	response := Assets{}
	err = util.UnmarshallBody(&response, resp.Body)
	if err != nil {
		return nil, err
	}

	doneReceiving := false
	for _, asset := range response.Assets {
		if it, ok := alreadyReceived[asset.ID]; ok {
			logrus.Debugf("response already received asset: %s", it)
			doneReceiving = true
			continue
		}
		result = append(result, asset)
		alreadyReceived[asset.ID] = fmt.Sprintf("asset-%d|offset-%d|len-%d|dir-%s", asset.ID, pOffset, len(result), dir)
	}
	if doneReceiving {
		return result, nil
	}
	if len(response.Assets) == 50 {
		next, err := FetchAssetsForWallet(pWalletAddress, pOffset+50, 0, alreadyReceived)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

func assetsToNFTs(pCtx context.Context, pWalletAddress persist.Address, openseaNfts []Asset, nftRepo persist.NFTRepository) ([]persist.NFT, error) {

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

func openseaToDBNft(pCtx context.Context, pWalletAddress persist.Address, nft Asset, nftRepo persist.NFTRepository) persist.NFT {

	result := persist.NFT{
		OwnerAddress:         pWalletAddress,
		MultipleOwners:       nft.Owner.Address == persist.ZeroAddress,
		Name:                 persist.NullString(nft.Name),
		Description:          persist.NullString(nft.Description),
		ExternalURL:          persist.NullString(nft.ExternalURL),
		ImageURL:             persist.NullString(nft.ImageURL),
		CreatorAddress:       nft.Creator.Address,
		AnimationURL:         persist.NullString(nft.AnimationURL),
		OpenseaTokenID:       nft.TokenID,
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
	if dbNFT != nil && len(dbNFT) == 1 {
		result.ID = dbNFT[0].ID
	}

	return result

}

func (e errNoNFTForTokenIdentifiers) Error() string {
	return fmt.Sprintf("no NFT found for token id %s and contract address %s", e.tokenID, e.contractAddress)
}

func (e errNoSingleNFTForOpenseaID) Error() string {
	return fmt.Sprintf("no single NFT found for opensea id %d", e.openseaID)
}
