package opensea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
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
	Contract         persist.NftContract `json:"asset_contract"`
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

// PipelineAssetsForAcc is a pipeline for getting assets for an account
func PipelineAssetsForAcc(pCtx context.Context, pUserID persist.DBID, pOwnerWalletAddresses []persist.Address,
	nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, historyRepo persist.OwnershipHistoryRepository) ([]persist.NFT, error) {

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return nil, err
	}
	if len(pOwnerWalletAddresses) == 0 {
		pOwnerWalletAddresses = user.Addresses
	}

	asDBNfts, err := openseaFetchAssetsForWallets(pCtx, pOwnerWalletAddresses, nftRepo)
	if err != nil {
		return nil, err
	}

	ids, err := nftRepo.BulkUpsert(pCtx, pUserID, asDBNfts)
	if err != nil {
		return nil, err
	}

	// update other user's collections and this user's collection so that they and ONLY they can display these
	// specific NFTs while also ensuring that NFTs they don't own don't list them as the owner
	if err := collRepo.ClaimNFTs(pCtx, pUserID, pOwnerWalletAddresses, persist.CollectionUpdateNftsInput{Nfts: ids}); err != nil {
		return nil, err
	}

	result, err := dbToGalleryNFTs(pCtx, asDBNfts, user, nftRepo)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func openseaSyncHistories(pCtx context.Context, pNfts []*persist.NFTDB, userRepo persist.UserRepository, nftRepo persist.NFTRepository, historyRepo persist.OwnershipHistoryRepository) ([]*persist.NFTDB, error) {
	resultChan := make(chan *persist.NFTDB)
	errorChan := make(chan error)
	updatedNfts := make([]*persist.NFTDB, len(pNfts))

	// create a goroutine for each nft and sync history
	for _, nft := range pNfts {
		go func(n *persist.NFTDB) {
			if !n.MultipleOwners {
				history, err := openseaSyncHistory(pCtx, n.OpenseaTokenID, n.Contract.ContractAddress, n.OwnerAddress, userRepo, nftRepo, historyRepo)
				if err != nil {
					errorChan <- err
					return
				}

				n.OwnershipHistory = history
			}
			resultChan <- n
		}(nft)

	}

	// wait for all goroutines to finish and collect results from chan
	for i := 0; i < len(pNfts); i++ {
		select {
		case nft := <-resultChan:
			updatedNfts[i] = nft
		case err := <-errorChan:
			return nil, err

		}
	}
	return updatedNfts, nil
}

func openseaSyncHistory(pCtx context.Context, pTokenID persist.TokenID, pTokenContractAddress, pWalletAddress persist.Address, userRepo persist.UserRepository, nftRepo persist.NFTRepository, historyRepo persist.OwnershipHistoryRepository) (persist.OwnershipHistory, error) {
	getURL := fmt.Sprintf("https://api.opensea.io/api/v1/events?token_id=%s&asset_contract_address=%s&event_type=transfer&only_opensea=false&limit=50&offset=0", pTokenID, pTokenContractAddress)
	events := persist.OwnershipHistory{}
	resp, err := http.Get(getURL)
	if err != nil {
		return persist.OwnershipHistory{}, err
	}
	defer resp.Body.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return persist.OwnershipHistory{}, err
	}
	openseaEvents := &Events{}
	if err := json.Unmarshal(buf.Bytes(), openseaEvents); err != nil {
		return persist.OwnershipHistory{}, err
	}

	events, err = openseaToGalleryEvents(pCtx, openseaEvents, userRepo)
	if err != nil {
		return persist.OwnershipHistory{}, err
	}

	nfts, err := nftRepo.GetByContractData(pCtx, pTokenID, pTokenContractAddress)
	if err != nil {
		return persist.OwnershipHistory{}, err
	}
	if len(nfts) < 1 {
		return persist.OwnershipHistory{}, fmt.Errorf("no nfts found for token id: %s, contract address: %s", pTokenID, pTokenContractAddress)
	}

	err = historyRepo.Upsert(pCtx, nfts[0].ID, events)
	if err != nil {
		return persist.OwnershipHistory{}, err
	}

	return events, nil
}

