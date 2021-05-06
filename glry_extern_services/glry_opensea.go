package glry_extern_services

import (
	"fmt"
	"context"
	"encoding/json"
	"strings"
	log "github.com/sirupsen/logrus"
	"github.com/parnurzeal/gorequest"
	"github.com/mitchellh/mapstructure"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
	// "github.com/davecgh/go-spew/spew"
)

//-------------------------------------------------------------
type GLRYopenSeaAsset struct {
	IDint               int    `bson:"id"        mapstructure:"id"`
	TokenIDstr          string `bson:"token_id"  mapstructure:"token_id"`
	NumberOfSalesInt    int    `bson:"num_sales" mapstructure:"num_sales"`
	
	// IMAGES
	ImageURLstr         string                   `bson:"image_url"           mapstructure:"image_url"`
	ImagePreviewURLstr  string                   `bson:"image_preview_url"   mapstructure:"image_preview_url"`
	ImageThumbURLstr    string                   `bson:"image_thumbnail_url" mapstructure:"image_thumbnail_url"`
	ImageOriginalURLstr string                   `bson:"image_original_url"  mapstructure:"image_original_url"`
	AnimationURLstr     string                   `bson:"animation_url"       mapstructure:"animation_url"`

	NameStr             string                   `bson:"name"           mapstructure:"name"`
	DescriptionStr      string                   `bson:"description"    mapstructure:"description"`
	ExternLinkStr       string                   `bson:"external_link"  mapstructure:"external_link"`
	AssetContract       GLRYopenSeaAssetContract `bson:"asset_contract" mapstructure:"asset_contract"`
	Owner               GLRYopenSeaOwner         `bson:"owner"          mapstructure:"owner"`
	PermaLinkStr        string                   `bson:"permalink"      mapstructure:"permalink"`

	// IMPORTANT!! - OpenSea (unlike Gallery) only allows an Asset to be in a single collection 
	Collection GLRYopenSeaCollection `bson:"collection" mapstructure:"collection"`
	
	Creator        GLRYopenSeaCreator   `bson:"creator"      mapstructure:"creator"`
	ListingDateStr string               `bson:"listing_date" mapstructure:"listing_date"`
	LastSale       *GLRYopenSeaLastSale `bson:"last_sale" mapstructure:"last_sale"`
}

type GLRYopenSeaAssetContract struct {
	AddressStr     string `bson:"address"      mapstructure:"address"`
	CreatedDateStr string `bson:"created_date" mapstructure:"created_date"`
	NameStr        string `bson:"name"         mapstructure:"name"`
	OwnerInt       int    `bson:"owner"        mapstructure:"owner"`
	SymbolStr      string `bson:"symbol"       mapstructure:"symbol"`
	DescriptionStr string `bson:"description"  mapstructure:"description"`
}

type GLRYopenSeaOwner struct {
	User               GLRYopenSeaUser `bson:"user"            mapstructure:"user"`
	ProfileImageURLstr string          `bson:"profile_img_url" mapstructure:"profile_img_url"`
	AddressStr         string          `bson:"address"         mapstructure:"address"`
}

type GLRYopenSeaCollection struct {
	CreatedDateStr       string `bson:"created_date"       mapstructure:"created_date"`
	DescriptionStr       string `bson:"description"        mapstructure:"description"`
	ExternalURLstr       string `bson:"external_url"       mapstructure:"external_url"`
	ImageURLstr          string `bson:"image_url"          mapstructure:"image_url"`
	NameStr              string `bson:"name"               mapstructure:"name"`
	PayoutAddressStr     string `bson:"payout_address"     mapstructure:"payout_address"`
	TwitterUsernameStr   string `bson:"twitter_username"   mapstructure:"twitter_username"`
	InstagramUsernameStr string `bson:"instagram_username" mapstructure:"instagram_username"`
}

type GLRYopenSeaCreator struct {
	User               GLRYopenSeaUser `bson:"user"            mapstructure:"user"`
	ProfileImageURLstr string          `bson:"profile_img_url" mapstructure:"profile_img_url"`
	AddressStr         string          `bson:"address"         mapstructure:"address"`
}

type GLRYopenSeaUser struct {
	UsernameStr string `bson:"username" mapstructure:"username"`
}

