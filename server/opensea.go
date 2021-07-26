package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	"github.com/parnurzeal/gorequest"
	log "github.com/sirupsen/logrus"
	// "github.com/davecgh/go-spew/spew"
)

// persist.GLRYnft struct tags reflect the json data of an open sea response and therefore
// can be unmarshalled from the api response
type openSeaApiResponse struct {
	Assets []*OpenseaNFT `json:"assets"`
}

type OpenseaNFT struct {
	VersionInt int64 `json:"version"` // schema version for this model
	IDint      int   `json:"id"`

	NameStr        string `json:"name"`
	DescriptionStr string `json:"description"`

	ExternalURLstr      string           `json:"external_url"`
	TokenMetadataUrlStr string           `json:"token_metadata_url"`
	CreatorAddressStr   string           `json:"creator_address"`
	CreatorNameStr      string           `json:"creator_name"`
	Contract            persist.Contract `json:"asset_contract"`

	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	TokenIdStr string `json:"token_id"`

	// IMAGES - OPENSEA
	ImageURLstr             string `json:"image_url"`
	ImageThumbnailURLstr    string `json:"image_thumbnail_url"`
	ImagePreviewURLstr      string `json:"image_preview_url"`
	ImageOriginalUrlStr     string `json:"image_original_url"`
	AnimationUrlStr         string `json:"animation_url"`
	AnimationOriginalUrlStr string `json:"animation_original_url"`

	AcquisitionDateStr string `json:"acquisition_date"`
}

//-------------------------------------------------------------
// OPEN_SEA_PIPELINE__ASSETS_FOR_ACC

// ADD!! - persist OpenSea fetched assets as well.

