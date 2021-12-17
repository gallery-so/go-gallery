package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
)

var openseaAssetsTTL time.Duration = time.Minute * 5

var insertNFTsSQL = "INSERT INTO nfts(ID, DELETED, VERSION, NAME, DESCRIPTION, EXTERNAL_URL, CREATOR_ADDRESS, CREATOR_NAME, OWNER_ADDRESS, MULTIPLE_OWNERS, CONTRACT, OPENSEA_ID, OPENSEA_TOKEN_ID, IMAGE_URL, IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL, ANIMATION_URL, ANIMATION_ORIGINAL_URL) VALUES "
var insertNFTsValuesSQL = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

var excludedNFTSQL = "ID = EXCLUDED.ID, DELETED = EXCLUDED.DELETED, VERSION = EXCLUDED.VERSION, NAME = EXCLUDED.NAME, DESCRIPTION = EXCLUDED.DESCRIPTION, EXTERNAL_URL = EXCLUDED.EXTERNAL_URL, CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS, CREATOR_NAME = EXCLUDED.CREATOR_NAME, OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS, MULTIPLE_OWNERS EXCLUDED.MULTIPLE_OWNERS, CONTRACT = EXCLUDED.CONTRACT, OPENSEA_ID = EXCLUDED.OPENSEA_ID, OPENSEA_TOKEN_ID = EXCLUDED.OPENSEA_TOKEN_ID, IMAGE_URL = EXCLUDED.IMAGE_URL, IMAGE_THUMBNAIL_URL EXCLUDED.IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL = EXCLUDED.IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL = EXCLUDED.IMAGE_ORIGINAL_URL, ANIMATION_URL = EXCLUDED.ANIMATION_URL, ANIMATION_ORIGINAL_URL = EXCLUDED.ANIMATION_ORIGINAL_URL"

// NFTPostgresRepository is a repository that stores collections in a postgres database
type NFTPostgresRepository struct {
	db                   *sql.DB
	openseaCache         memstore.Cache
	nftsCache            memstore.Cache
	nftsCacheUpdateQueue *memstore.UpdateQueue
}

// NewNFTPostgresRepository creates a new persist.NFTPostgresRepository
func NewNFTPostgresRepository(db *sql.DB, openseaCache memstore.Cache, nftsCache memstore.Cache) persist.NFTRepository {
	return &NFTPostgresRepository{
		db:                   db,
		openseaCache:         openseaCache,
		nftsCache:            nftsCache,
		nftsCacheUpdateQueue: memstore.NewUpdateQueue(nftsCache),
	}
}

// CreateBulk creates many new NFTs in the database
func (n *NFTPostgresRepository) CreateBulk(pCtx context.Context, pNFTs []persist.NFTDB) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL
	vals := []interface{}{}

	for _, nft := range pNFTs {
		sqlStr += insertNFTsValuesSQL + ","
		vals = append(vals, nft.ID, nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, nft.Contract, nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL)
	}

	sqlStr = sqlStr[0 : len(sqlStr)-1]

	sqlStr += " RETURNING ID"

	res, err := n.db.QueryContext(pCtx, sqlStr, vals...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	resultIDs := make([]persist.DBID, len(pNFTs))

	for i := 0; res.Next(); i++ {
		var id string
		err = res.Scan(&id)
		if err != nil {
			return nil, err
		}

		resultIDs[i] = persist.DBID(id)
	}

	return resultIDs, nil
}

// Create creates a new NFT in the database
func (n *NFTPostgresRepository) Create(pCtx context.Context, pNFT persist.NFTDB) (persist.DBID, error) {
	sqlStr := insertNFTsSQL + insertNFTsValuesSQL + " RETURNING ID"

	res, err := n.db.QueryContext(pCtx, sqlStr, pNFT.ID, pNFT.Deleted, pNFT.Version, pNFT.Name, pNFT.Description, pNFT.ExternalURL, pNFT.CreatorAddress, pNFT.CreatorName, pNFT.OwnerAddress, pNFT.MultipleOwners, pNFT.Contract, pNFT.OpenseaID, pNFT.OpenseaTokenID, pNFT.ImageURL, pNFT.ImageThumbnailURL, pNFT.ImagePreviewURL, pNFT.ImageOriginalURL, pNFT.AnimationURL, pNFT.AnimationOriginalURL)
	if err != nil {
		return "", err
	}
	defer res.Close()

	var id string
	err = res.Scan(&id)
	if err != nil {
		return "", err
	}
	return persist.DBID(id), nil
}

