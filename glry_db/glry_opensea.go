package glry_db

//-------------------------------------------------------------
type GLRYopenSeaAsset struct {
	VersionInt    int64   `bson:"version"`                         // schema version for this model
	IDint         int     `bson:"_id"           mapstructure:"id"` // ID assigned to asset by OpenSea
	CreationTimeF float64 `bson:"creation_time" json:"creation_time"`
	DeletedBool   bool    `bson:"deleted"`

	TokenIDstr       string `bson:"token_id"  mapstructure:"token_id"`
	NumberOfSalesInt int    `bson:"num_sales" mapstructure:"num_sales"`

	// IMAGES
	ImageURLstr         string `bson:"image_url"           mapstructure:"image_url"`
	ImagePreviewURLstr  string `bson:"image_preview_url"   mapstructure:"image_preview_url"`
	ImageThumbURLstr    string `bson:"image_thumbnail_url" mapstructure:"image_thumbnail_url"`
	ImageOriginalURLstr string `bson:"image_original_url"  mapstructure:"image_original_url"`
	AnimationURLstr     string `bson:"animation_url"       mapstructure:"animation_url"`

	NameStr        string                   `bson:"name"           mapstructure:"name"`
	DescriptionStr string                   `bson:"description"    mapstructure:"description"`
	ExternLinkStr  string                   `bson:"external_link"  mapstructure:"external_link"`
	AssetContract  GLRYopenSeaAssetContract `bson:"asset_contract" mapstructure:"asset_contract"`
	Owner          GLRYopenSeaOwner         `bson:"owner"          mapstructure:"owner"`
	PermaLinkStr   string                   `bson:"permalink"      mapstructure:"permalink"`

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
	TokenIDstr        string                          `bson:"token_id"` // this is nested in "asset" field, but I wanted surfaced as a top attribute, so no mapstructure
	EventTimestampStr string                          `bson:"event_timestamp" mapstructure:"eventtimestamp"`
	EventTypeStr      string                          `bson:"event_type"      mapstructure:"event_type"`
	PaymentToken      GLRYopenSeaLastSalePaymentToken `bson:"payment_token"   mapstructure:"payment_token"`
	QuantityStr       string                          `bson:"quantity"        mapstructure:"quantity"`
	Transaction       GLRYopenSeaLastSaleTx           `bson:"transaction"     mapstructure:"transaction"`
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
func OpenSeaCreateBulk() {

}
