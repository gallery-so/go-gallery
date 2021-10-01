package server

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

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// persist.GLRYnft struct tags reflect the json data of an open sea response and therefore
// can be unmarshalled from the api response
type openseaAssets struct {
	Assets []*openseaAsset `json:"assets"`
}

type openseaAsset struct {
	Version int64 `json:"version"` // schema version for this model
	ID      int   `json:"id"`

	Name        string `json:"name"`
	Description string `json:"description"`

	ExternalURL      string            `json:"external_link"`
	TokenMetadataURL string            `json:"token_metadata_url"`
	Creator          openseaAccount    `json:"creator"`
	Owner            openseaAccount    `json:"owner"`
	Contract         persist.Contract  `json:"asset_contract"`
	Collection       openseaCollection `json:"collection"`

	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	TokenID string `json:"token_id"`

	// IMAGES - OPENSEA
	ImageURL             string `json:"image_url"`
	ImageThumbnailURL    string `json:"image_thumbnail_url"`
	ImagePreviewURL      string `json:"image_preview_url"`
	ImageOriginalURL     string `json:"image_original_url"`
	AnimationURL         string `json:"animation_url"`
	AnimationOriginalURL string `json:"animation_original_url"`

	AcquisitionDateStr string `json:"acquisition_date"`
}

type openseaEvents struct {
	Events []openseaEvent `json:"asset_events"`
}
type openseaEvent struct {
	Asset       openseaAsset   `json:"asset"`
	ToAccount   openseaAccount `json:"to_account"`
	CreatedDate string         `json:"created_date"`
}

type openseaAccount struct {
	User    openseaUser `json:"user"`
	Address string      `json:"address"`
}

type openseaUser struct {
	Username string `json:"username"`
}

type openseaCollection struct {
	Name string `json:"name"`
}

