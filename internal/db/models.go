package db

import "time"

// Define NFT struct
// Query Function by user_id
// 7bfaafcc-722e-4dce-986f-fe0d9bee2047
// return populated NFT struct

type NFT struct {
	ID int64 `db:"id" json:"id"`
	UserID string `db:"user_id" json:"user_id"`
	ImageURL string `db:"image_url" json:"image_url"`
	Description string `db:"description" json:"description"`
	Name string `db:"name" json:"name"`
	CollectionName string `db:"collection_name" json:"collection_name"`
	Position int64 `db:"position" json:"position"`
	ExternalURL string `db:"external_url" json:"external_url"`
	CreatedDate time.Time `db:"created_date" json:"created_date"`
	CreatorAddress string `db:"creator_address" json:"creator_address"`
	ContractAddress string `db:"contract_address" json:"contract_address"`
	TokenID int64 `db:"token_id" json:"token_id"`
	Hidden bool `db:"hidden" json:"hidden"`
	ImageThumbnailURL string `db:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL string `db:"image_preview_url" json:"image_preview_url"`
}
