package db

import (
	"context"
	gfcore "github.com/gloflow/gloflow/go/gf_core"
)

//-------------------------------------------------------------
// legacy NFT type, this is the schema in the initial v0 prototype of the system
type NFTlegacy struct {

	ID int64 `bson:"_id" json:"id"`

	// removed from newer NFT model, NFT's might be associated with multiple Users,
	// so we dont want to limit to a single user.
	UserID string `bson:"user_id" json:"user_id"`

	ImageURL          string    `bson:"image_url"           json:"image_url"`
	Description       string    `bson:"description"         json:"description"`
	Name              string    `bson:"name"                json:"name"`
	CollectionName    string    `bson:"collection_name"     json:"collection_name"`
	Position          int64     `bson:"position"            json:"position"`
	ExternalURL       string    `bson:"external_url"        json:"external_url"`
	CreatedDate       float64   `bson:"created_date"        json:"created_date"`
	CreatorAddress    string    `bson:"creator_address"     json:"creator_address"`
	ContractAddress   string    `bson:"contract_address"    json:"contract_address"`
	TokenID           int64     `bson:"token_id"            json:"token_id"`
	Hidden            bool      `bson:"hidden"              json:"hidden"`
	ImageThumbnailURL string    `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL   string    `bson:"image_preview_url"   json:"image_preview_url"`
}

type NFT struct {
	Version           int64     `bson:"version"             json:"version"` // schema version for this model
	ID                int64     `bson:"id"                  json:"id"`
	ImageURL          string    `bson:"image_url"           json:"image_url"`
	Description       string    `bson:"description"         json:"description"`
	Name              string    `bson:"name"                json:"name"`
	CollectionName    string    `bson:"collection_name"     json:"collection_name"`
	Position          int64     `bson:"position"            json:"position"`
	ExternalURL       string    `bson:"external_url"        json:"external_url"`
	CreatedDate       float64   `bson:"created_date"        json:"created_date"`
	CreatorAddress    string    `bson:"creator_address"     json:"creator_address"`
	ContractAddress   string    `bson:"contract_address"    json:"contract_address"`
	TokenID           int64     `bson:"token_id"            json:"token_id"`
	Hidden            bool      `bson:"hidden"              json:"hidden"`
	ImageThumbnailURL string    `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL   string    `bson:"image_preview_url"   json:"image_preview_url"`
}

//-------------------------------------------------------------
func NFTcreate(pNFT *NFT,
	pCtx        context.Context,
	pRuntimeSys *gfcore.Runtime_sys) *gfcore.Gf_error {



	collNameStr := "glry_nfts"
	
	gErr := gfcore.Mongo__insert(pNFT,
		collNameStr,
		map[string]interface{}{
			"nft_name":           pNFT.Name,
			"nft_image_url":      pNFT.ImageURL,
			"caller_err_msg_str": "failed to insert a new NFT into the DB",
		},
		pCtx,
		pRuntimeSys)
	if gErr != nil {
		return gErr
	}



	return nil
}

//-------------------------------------------------------------
func NFTgetByUserID(pUserIDstr string,
	pCtx context.Context) ([]*NFT, *gfcore.Gf_error) {





	return nil, nil
}

/*func (db *DB) GetNFTsByUserID(ctx context.Context, userID string) ([]*NFT, error) {
	var nfts []*NFT

	query := `
SELECT
	id,
	user_id,
	image_url,
--	description
	name,
	collection_name,
	position,
	external_url,
	created_date,
	creator_address,
	contract_address,
--	token_id,
	hidden,
	image_thumbnail_url,
	image_preview_url
FROM nfts
WHERE user_id='%s'
`
	err := pgxscan.Select(ctx, db.pool, &nfts, fmt.Sprintf(query, userID))
	if err != nil {
		return nil, err
	}

	return nfts, nil
}*/