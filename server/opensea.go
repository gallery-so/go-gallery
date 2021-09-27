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
type openSeaAssets struct {
	Assets []*openseaAsset `json:"assets"`
}

type openseaAsset struct {
	Version int64 `json:"version"` // schema version for this model
	ID      int   `json:"id"`

	Name        string `json:"name"`
	Description string `json:"description"`

	ExternalURL      string           `json:"external_link"`
	TokenMetadataURL string           `json:"token_metadata_url"`
	Creator          openseaAccount   `json:"creator"`
	Owner            openseaAccount   `json:"owner"`
	Contract         persist.Contract `json:"asset_contract"`

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

	openSeaAssetsForAccLst, err := openseaFetchAssetsForWallets(pOwnerWalletAddresses)
	if err != nil {
		return nil, err
	}

	asDBNfts, err := openseaToDBNfts(pCtx, openSeaAssetsForAccLst, user, pRuntime)
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
		err = persist.CollClaimNFTs(pCtx, pUserID, pOwnerWalletAddresses, &persist.CollectionUpdateNftsInput{Nfts: ids}, pRuntime)
		if err != nil {
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
		go func(nft *persist.NftDB) {
			history, err := openseaSyncHistory(pCtx, nft.OpenSeaTokenID, nft.Contract.ContractAddress, pRuntime)
			if err != nil {
				errorChan <- err
				return
			}

			nft.OwnershipHistory = history
			resultChan <- nft
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
	var openseaID int
	if len(openseaEvents.Events) > 0 {
		openseaID = openseaEvents.Events[0].Asset.ID
	}

	events, err = openseaToGalleryEvents(pCtx, openseaEvents, pRuntime)
	if err != nil {
		return nil, err
	}

	nfts, err := persist.NftGetByOpenseaID(pCtx, openseaID, pRuntime)
	if err != nil {
		return nil, err
	}
	if len(nfts) == 0 {
		return nil, fmt.Errorf("no NFT found for opensea id %d", openseaID)
	}
	nft := nfts[0]

	err = persist.HistoryUpsert(pCtx, nft.ID, events, pRuntime)
	if err != nil {
		return nil, err
	}

	return events, nil
}

func openseaFetchAssetsForWallets(pWalletAddresses []string) ([]*openseaAsset, error) {
	result := []*openseaAsset{}
	for _, walletAddress := range pWalletAddresses {
		assets, err := openseaFetchAssetsForWallet(walletAddress, 0)
		if err != nil {
			return nil, err
		}
		result = append(result, assets...)
	}

	return result, nil
}

// recursively fetches all assets for a wallet
func openseaFetchAssetsForWallet(pWalletAddress string, offset int) ([]*openseaAsset, error) {

	result := []*openseaAsset{}
	qsArgsMap := map[string]string{
		"owner":           pWalletAddress,
		"order_direction": "desc",
		"offset":          fmt.Sprintf("%d", offset),
		"limit":           fmt.Sprintf("%d", 50),
	}

	qsLst := []string{}
	for k, v := range qsArgsMap {
		qsLst = append(qsLst, fmt.Sprintf("%s=%s", k, v))
	}
	qsStr := strings.Join(qsLst, "&")
	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/assets?%s", qsStr)

	resp, err := http.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	response := &openSeaAssets{}
	err = util.UnmarshallBody(response, resp.Body)
	if err != nil {
		return nil, err
	}
	result = append(result, response.Assets...)
	if len(response.Assets) == 50 {
		next, err := openseaFetchAssetsForWallet(pWalletAddress, offset+50)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}
	return result, nil
}

func openseaToDBNfts(pCtx context.Context, openseaNfts []*openseaAsset, pUser *persist.User, pRuntime *runtime.Runtime) ([]*persist.NftDB, error) {

	nfts := make([]*persist.NftDB, len(openseaNfts))
	nftChan := make(chan *persist.NftDB)
	for _, openseaNft := range openseaNfts {
		go func(openseaNft *openseaAsset) {
			nftChan <- openseaToDBNft(pCtx, openseaNft, pUser.ID, pRuntime)
		}(openseaNft)
	}
	for i := 0; i < len(openseaNfts); i++ {
		select {
		case nft := <-nftChan:
			nfts[i] = nft
		}
	}
	return nfts, nil
}

func dbToGalleryNFTs(pCtx context.Context, pNfts []*persist.NftDB, pUser *persist.User, pRuntime *runtime.Runtime) ([]*persist.Nft, error) {

	nfts := make([]*persist.Nft, len(pNfts))
	for i, nft := range pNfts {
		nfts[i] = &persist.Nft{
			ID:                   nft.ID,
			Name:                 nft.Name,
			Description:          nft.Description,
			Version:              nft.Version,
			CreationTime:         nft.CreationTime,
			Deleted:              nft.Deleted,
			CollectorsNote:       nft.CollectorsNote,
			OwnerUserID:          pUser.ID,
			OwnerUsername:        pUser.UserName,
			OwnershipHistory:     nft.OwnershipHistory,
			ExternalURL:          nft.ExternalURL,
			TokenMetadataURL:     nft.TokenMetadataURL,
			ImageURL:             nft.ImageURL,
			CreatorAddress:       nft.CreatorAddress,
			CreatorName:          nft.CreatorName,
			OwnerAddress:         nft.OwnerAddress,
			Contract:             nft.Contract,
			OpenSeaID:            nft.OpenSeaID,
			OpenSeaTokenID:       nft.OpenSeaTokenID,
			ImageThumbnailURL:    nft.ImageThumbnailURL,
			ImagePreviewURL:      nft.ImagePreviewURL,
			ImageOriginalURL:     nft.ImageOriginalURL,
			AnimationURL:         nft.AnimationURL,
			AnimationOriginalURL: nft.AnimationOriginalURL,
			AcquisitionDateStr:   nft.AcquisitionDateStr,
		}
	}

	return nfts, nil
}

func openseaToDBNft(pCtx context.Context, nft *openseaAsset, ownerUserID persist.DBID, pRuntime *runtime.Runtime) *persist.NftDB {

	result := &persist.NftDB{
		OwnerUserID:          ownerUserID,
		OwnerAddress:         strings.ToLower(nft.Owner.Address),
		Name:                 nft.Name,
		Description:          nft.Description,
		ExternalURL:          nft.ExternalURL,
		ImageURL:             nft.ImageURL,
		CreatorAddress:       strings.ToLower(nft.Creator.Address),
		AnimationURL:         nft.AnimationURL,
		OpenSeaTokenID:       nft.TokenID,
		OpenSeaID:            nft.ID,
		ImageThumbnailURL:    nft.ImageThumbnailURL,
		ImagePreviewURL:      nft.ImagePreviewURL,
		ImageOriginalURL:     nft.ImageOriginalURL,
		TokenMetadataURL:     nft.TokenMetadataURL,
		Contract:             nft.Contract,
		AcquisitionDateStr:   nft.AcquisitionDateStr,
		CreatorName:          nft.Creator.User.Username,
		AnimationOriginalURL: nft.AnimationOriginalURL,
	}
	nfts, _ := persist.NftGetByOpenseaID(pCtx, nft.ID, pRuntime)
	if nfts != nil && len(nfts) > 0 {
		result.ID = nfts[0].ID
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
