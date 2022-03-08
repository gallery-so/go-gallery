package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// NFT represents an nft throughout the application
type NFT struct {
	Version         NullInt32       `json:"version"` // schema version for this model
	ID              DBID            `json:"id" binding:"required"`
	CreationTime    CreationTime    `json:"created_at"`
	Deleted         NullBool        `json:"-"`
	LastUpdatedTime LastUpdatedTime `json:"last_updated"`

	CollectorsNote NullString `json:"collectors_note"`

	// OwnerUsers     []*User  `bson:"owner_users" json:"owner_users"`
	OwnerAddress Address `json:"owner_address"`

	MultipleOwners NullBool `json:"multiple_owners"`

	Name                NullString  `json:"name"`
	Description         NullString  `json:"description"`
	ExternalURL         NullString  `json:"external_url"`
	TokenMetadataURL    NullString  `json:"token_metadata_url"`
	CreatorAddress      Address     `json:"creator_address"`
	CreatorName         NullString  `json:"creator_name"`
	Contract            NFTContract `json:"asset_contract"`
	TokenCollectionName NullString  `json:"token_collection_name"`

	OpenseaID NullInt64 `json:"opensea_id"`
	// OPEN_SEA_TOKEN_ID
	// https://api.opensea.io/api/v1/asset/0xa7d8d9ef8d8ce8992df33d8b8cf4aebabd5bd270/26000331
	// (/asset/:contract_address/:token_id)
	OpenseaTokenID TokenID `json:"opensea_token_id"`

	// IMAGES - OPENSEA
	ImageURL             NullString `json:"image_url"`
	ImageThumbnailURL    NullString `json:"image_thumbnail_url"`
	ImagePreviewURL      NullString `json:"image_preview_url"`
	ImageOriginalURL     NullString `json:"image_original_url"`
	AnimationURL         NullString `json:"animation_url"`
	AnimationOriginalURL NullString `json:"animation_original_url"`

	AcquisitionDateStr NullString `json:"acquisition_date"`
}

// CollectionNFT represents and NFT in a collection of NFTs
type CollectionNFT struct {
	ID           DBID         `json:"id" binding:"required"`
	CreationTime CreationTime `json:"created_at"`

	OwnerAddress Address `json:"owner_address"`

	MultipleOwners NullBool `json:"multiple_owners"`

	Name NullString `json:"name"`

	Contract            ContractCollectionNFT `json:"asset_contract"`
	TokenCollectionName NullString            `json:"token_collection_name"`
	CreatorAddress      Address               `json:"creator_address"`
	CreatorName         NullString            `json:"creator_name"`

	// IMAGES - OPENSEA
	ImageURL             NullString `json:"image_url"`
	ImageThumbnailURL    NullString `json:"image_thumbnail_url"`
	ImagePreviewURL      NullString `json:"image_preview_url"`
	AnimationOriginalURL NullString `json:"animation_original_url"`
	AnimationURL         NullString `json:"animation_url"`
}

// NFTContract represents a smart contract's information for a given NFT
type NFTContract struct {
	ContractAddress      Address    `json:"address"`
	ContractName         NullString `json:"name"`
	ContractImage        NullString `json:"image_url"`
	ContractDescription  NullString `json:"description"`
	ContractExternalLink NullString `json:"external_link"`
	ContractSchemaName   NullString `json:"schema_name"`
	ContractSymbol       NullString `json:"symbol"`
	ContractTotalSupply  NullString `json:"total_supply"`
}

// ContractCollectionNFT represents a contract within a collection nft
type ContractCollectionNFT struct {
	ContractName  NullString `json:"name"`
	ContractImage NullString `json:"image_url"`
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
	BulkUpsert(context.Context, []NFT) ([]DBID, error)
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
