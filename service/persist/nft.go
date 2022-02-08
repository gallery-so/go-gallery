package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// NFT represents an nft throughout the application
type NFT struct {
	Version         NullInt64       `bson:"version"              json:"version"` // schema version for this model
	ID              DBID            `bson:"_id"                  json:"id" binding:"required"`
	CreationTime    CreationTime    `bson:"created_at"        json:"created_at"`
	Deleted         NullBool        `bson:"deleted" json:"-"`
	LastUpdatedTime LastUpdatedTime `bson:"last_updated" json:"last_updated"`

	CollectorsNote NullString `bson:"collectors_note" json:"collectors_note"`

	// OwnerUsers     []*User  `bson:"owner_users" json:"owner_users"`
	OwnerAddress Address `bson:"owner_address" json:"owner_address"`

	MultipleOwners NullBool `bson:"multiple_owners" json:"multiple_owners"`

	Name                NullString  `bson:"name"                 json:"name"`
	Description         NullString  `bson:"description"          json:"description"`
	ExternalURL         NullString  `bson:"external_url"         json:"external_url"`
	TokenMetadataURL    NullString  `bson:"token_metadata_url" json:"token_metadata_url"`
	CreatorAddress      Address     `bson:"creator_address"      json:"creator_address"`
	CreatorName         NullString  `bson:"creator_name" json:"creator_name"`
	Contract            NFTContract `bson:"contract"     json:"asset_contract"`
	TokenCollectionName NullString  `bson:"token_collection_name" json:"token_collection_name"`

	OpenseaID NullInt64 `bson:"opensea_id"       json:"opensea_id"`
	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenseaTokenID TokenID `bson:"opensea_token_id" json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURL             NullString `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL    NullString `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL      NullString `bson:"image_preview_url"   json:"image_preview_url"`
	ImageOriginalURL     NullString `bson:"image_original_url" json:"image_original_url"`
	AnimationURL         NullString `bson:"animation_url" json:"animation_url"`
	AnimationOriginalURL NullString `bson:"animation_original_url" json:"animation_original_url"`

	AcquisitionDateStr NullString `bson:"acquisition_date" json:"acquisition_date"`
}

// CollectionNFT represents and NFT in a collection of NFTs
type CollectionNFT struct {
	ID           DBID         `bson:"_id"                  json:"id" binding:"required"`
	CreationTime CreationTime `bson:"created_at"        json:"created_at"`

	OwnerAddress Address `bson:"owner_address" json:"owner_address"`

	MultipleOwners NullBool `bson:"multiple_owners" json:"multiple_owners"`

	Name NullString `bson:"name"                 json:"name"`

	Contract            ContractCollectionNFT `bson:"contract"     json:"asset_contract"`
	TokenCollectionName NullString            `bson:"token_collection_name" json:"token_collection_name"`
	CreatorAddress      Address               `bson:"creator_address"      json:"creator_address"`
	CreatorName         NullString            `bson:"creator_name" json:"creator_name"`

	// IMAGES - OPENSEA
	ImageURL             NullString `bson:"image_url"           json:"image_url"`
	ImageThumbnailURL    NullString `bson:"image_thumbnail_url" json:"image_thumbnail_url"`
	ImagePreviewURL      NullString `bson:"image_preview_url"   json:"image_preview_url"`
	AnimationOriginalURL NullString `bson:"animation_original_url" json:"animation_original_url"`
}

// NFTContract represents a smart contract's information for a given NFT
type NFTContract struct {
	ContractAddress      Address    `bson:"contract_address"     json:"address"`
	ContractName         NullString `bson:"contract_name" json:"name"`
	ContractImage        NullString `bson:"contract_image_url" json:"image_url"`
	ContractDescription  NullString `bson:"contract_description" json:"description"`
	ContractExternalLink NullString `bson:"contract_external_link" json:"external_link"`
	ContractSchemaName   NullString `bson:"contract_schema_name" json:"schema_name"`
	ContractSymbol       NullString `bson:"contract_symbol" json:"symbol"`
	ContractTotalSupply  NullString `bson:"contract_total_supply" json:"total_supply"`
}

// ContractCollectionNFT represents a contract within a collection nft
type ContractCollectionNFT struct {
	ContractName  NullString `bson:"contract_name" json:"name"`
	ContractImage NullString `bson:"contract_image_url" json:"image_url"`
}

// NFTUpdateInfoInput represents a MongoDB input to update the user defined info
// associated with a given NFT in the DB
type NFTUpdateInfoInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	CollectorsNote NullString `json:"collectors_note"`
}

// NFTUpdateOwnerAddressInput represents an update to an NFTs owner address
type NFTUpdateOwnerAddressInput struct {
	LastUpdated LastUpdatedTime `json:"last_updated"`

	OwnerAddress Address `json:"owner_address"`
}

// NFTRepository represents the interface for interacting with persisted NFTs
type NFTRepository interface {
	CreateBulk(context.Context, []NFT) ([]DBID, error)
	Create(context.Context, NFT) (DBID, error)
	GetByUserID(context.Context, DBID) ([]NFT, error)
	GetByAddresses(context.Context, []Address) ([]NFT, error)
	GetByID(context.Context, DBID) (NFT, error)
	GetByContractData(context.Context, TokenID, Address) ([]NFT, error)
	GetByOpenseaID(context.Context, NullInt64, Address) ([]NFT, error)
	UpdateByID(context.Context, DBID, DBID, interface{}) error
	UpdateByIDUnsafe(context.Context, DBID, interface{}) error
	BulkUpsert(context.Context, DBID, []NFT) ([]DBID, error)
	OpenseaCacheGet(context.Context, []Address) ([]NFT, error)
	OpenseaCacheSet(context.Context, []Address, []NFT) error
	OpenseaCacheDelete(context.Context, []Address) error
}

// Value implements the driver.Valuer interface for the ContractCollectionNFT type
func (c ContractCollectionNFT) Value() (driver.Value, error) {
	return json.Marshal(c)

}

// Scan implements the sql.Scanner interface for the ContractCollectionNFT type
func (c *ContractCollectionNFT) Scan(src interface{}) error {
	if src == nil {
		*c = ContractCollectionNFT{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), c)
}

// Value implements the driver.Valuer interface for the NFTContract type
func (c NFTContract) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for the NFTContract type
func (c *NFTContract) Scan(src interface{}) error {
	if src == nil {
		*c = NFTContract{}
		return nil
	}
	return json.Unmarshal(src.([]uint8), c)
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