// GetByUserID gets all NFTs for a user
func (n *NFTPostgresRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.NFT, error) {
	sqlStr := `SELECT addresses FROM users WHERE ID = $1`
	pAddresses := []persist.Address{}
	err := n.db.QueryRowContext(pCtx, sqlStr, pUserID).Scan(pq.Array(&pAddresses))
	if err != nil {
		return nil, err
	}
	return n.GetByAddresses(pCtx, pAddresses)
}

// GetByAddresses gets all NFTs owned by a set of addresses
func (n *NFTPostgresRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE OWNER_ADDRESS = ANY($1)`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pAddresses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, len(pAddresses)*25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	return nfts, nil
}

// GetByID gets a NFT by its ID
func (n *NFTPostgresRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE ID = $1`
	var nft persist.NFT
	err := n.db.QueryRowContext(pCtx, sqlStr, pID).Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL)
	if err != nil {
		return persist.NFT{}, err
	}
	return nft, nil
}

// GetByContractData gets a NFT by its contract data (contract address and token ID)
func (n *NFTPostgresRepository) GetByContractData(pCtx context.Context, pTokenID persist.TokenID, pContract persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE CONTRACT -> 'contract_address' = $1 AND OPENSEA_TOKEN_ID = $2`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pContract.String(), pTokenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	return nfts, nil
}

// GetByOpenseaID gets a NFT by its Opensea ID and owner address
func (n *NFTPostgresRepository) GetByOpenseaID(pCtx context.Context, pOpenseaID int, pWalletAddress persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE OPENSEA_ID = $1 AND OWNER_ADDRESS = $2`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pOpenseaID, pWalletAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	return nfts, nil
}

// UpdateByID updates a NFT by its ID
func (n *NFTPostgresRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	getUserSQLStr := `SELECT addresses FROM users WHERE ID = $1`
	var userAddresses []persist.Address
	err := n.db.QueryRowContext(pCtx, getUserSQLStr, pUserID).Scan(pq.Array(&userAddresses))
	if err != nil {
		return err
	}

	sqlStr := `UPDATE nfts `
	sqlStr += prepareSet(pUpdate)
	sqlStr += ` WHERE ID = $1 AND OWNER_ADDRESS = ANY($2)`
	_, err = n.db.ExecContext(pCtx, sqlStr, pID, userAddresses)
	if err != nil {
		return err
	}
	return nil
}

// BulkUpsert inserts or updates multiple NFTs
func (n *NFTPostgresRepository) BulkUpsert(pCtx context.Context, pUserID persist.DBID, pNFTs []persist.NFTDB) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL
	vals := []interface{}{}

	for _, nft := range pNFTs {
		sqlStr += insertNFTsValuesSQL + ","
		vals = append(vals, nft.ID, nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, nft.Contract, nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL)
	}

	sqlStr = sqlStr[0 : len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (OPENSEA_ID, OWNER_ADDRESS) DO UPDATE SET ` + excludedNFTSQL

	sqlStr += " RETURNING ID"

	res, err := n.db.QueryContext(pCtx, sqlStr, vals...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	resultIDs := make([]persist.DBID, len(pNFTs))

	for i := 0; res.Next(); i++ {
		var id string
		err = res.Scan(&id)
		if err != nil {
			return nil, err
		}

		resultIDs[i] = persist.DBID(id)
	}

	return resultIDs, nil
}

// OpenseaCacheSet adds a set of nfts to the opensea cache under a given set of wallet addresses as well as ensures
// that the nfts for user cache is most up to date
func (n *NFTPostgresRepository) OpenseaCacheSet(pCtx context.Context, pWalletAddresses []persist.Address, pNfts []persist.NFT) error {
	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	toCache, err := json.Marshal(pNfts)
	if err != nil {
		return err
	}

	return n.openseaCache.Set(pCtx, fmt.Sprint(pWalletAddresses), toCache, openseaAssetsTTL)
}

// OpenseaCacheDelete deletes a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTPostgresRepository) OpenseaCacheDelete(pCtx context.Context, pWalletAddresses []persist.Address) error {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	return n.openseaCache.Delete(pCtx, fmt.Sprint(pWalletAddresses))
}

// OpenseaCacheGet gets a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTPostgresRepository) OpenseaCacheGet(pCtx context.Context, pWalletAddresses []persist.Address) ([]persist.NFT, error) {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	result, err := n.openseaCache.Get(pCtx, fmt.Sprint(pWalletAddresses))
	if err != nil {
		return nil, err
	}

	nfts := []persist.NFT{}
	if err := json.Unmarshal([]byte(result), &nfts); err != nil {
		return nil, err
	}
	return nfts, nil
}