func openSeaPipelineAssetsForAcc(pCtx context.Context, pUserID persist.DBID, pOwnerWalletAddresses []string, skipCache bool,
	pRuntime *runtime.Runtime) ([]*persist.Nft, error) {

	if !skipCache {
		nfts, err := persist.NftOpenseaCacheGet(pCtx, pOwnerWalletAddresses, pRuntime)
		if err == nil && len(nfts) > 0 {
			return nfts, nil
		}
	}

	user, err := persist.UserGetByID(pCtx, pUserID, pRuntime)
	if err != nil {
		return nil, err
	}
	if len(pOwnerWalletAddresses) == 0 {
		pOwnerWalletAddresses = user.Addresses
	}

	asDBNfts, err := openseaFetchAssetsForWallets(pCtx, pOwnerWalletAddresses, user, pRuntime)
	if err != nil {
		return nil, err
	}

	ids, err := persist.NftBulkUpsert(pCtx, asDBNfts, pRuntime)
	if err != nil {
		return nil, err
	}

	// update other user's collections and this user's collection so that they and ONLY they can display these
	// specific NFTs while also ensuring that NFTs they don't own don't list them as the owner
	go func() {
		if err := persist.CollClaimNFTs(pCtx, pUserID, pOwnerWalletAddresses, &persist.CollectionUpdateNftsInput{Nfts: ids}, pRuntime); err != nil {
			logrus.WithFields(logrus.Fields{"method": "openSeaPipelineAssetsForAcc"}).Errorf("failed to claim nfts: %v", err)
		}
	}()

	updatedNfts, err := openseaSyncHistories(pCtx, asDBNfts, pRuntime)
	if err != nil {
		return nil, err
	}

	result, err := dbToGalleryNFTs(pCtx, updatedNfts, user, pRuntime)
	if err != nil {
		return nil, err
	}

	err = persist.NftOpenseaCacheSet(pCtx, pOwnerWalletAddresses, result, pRuntime)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func openseaSyncHistories(pCtx context.Context, pNfts []*persist.NftDB, pRuntime *runtime.Runtime) ([]*persist.NftDB, error) {
	resultChan := make(chan *persist.NftDB)
	errorChan := make(chan error)
	updatedNfts := make([]*persist.NftDB, len(pNfts))

	// create a goroutine for each nft and sync history
	for _, nft := range pNfts {
		go func(n *persist.NftDB) {
			if !n.MultipleOwners {
				history, err := openseaSyncHistory(pCtx, n.OpenSeaTokenID, n.Contract.ContractAddress, pRuntime)
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

func openseaSyncHistory(pCtx context.Context, pTokenID string, pTokenContractAddress string, pRuntime *runtime.Runtime) (*persist.OwnershipHistory, error) {
	getURL := fmt.Sprintf("https://api.opensea.io/api/v1/events?token_id=%s&asset_contract_address=%s&event_type=transfer&only_opensea=false&limit=50&offset=0", pTokenID, pTokenContractAddress)
	events := &persist.OwnershipHistory{}
	resp, err := http.Get(getURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, err
	}
	openseaEvents := &openseaEvents{}
	if err := json.Unmarshal(buf.Bytes(), openseaEvents); err != nil {
		return nil, err
	}

	events, err = openseaToGalleryEvents(pCtx, openseaEvents, pRuntime)
	if err != nil {
		return nil, err
	}

	nfts, err := persist.NftGetByContractData(pCtx, pTokenID, pTokenContractAddress, pRuntime)
	if err != nil {
		return nil, err
	}
	if len(nfts) == 0 {
		return nil, fmt.Errorf("no NFT found for token id %s and contract address %s", pTokenID, pTokenContractAddress)
	}
	nft := nfts[0]

	err = persist.HistoryUpsert(pCtx, nft.ID, events, pRuntime)
	if err != nil {
		return nil, err
	}

	return events, nil
}

func openseaFetchAssetsForWallets(pCtx context.Context, pWalletAddresses []string, pUser *persist.User, pRuntime *runtime.Runtime) ([]*persist.NftDB, error) {
	result := []*persist.NftDB{}
	nftsChan := make(chan []*persist.NftDB)
	errChan := make(chan error)
	for _, walletAddress := range pWalletAddresses {
		go func(wa string) {
			assets, err := openseaFetchAssetsForWallet(wa, 0, pRuntime)
			if err != nil {
				errChan <- err
				return
			}
			asGlry, err := openseaToDBNfts(pCtx, wa, assets, pUser, pRuntime)
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
func openseaFetchAssetsForWallet(pWalletAddress string, pOffset int, pRuntime *runtime.Runtime) ([]*openseaAsset, error) {

	result := []*openseaAsset{}
	qsArgsMap := map[string]string{
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
	if pRuntime.Config.OpenseaAPIKey != "" {
		req.Header.Set("X-API-KEY", pRuntime.Config.OpenseaAPIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	response := &openseaAssets{}
	err = util.UnmarshallBody(response, resp.Body)
	if err != nil {
		return nil, err
	}
	result = append(result, response.Assets...)
	if len(response.Assets) == 50 {
		next, err := openseaFetchAssetsForWallet(pWalletAddress, pOffset+50, pRuntime)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

func openseaToDBNfts(pCtx context.Context, pWalletAddress string, openseaNfts []*openseaAsset, pUser *persist.User, pRuntime *runtime.Runtime) ([]*persist.NftDB, error) {

	nfts := make([]*persist.NftDB, len(openseaNfts))
	nftChan := make(chan *persist.NftDB)
	for _, openseaNft := range openseaNfts {
		go func(nft *openseaAsset) {
			nftChan <- openseaToDBNft(pCtx, pWalletAddress, nft, pUser.ID, pRuntime)
		}(openseaNft)
	}
	for i := 0; i < len(openseaNfts); i++ {
		nfts[i] = <-nftChan
	}
	return nfts, nil
}

func dbToGalleryNFTs(pCtx context.Context, pNfts []*persist.NftDB, pUser *persist.User, pRuntime *runtime.Runtime) ([]*persist.Nft, error) {

	nfts := make([]*persist.Nft, len(pNfts))
	nftChan := make(chan *persist.Nft)
	errChan := make(chan error)
	for _, nft := range pNfts {
		go func(n *persist.NftDB) {
			result := &persist.Nft{
				ID:                   n.ID,
				Name:                 n.Name,
				MultipleOwners:       n.MultipleOwners,
				Description:          n.Description,
				Version:              n.Version,
				CreationTime:         n.CreationTime,
				Deleted:              n.Deleted,
				CollectorsNote:       n.CollectorsNote,
				OwnerUsers:           []*persist.User{pUser},
				TokenCollectionName:  n.TokenCollectionName,
				OwnershipHistory:     n.OwnershipHistory,
				ExternalURL:          n.ExternalURL,
				TokenMetadataURL:     n.TokenMetadataURL,
				ImageURL:             n.ImageURL,
				CreatorAddress:       n.CreatorAddress,
				CreatorName:          n.CreatorName,
				OwnerAddresses:       n.OwnerAddresses,
				Contract:             n.Contract,
				OpenSeaID:            n.OpenSeaID,
				OpenSeaTokenID:       n.OpenSeaTokenID,
				ImageThumbnailURL:    n.ImageThumbnailURL,
				ImagePreviewURL:      n.ImagePreviewURL,
				ImageOriginalURL:     n.ImageOriginalURL,
				AnimationURL:         n.AnimationURL,
				AnimationOriginalURL: n.AnimationOriginalURL,
				AcquisitionDateStr:   n.AcquisitionDateStr,
			}
			if n.ID == "" {
				dbNFT, err := persist.NftGetByOpenseaID(pCtx, n.OpenSeaID, pRuntime)
				if err != nil {
					errChan <- err
					return
				}
				if len(dbNFT) != 1 {
					errChan <- fmt.Errorf("unable to find a single nft with opensea id %d", n.OpenSeaID)
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

func openseaToDBNft(pCtx context.Context, pWalletAddress string, nft *openseaAsset, ownerUserID persist.DBID, pRuntime *runtime.Runtime) *persist.NftDB {

	result := &persist.NftDB{
		OwnerAddresses:       []string{pWalletAddress},
		MultipleOwners:       nft.Owner.Address == "0x0000000000000000000000000000000000000000",
		Name:                 nft.Name,
		Description:          nft.Description,
		ExternalURL:          nft.ExternalURL,
		ImageURL:             nft.ImageURL,
		CreatorAddress:       strings.ToLower(nft.Creator.Address),
		AnimationURL:         nft.AnimationURL,
		OpenSeaTokenID:       nft.TokenID,
		OpenSeaID:            nft.ID,
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

	dbNFT, _ := persist.NftGetByOpenseaID(pCtx, nft.ID, pRuntime)
	if dbNFT != nil && len(dbNFT) == 1 {
		result.ID = dbNFT[0].ID
	}

	return result

}

func openseaToGalleryEvents(pCtx context.Context, pEvents *openseaEvents, pRuntime *runtime.Runtime) (*persist.OwnershipHistory, error) {
	timeLayout := "2006-01-02T15:04:05"
	ownershipHistory := &persist.OwnershipHistory{Owners: []*persist.Owner{}}
	for _, event := range pEvents.Events {
		owner := &persist.Owner{}
		time, err := time.Parse(timeLayout, event.CreatedDate)
		if err != nil {
			return nil, err
		}
		owner.TimeObtained = primitive.NewDateTimeFromTime(time)
		owner.Address = event.ToAccount.Address
		user, err := persist.UserGetByAddress(pCtx, event.ToAccount.Address, pRuntime)
		if err == nil {
			owner.UserID = user.ID
			owner.Username = user.UserName
		}
		ownershipHistory.Owners = append(ownershipHistory.Owners, owner)
	}
	sort.Slice(ownershipHistory.Owners, func(i, j int) bool {
		return ownershipHistory.Owners[i].TimeObtained.Time().After(ownershipHistory.Owners[j].TimeObtained.Time())
	})
	return ownershipHistory, nil
}