func OpenSeaPipelineAssetsForAcc(pOwnerWalletAddressStr string,
	pCtx context.Context,
	pRuntime *runtime.Runtime) ([]*persist.Nft, error) {

	//--------------------
	// OPENSEA_FETCH
	openSeaAssetsForAccLst, err := OpenSeaFetchAssetsForAcc(pOwnerWalletAddressStr, pCtx)
	if err != nil {
		return nil, err
	}

	//--------------------

	asGalleryNfts, err := openseaToGalleryNfts(openSeaAssetsForAccLst, pOwnerWalletAddressStr, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	// DB_PERSIST
	// CREATE_OR_UPDATE_BULK
	err = persist.NftBulkUpsert(pOwnerWalletAddressStr, asGalleryNfts, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}
	err = persist.NftRemoveDifference(asGalleryNfts, pOwnerWalletAddressStr, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	return asGalleryNfts, nil
}

//-------------------------------------------------------------
func OpenSeaFetchAssetsForAcc(pOwnerWalletAddressStr string,
	pCtx context.Context) ([]*OpenseaNFT, error) {

	/*{
	*	"id": 21976544,
	*	"token_id": "1137",
	*	"num_sales": 0,
		"background_color": null,
	*	"image_url": "https://lh3.googleusercontent.com/8S2uhc_74_JijJwYnNOEQvlnHs6dI4lU86k8Zj2WcelVG9Gp4hx62UDzf2B_R4cTMdd_03SLOV_rFZFF8_5vwFSEz76OX61of4ZPaA",
	*	"image_preview_url": "https://lh3.googleusercontent.com/8S2uhc_74_JijJwYnNOEQvlnHs6dI4lU86k8Zj2WcelVG9Gp4hx62UDzf2B_R4cTMdd_03SLOV_rFZFF8_5vwFSEz76OX61of4ZPaA=s250",
	*	"image_thumbnail_url": "https://lh3.googleusercontent.com/8S2uhc_74_JijJwYnNOEQvlnHs6dI4lU86k8Zj2WcelVG9Gp4hx62UDzf2B_R4cTMdd_03SLOV_rFZFF8_5vwFSEz76OX61of4ZPaA=s128",
	*	"image_original_url": "https://coin-nfts.s3.us-east-2.amazonaws.com/coin-500px.gif",
	*	"animation_url": null,
		"animation_original_url": null,
	*	"name": "April 14 2021",
	*	"description": "A special thank you for all of the hard work you put in to usher in an open financial system and bring Coinbase public.",
	*	"external_link": "https://www.coinbase.com/",
		"asset_contract": {
	*		"address": "0x6966ac85200cadd8c66d14d6c1a5431353edc8c9",
			"asset_contract_type": "non-fungible",
	*		"created_date": "2021-04-14T19:18:41.926804",
	*		"name": "Coinbase Direct Public Offering",
			"nft_version": "3.0",
			"opensea_version": null,
	*		"owner": 833403,
			"schema_name": "ERC721",
	*		"symbol": "$COIN",
			"total_supply": "0",
	*		"description": "We did it! This commemorative NFT is a special thank you to all of the people who worked tirelessly to bring Coinbase public.",
			"external_link": "https://www.coinbase.com/",
			"image_url": "https://lh3.googleusercontent.com/cudWSCgwfsRLdmHrZ7wBx74pk5xBLDOcAvxYMgQicyZ1wG3VeASwL5WXoJbR1P70CjGTCE-wc6mPpvV-AMGVhVZ9QTPzzcHGpal1=s120",
			"default_to_fiat": false,
			"dev_buyer_fee_basis_points": 0,
			"dev_seller_fee_basis_points": 0,
			"only_proxied_transfers": false,
			"opensea_buyer_fee_basis_points": 0,
			"opensea_seller_fee_basis_points": 250,
			"buyer_fee_basis_points": 0,
			"seller_fee_basis_points": 250,
			"payout_address": null
		},
		"owner": {
			"user": null,
	*		"profile_img_url": "https://storage.googleapis.com/opensea-static/opensea-profile/25.png",
	*		"address": "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
			"config": "",
			"discord_id": ""
		},
		"permalink": "https://opensea.io/assets/0x6966ac85200cadd8c66d14d6c1a5431353edc8c9/1137",
		"collection": {
			"banner_image_url": null,
			"chat_url": null,
	*		"created_date": "2021-04-14T19:22:29.035968",
			"default_to_fiat": false,
	*		"description": "We did it! This commemorative NFT is a special thank you to all of the people who worked tirelessly to bring Coinbase public.",
			"dev_buyer_fee_basis_points": "0",
			"dev_seller_fee_basis_points": "0",
			"discord_url": null,
			"display_data": {
				"card_display_style": "contain",
				"images": []
			},
	*		"external_url": "https://www.coinbase.com/",
			"featured": false,
			"featured_image_url": null,
			"hidden": true,
			"safelist_request_status": "not_requested",
	*		"image_url": "https://lh3.googleusercontent.com/cudWSCgwfsRLdmHrZ7wBx74pk5xBLDOcAvxYMgQicyZ1wG3VeASwL5WXoJbR1P70CjGTCE-wc6mPpvV-AMGVhVZ9QTPzzcHGpal1=s120",
			"is_subject_to_whitelist": false,
			"large_image_url": "https://lh3.googleusercontent.com/cudWSCgwfsRLdmHrZ7wBx74pk5xBLDOcAvxYMgQicyZ1wG3VeASwL5WXoJbR1P70CjGTCE-wc6mPpvV-AMGVhVZ9QTPzzcHGpal1",
			"medium_username": null,
	*		"name": "Coinbase Direct Public Offering",
			"only_proxied_transfers": false,
			"opensea_buyer_fee_basis_points": "0",
			"opensea_seller_fee_basis_points": "250",
	*		"payout_address": null,
			"require_email": false,
			"short_description": null,
			"slug": "coinbase-direct-public-offering",
			"telegram_url": null,
	*		"twitter_username": null,
	*		"instagram_username": null,
			"wiki_url": null
		},
		"decimals": 0,
		"sell_orders": null,
		"creator": {
	*		"user": null,
	*		"profile_img_url": "https://storage.googleapis.com/opensea-static/opensea-profile/25.png",
	*		"address": "0x70d04384b5c3a466ec4d8cfb8213efc31c6a9d15",
			"config": "",
			"discord_id": ""
		},
		"traits": [],
	*	"last_sale": null,
		"top_bid": null,
	*	"listing_date": null,
		"is_presale": false,
		"transfer_fee_payment_token": null,
		"transfer_fee": null
	}


	LAST_SALE:
	map[
		asset:map[
			decimals: <nil>
	*		token_id: 98168371784320387514732815439041609751844866237332060982262479279862392553473
		]
		asset_bundle:    <nil>
		auction_type:    <nil>
	*	created_date:    2021-03-18T17:22:56.965029
	*	event_timestamp: 2021-03-18T17:22:37
	*	event_type:      successful
		payment_token: map[
			address:   0x0000000000000000000000000000000000000000
			decimals:  18
	*		eth_price: 1.000000000000000
			id:        1
			image_url: https://lh3.googleusercontent.com/7hQyiGtBt8vmUTq4T0aIUhIhT00dPhnav87TuFQ5cLtjlk724JgXdjQjoH_CzYz-z37JpPuMFbRRQuyC7I9abyZRKA
			name:      Ether
	*		symbol:    ETH
	*		usd_price: 3446.929999999999836000
		]
	*	quantity:    1
		total_price: 250000000000000000
		transaction: map[
	*		block_hash:   0x16b63828a3ee23184949ed0ddbf6f24a1718cea610c59e83497d4db0a4ed6d50
	*		block_number: 12063985
	*		from_account: map[
	*			address: 0xbb3f043290841b97b9c92f6bc001a020d4b33255
				config:
				discord_id:
	*			profile_img_url: https://storage.googleapis.com/opensea-static/opensea-profile/8.png
				user: map[
	*				username:mikeybitcoin
				]
			]
	*		id:         9.1561879e+07
	*		timestamp:  2021-03-18T17:22:37
	*		to_account: map[
	*			address: 0x7be8076f4ea4a4ad08075c2508e481d6c946d12b
				config:  verified
				discord_id:
	*			profile_img_url: https://storage.googleapis.com/opensea-static/opensea-profile/22.png
				user: map[
	*				username:OpenSea-Orders
				]
			]
	*		transaction_hash:  0xb49f436bf95b22c6ddd35494e393d41eae728045045cf4cefbb2df599933adbd
	*		transaction_index: 68
		]
	]
	*/

	offsetInt := 0
	limitInt := 50
	qsArgsMap := map[string]string{
		"owner":           pOwnerWalletAddressStr,
		"order_direction": "desc",
		"offset":          fmt.Sprintf("%d", offsetInt),
		"limit":           fmt.Sprintf("%d", limitInt),
	}

	qsLst := []string{}
	for k, v := range qsArgsMap {
		qsLst = append(qsLst, fmt.Sprintf("%s=%s", k, v))
	}
	qsStr := strings.Join(qsLst, "&")
	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/assets?%s", qsStr)

	log.WithFields(log.Fields{
		"url":                  urlStr,
		"owner_wallet_address": pOwnerWalletAddressStr,
	}).Info("making HTTP request to OpenSea API")
	_, respBytes, errs := gorequest.New().Get(urlStr).EndBytes()
	if len(errs) > 0 {
		return nil, errs[0]
	}

	response := openSeaApiResponse{}
	err := json.Unmarshal(respBytes, &response)
	if err != nil {

		return nil, err
	}

	// spew.Dump(assetsForAccLst)

	return response.Assets, nil
}

func openseaToGalleryNfts(openseaNfts []*OpenseaNFT, pWalletAddress string, pCtx context.Context, pRuntime *runtime.Runtime) ([]*persist.Nft, error) {
	ownerUser, err := persist.UserGetByAddress(pWalletAddress, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}
	nfts := make([]*persist.Nft, len(openseaNfts))
	for i, openseaNft := range openseaNfts {
		nfts[i] = openseaToGalleryNft(openseaNft, pWalletAddress, ownerUser.IDstr)
	}
	return nfts, nil
}

func openseaToGalleryNft(nft *OpenseaNFT, pWalletAddres string, ownerUserId persist.DbId) *persist.Nft {

	result := &persist.Nft{
		OwnerUserIdStr:          ownerUserId,
		OwnerAddressStr:         pWalletAddres,
		NameStr:                 nft.NameStr,
		DescriptionStr:          nft.DescriptionStr,
		ExternalURLstr:          nft.ExternalURLstr,
		ImageURLstr:             nft.ImageURLstr,
		CreatorAddressStr:       nft.CreatorAddressStr,
		AnimationUrlStr:         nft.AnimationUrlStr,
		OpenSeaTokenIDstr:       nft.TokenIdStr,
		OpenSeaIDint:            nft.IDint,
		ImageThumbnailURLstr:    nft.ImageThumbnailURLstr,
		ImagePreviewURLstr:      nft.ImagePreviewURLstr,
		ImageOriginalUrlStr:     nft.ImageOriginalUrlStr,
		TokenMetadataUrlStr:     nft.TokenMetadataUrlStr,
		Contract:                nft.Contract,
		AcquisitionDateStr:      nft.AcquisitionDateStr,
		CreatorNameStr:          nft.CreatorNameStr,
		AnimationOriginalUrlStr: nft.AnimationOriginalUrlStr,
	}

	return result

}