// LAST_SALE
// ADD!! - this is a single LastSale, not a chain of custody, 
//         so if this chain is to be rebuilt this LastSale has to be continuously queried
//         for an assert and results persisted for future reference.
type GLRYopenSeaLastSale struct {
	TokenIDstr        string                     `bson:"token_id"` // this is nested in "asset" field, but I wanted surfaced as a top attribute, so no mapstructure
	EventTimestampStr string                     `bson:"event_timestamp" mapstructure:"eventtimestamp"` 
	EventTypeStr      string                     `bson:"event_type"      mapstructure:"event_type"`
	PaymentToken GLRYopenSeaLastSalePaymentToken `bson:"payment_token"   mapstructure:"payment_token"`
	QuantityStr  string                          `bson:"quantity"        mapstructure:"quantity"`
	Transaction  GLRYopenSeaLastSaleTx           `bson:"transaction"     mapstructure:"transaction"`
}

type GLRYopenSeaLastSalePaymentToken struct {
	EthPriceStr string `bson:"eth_price" mapstructure:"eth_price"`
	SymbolStr   string `bson:"symbol"    mapstructure:"symbol"`
	USDpriceStr string `bson:"usd_price" mapstructure:"usd_price"`
}

// TX
type GLRYopenSeaLastSaleTx struct {
	IDf            float64        `bson:"id"           mapstructure:"id"`
	TimestampStr   string         `bson:"timestamp"    mapstructure:"timestamp"`
	BlockHashStr   string         `bson:"block_hash"   mapstructure:"block_hash"`
	BlockNumberStr string         `bson:"block_number" mapstructure:"block_number"`
	FromAcc        GLRYopenSeaAcc `bson:"from_acc"     mapstructure:"from_account"`
	ToAcc          GLRYopenSeaAcc `bson:"to_acc"       mapstructure:"to_account"`
	TxHashStr      string         `bson:"tx_hash"      mapstructure:"transaction_hash"`
	TxIndexStr     string         `bson:"tx_index"     mapstructure:"transaction_index"`
}

// ACC
type GLRYopenSeaAcc struct {
	AddressStr       string            `bson:"address"         mapstructure:"address"`
	ProfileImgURLstr string            `bson:"profile_img_url" mapstructure:"profile_img_url"`
	UserMap          map[string]string `bson:"user"            mapstructure:"user"`
}

//-------------------------------------------------------------
func OpenSeaFetchAssetsForAcc(pOwnerWalletAddressStr string,
	pCtx context.Context,
	pRuntimeSys *gfcore.Runtime_sys) ([]*GLRYopenSeaAsset, *gfcore.Gf_error) {

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
		
		// FIX!! - add error capture here!!!
	}

	var response map[string]interface{}
	err := json.Unmarshal(respBytes, &response)
	if err != nil {
		gErr := gfcore.Error__create(fmt.Sprintf("failed to parse json response from OpenSea API"), 
			"json_decode_error",
			map[string]interface{}{"url": urlStr,},
			err, "glry_extern_services", pRuntimeSys)
		return nil, gErr
	}




	assetsLst       := response["assets"].([]interface{})
	assetsForAccLst := []*GLRYopenSeaAsset{}

	for _, aMap := range assetsLst {

		var asset GLRYopenSeaAsset
		err := mapstructure.Decode(aMap, &asset)
		if err != nil {
			
			gErr := gfcore.Error__create("failed to load OpenSea asset map into a GLRYopenSeaAsset struct",
				"mapstruct__decode",
				map[string]interface{}{
					"url":                  urlStr,
					"owner_wallet_address": pOwnerWalletAddressStr,
				},
				err, "glry_extern_services", pRuntimeSys)
			
			return nil, gErr
		}




		// LAST_SALE - for some assets this is null, for others its specified.
		//             this loading of token_id is done manually here to avoid another nested struct "asset"
		//             with just a single attribute "token_id".
		//             allthough this special case conditional adds confusion and is not consistent with how
		//             all other substructs are loaded.
		if asset.LastSale != nil {
			asset.LastSale.TokenIDstr = aMap.(map[string]interface{})["last_sale"].(map[string]interface{})["asset"].(map[string]interface{})["token_id"].(string)
		}




		assetsForAccLst = append(assetsForAccLst, &asset)
	}
	
	// spew.Dump(assetsForAccLst)

	return assetsForAccLst, nil
}