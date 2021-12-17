package persist

import (
	"context"
	"fmt"
)

// NFTDB represents an nft in the database
type NFTDB struct {
	Version         int64           `bson:"version"              json:"version"` // schema version for this model
	ID              DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime    CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted         bool            `bson:"deleted" json:"-"`
	LastUpdatedTime LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote string  `bson:"collectors_note" json:"collectors_note"`
	OwnerAddress   Address `bson:"owner_address" json:"owner_address"`

	MultipleOwners bool `bson:"multiple_owners" json:"multiple_owners"`

	OwnershipHistory OwnershipHistory `bson:"ownership_history,only_get" json:"ownership_history"`

	Name                string      `bson:"name"                 json:"name"`
	Description         string      `bson:"description"          json:"description"`
	ExternalURL         string      `bson:"external_url"         json:"external_url"`
	TokenMetadataURL    string      `bson:"token_metadata_url" json:"token_metadata_url"`
	CreatorAddress      Address     `bson:"creator_address"      json:"creator_address"`
	CreatorName         string      `bson:"creator_name" json:"creator_name"`
	Contract            NftContract `bson:"contract"     json:"asset_contract"`
	TokenCollectionName string      `bson:"token_collection_name" json:"token_collection_name"`

	OpenseaID int `bson:"opensea_id"       json:"opensea_id"`
	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenseaTokenID TokenID `bson:"opensea_token_id" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURL             string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL    string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL      string `bson:"image_preview_url"   json:"image_preview_url"`
	ImageOriginalURL     string `bson:"image_original_url" json:"image_original_url"`
	AnimationURL         string `bson:"animation_url" json:"animation_url"`
	AnimationOriginalURL string `bson:"animation_original_url" json:"animation_original_url"`

	AcquisitionDateStr string `bson:"acquisition_date" json:"acquisition_date"`
}

// NFT represents an nft throughout the application
type NFT struct {
	Version         int64           `bson:"version"              json:"version"` // schema version for this model
	ID              DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime    CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted         bool            `bson:"deleted" json:"-"`
	LastUpdatedTime LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote string `bson:"collectors_note" json:"collectors_note"`

	// OwnerUsers     []*User  `bson:"owner_users" json:"owner_users"`
	OwnerAddress Address `bson:"owner_address" json:"owner_address"`

	MultipleOwners bool `bson:"multiple_owners" json:"multiple_owners"`

	OwnershipHistory OwnershipHistory `bson:"ownership_history" json:"ownership_history"`

	Name                string      `bson:"name"                 json:"name"`
	Description         string      `bson:"description"          json:"description"`
	ExternalURL         string      `bson:"external_url"         json:"external_url"`
	TokenMetadataURL    string      `bson:"token_metadata_url" json:"token_metadata_url"`
	CreatorAddress      Address     `bson:"creator_address"      json:"creator_address"`
	CreatorName         string      `bson:"creator_name" json:"creator_name"`
	Contract            NftContract `bson:"contract"     json:"asset_contract"`
	TokenCollectionName string      `bson:"token_collection_name" json:"token_collection_name"`

	OpenseaID int `bson:"opensea_id"       json:"opensea_id"`
	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenseaTokenID TokenID `bson:"opensea_token_id" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURL             string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL    string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL      string `bson:"image_preview_url"   json:"image_preview_url"`
	ImageOriginalURL     string `bson:"image_original_url" json:"image_original_url"`
	AnimationURL         string `bson:"animation_url" json:"animation_url"`
	AnimationOriginalURL string `bson:"animation_original_url" json:"animation_original_url"`

	AcquisitionDateStr string `bson:"acquisition_date" json:"acquisition_date"`
}

// CollectionNFT represents and NFT in a collection of NFTs
type CollectionNFT struct {
	ID           DBID         `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime `bson:"created_at"        json:"created_at"`

	OwnerAddress Address `bson:"owner_address" json:"owner_address"`

	MultipleOwners bool `bson:"multiple_owners" json:"multiple_owners"`

	Name string `bson:"name"                 json:"name"`

	Contract            ContractCollectionNFT `bson:"contract"     json:"asset_contract"`
	TokenCollectionName string                `bson:"token_collection_name" json:"token_collection_name"`
	CreatorAddress      string                `bson:"creator_address"      json:"creator_address"`
	CreatorName         string                `bson:"creator_name" json:"creator_name"`

	// IMAGES - OPENSEA
	ImageURL          string `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL string `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL   string `bson:"image_preview_url"   json:"image_preview_url"`
}

// NftContract represents a smart contract's information for a given NFT
type NftContract struct {
	ContractAddress      Address `bson:"contract_address"     json:"address"`
	ContractName         string  `bson:"contract_name" json:"name"`
	ContractImage        string  `bson:"contract_image_url" json:"image_url"`
	ContractDescription  string  `bson:"contract_description" json:"description"`
	ContractExternalLink string  `bson:"contract_external_link" json:"external_link"`
	ContractSchemaName   string  `bson:"contract_schema_name" json:"schema_name"`
	ContractSymbol       string  `bson:"contract_symbol" json:"symbol"`
	ContractTotalSupply  string  `bson:"contract_total_supply" json:"total_supply"`
}

// ContractCollectionNFT represents a contract within a collection nft
type ContractCollectionNFT struct {
	ContractName  string `bson:"contract_name" json:"name"`
	ContractImage string `bson:"contract_image_url" json:"image_url"`
}

// UpdateNFTInfoInput represents a MongoDB input to update the user defined info
// associated with a given NFT in the DB
type UpdateNFTInfoInput struct {
	LastUpdated LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote string `bson:"collectors_note"`
}

// NFTRepository represents the interface for interacting with persisted NFTs
type NFTRepository interface {
	CreateBulk(context.Context, []NFTDB) ([]DBID, error)
	Create(context.Context, NFTDB) (DBID, error)
	GetByUserID(context.Context, DBID) ([]NFT, error)
	GetByAddresses(context.Context, []Address) ([]NFT, error)
	GetByID(context.Context, DBID) (NFT, error)
	GetByContractData(context.Context, TokenID, Address) ([]NFT, error)
	GetByOpenseaID(context.Context, int, Address) ([]NFT, error)
	UpdateByID(context.Context, DBID, DBID, interface{}) error
	BulkUpsert(context.Context, DBID, []NFTDB) ([]DBID, error)
	OpenseaCacheGet(context.Context, []Address) ([]NFT, error)
	OpenseaCacheSet(context.Context, []Address, []NFT) error
	OpenseaCacheDelete(context.Context, []Address) error
}

// ErrNFTNotFoundByID is an error that occurs when an NFT is not found by its ID
type ErrNFTNotFoundByID struct {
	ID DBID
}

// ErrNFTNotFoundByContractData is an error that occurs when an NFT is not found by its contract data (token ID and contract address)
type ErrNFTNotFoundByContractData struct {
	TokenID, ContractAddress string
}

func (e ErrNFTNotFoundByID) Error() string {
	return fmt.Sprintf("could not find NFT with ID: %v", e.ID)
}

func (e ErrNFTNotFoundByContractData) Error() string {
	return fmt.Sprintf("could not find NFT with contract address %v and token ID %v", e.ContractAddress, e.TokenID)
}