func openseaFetchAssetsForWallets(pCtx context.Context, pWalletAddresses []persist.Address, nftRepo persist.NFTRepository) ([]persist.NFTDB, error) {
	result := []persist.NFTDB{}
	nftsChan := make(chan []persist.NFTDB)
	errChan := make(chan error)
	for _, walletAddress := range pWalletAddresses {
		go func(wa persist.Address) {
			assets, err := openseaFetchAssetsForWallet(wa, 0)
			if err != nil {
				errChan <- err
				return
			}
			if len(assets) == 0 {
				time.Sleep(time.Second)
				assets, err = openseaFetchAssetsForWallet(wa, 0)
				if err != nil {
					errChan <- err
					return
				}
			}
			asGlry, err := openseaToDBNfts(pCtx, wa, assets, nftRepo)
			if err != nil {
				errChan <- err
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

// recursively fetches all assets for a wallet
func openseaFetchAssetsForWallet(pWalletAddress persist.Address, pOffset int) ([]Asset, error) {

	result := []Asset{}
	qsArgsMap := map[string]interface{}{
		"owner":           pWalletAddress,
		"order_direction": "desc",
		"offset":          fmt.Sprintf("%d", pOffset),
		"limit":           fmt.Sprintf("%d", 50),
	}

	qsLst := []string{}
	for k, v := range qsArgsMap {
		qsLst = append(qsLst, fmt.Sprintf("%s=%s", k, v))
	}
	qsStr := strings.Join(qsLst, "&")
	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/assets?%s", qsStr)

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
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	response := Assets{}
	err = util.UnmarshallBody(&response, resp.Body)
	if err != nil {
		return nil, err
	}
	result = append(result, response.Assets...)
	if len(response.Assets) == 50 {
		next, err := openseaFetchAssetsForWallet(pWalletAddress, pOffset+50)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

func openseaToDBNfts(pCtx context.Context, pWalletAddress persist.Address, openseaNfts []Asset, nftRepo persist.NFTRepository) ([]persist.NFTDB, error) {

	nfts := make([]persist.NFTDB, len(openseaNfts))
	nftChan := make(chan persist.NFTDB)
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

func dbToGalleryNFTs(pCtx context.Context, pNfts []persist.NFTDB, pUser persist.User, nftRepo persist.NFTRepository) ([]persist.NFT, error) {

	nfts := make([]persist.NFT, len(pNfts))
	nftChan := make(chan persist.NFT)
	errChan := make(chan error)
	for _, nft := range pNfts {
		go func(n persist.NFTDB) {
			result := persist.NFT{
				ID:                   n.ID,
				Name:                 n.Name,
				MultipleOwners:       n.MultipleOwners,
				Description:          n.Description,
				Version:              n.Version,
				CreationTime:         n.CreationTime,
				Deleted:              n.Deleted,
				CollectorsNote:       n.CollectorsNote,
				TokenCollectionName:  n.TokenCollectionName,
				ExternalURL:          n.ExternalURL,
				TokenMetadataURL:     n.TokenMetadataURL,
				ImageURL:             n.ImageURL,
				CreatorAddress:       n.CreatorAddress,
				OwnershipHistory:     n.OwnershipHistory,
				CreatorName:          n.CreatorName,
				OwnerAddress:         n.OwnerAddress,
				Contract:             n.Contract,
				OpenseaID:            n.OpenseaID,
				OpenseaTokenID:       n.OpenseaTokenID,
				ImageThumbnailURL:    n.ImageThumbnailURL,
				ImagePreviewURL:      n.ImagePreviewURL,
				ImageOriginalURL:     n.ImageOriginalURL,
				AnimationURL:         n.AnimationURL,
				AnimationOriginalURL: n.AnimationOriginalURL,
				AcquisitionDateStr:   n.AcquisitionDateStr,
			}
			if n.ID == "" {
				dbNFT, err := nftRepo.GetByOpenseaID(pCtx, n.OpenseaID, n.OwnerAddress)
				if err != nil {
					errChan <- err
					return
				}
				if len(dbNFT) == 0 {
					errChan <- errNoSingleNFTForOpenseaID{n.OpenseaID}
					return
				}
				result.ID = dbNFT[0].ID
			}
			nftChan <- result
		}(nft)
	}
	for i := 0; i < len(pNfts); i++ {
		select {
		case nft := <-nftChan:
			nfts[i] = nft
		case err := <-errChan:
			return nil, err
		}
	}

	return nfts, nil
}

func openseaToDBNft(pCtx context.Context, pWalletAddress persist.Address, nft Asset, nftRepo persist.NFTRepository) persist.NFTDB {

	result := persist.NFTDB{
		OwnerAddress:         pWalletAddress,
		MultipleOwners:       nft.Owner.Address == "0x0000000000000000000000000000000000000000",
		Name:                 nft.Name,
		Description:          nft.Description,
		ExternalURL:          nft.ExternalURL,
		ImageURL:             nft.ImageURL,
		CreatorAddress:       nft.Creator.Address,
		AnimationURL:         nft.AnimationURL,
		OpenseaTokenID:       nft.TokenID,
		OpenseaID:            nft.ID,
		TokenCollectionName:  nft.Collection.Name,
		ImageThumbnailURL:    nft.ImageThumbnailURL,
		ImagePreviewURL:      nft.ImagePreviewURL,
		ImageOriginalURL:     nft.ImageOriginalURL,
		TokenMetadataURL:     nft.TokenMetadataURL,
		Contract:             nft.Contract,
		AcquisitionDateStr:   nft.AcquisitionDateStr,
		CreatorName:          nft.Creator.User.Username,
		AnimationOriginalURL: nft.AnimationOriginalURL,
	}

	dbNFT, _ := nftRepo.GetByOpenseaID(pCtx, nft.ID, pWalletAddress)
	if dbNFT != nil && len(dbNFT) == 1 {
		result.ID = dbNFT[0].ID
	}

	return result

}

func openseaToGalleryEvents(pCtx context.Context, pEvents *Events, userRepo persist.UserRepository) (persist.OwnershipHistory, error) {
	timeLayout := "2006-01-02T15:04:05"
	ownershipHistory := persist.OwnershipHistory{Owners: []persist.Owner{}}
	for _, event := range pEvents.Events {
		owner := persist.Owner{}
		time, err := time.Parse(timeLayout, event.CreatedDate)
		if err != nil {
			return persist.OwnershipHistory{}, err
		}
		owner.TimeObtained = time
		owner.Address = event.ToAccount.Address
		user, err := userRepo.GetByAddress(pCtx, event.ToAccount.Address)
		if err == nil {
			owner.UserID = user.ID
			owner.Username = user.UserName
		}
		ownershipHistory.Owners = append(ownershipHistory.Owners, owner)
	}
	sort.Slice(ownershipHistory.Owners, func(i, j int) bool {
		return ownershipHistory.Owners[i].TimeObtained.After(ownershipHistory.Owners[j].TimeObtained)
	})
	return ownershipHistory, nil
}

func (e errNoNFTForTokenIdentifiers) Error() string {
	return fmt.Sprintf("no NFT found for token id %s and contract address %s", e.tokenID, e.contractAddress)
}

func (e errNoSingleNFTForOpenseaID) Error() string {
	return fmt.Sprintf("no single NFT found for opensea id %d", e.openseaID)
}
