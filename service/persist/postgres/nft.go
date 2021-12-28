package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/memstore"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

var openseaAssetsTTL time.Duration = time.Minute * 5

var insertNFTsSQL = `INSERT INTO nfts (ID, DELETED, VERSION, NAME, DESCRIPTION, EXTERNAL_URL, CREATOR_ADDRESS, CREATOR_NAME, OWNER_ADDRESS, MULTIPLE_OWNERS, CONTRACT, OPENSEA_ID, OPENSEA_TOKEN_ID, IMAGE_URL, IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL, ANIMATION_URL, ANIMATION_ORIGINAL_URL, TOKEN_COLLECTION_NAME) VALUES `

var excludedNFTSQL = `ID = EXCLUDED.ID, DELETED = EXCLUDED.DELETED, VERSION = EXCLUDED.VERSION, NAME = EXCLUDED.NAME, DESCRIPTION = EXCLUDED.DESCRIPTION, EXTERNAL_URL = EXCLUDED.EXTERNAL_URL, CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS, CREATOR_NAME = EXCLUDED.CREATOR_NAME, OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS, MULTIPLE_OWNERS EXCLUDED.MULTIPLE_OWNERS, string(contract) = EXCLUDED.CONTRACT, OPENSEA_ID = EXCLUDED.OPENSEA_ID, OPENSEA_TOKEN_ID = EXCLUDED.OPENSEA_TOKEN_ID, IMAGE_URL = EXCLUDED.IMAGE_URL, IMAGE_THUMBNAIL_URL EXCLUDED.IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL = EXCLUDED.IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL = EXCLUDED.IMAGE_ORIGINAL_URL, ANIMATION_URL = EXCLUDED.ANIMATION_URL, ANIMATION_ORIGINAL_URL = EXCLUDED.ANIMATION_ORIGINAL_URL, TOKEN_COLLECTION_NAME = EXCLUDED.TOKEN_COLLECTION_NAME`

// NFTRepository is a repository that stores collections in a postgres database
type NFTRepository struct {
	db                   *sql.DB
	openseaCache         memstore.Cache
	nftsCache            memstore.Cache
	nftsCacheUpdateQueue *memstore.UpdateQueue
}

// NewNFTRepository creates a new persist.NFTPostgresRepository
func NewNFTRepository(db *sql.DB, openseaCache memstore.Cache, nftsCache memstore.Cache) *NFTRepository {
	return &NFTRepository{
		db:                   db,
		openseaCache:         openseaCache,
		nftsCache:            nftsCache,
		nftsCacheUpdateQueue: memstore.NewUpdateQueue(nftsCache),
	}
}

// CreateBulk creates many new NFTs in the database
func (n *NFTRepository) CreateBulk(pCtx context.Context, pNFTs []persist.NFTDB) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL
	vals := make([]interface{}, 0, len(pNFTs)*20)

	for i, nft := range pNFTs {
		sqlStr += generateValuesPlaceholders(20, i*20) + ","
		contract, err := json.Marshal(nft.Contract)
		if err != nil {
			return nil, err
		}
		vals = append(vals, persist.GenerateID(), nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, string(contract), nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL, nft.TokenCollectionName)
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

	if err := res.Err(); err != nil {
		return nil, err
	}

	return resultIDs, nil
}

// Create creates a new NFT in the database
func (n *NFTRepository) Create(pCtx context.Context, pNFT persist.NFTDB) (persist.DBID, error) {
	sqlStr := insertNFTsSQL + `($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)` + " RETURNING ID"

	logrus.Infof("Create NFT: %s", sqlStr)
	contract, err := json.Marshal(pNFT.Contract)
	if err != nil {
		return "", err
	}
	var id string
	err = n.db.QueryRowContext(pCtx, sqlStr, persist.GenerateID(), pNFT.Deleted, pNFT.Version, pNFT.Name, pNFT.Description, pNFT.ExternalURL, pNFT.CreatorAddress, pNFT.CreatorName, pNFT.OwnerAddress, pNFT.MultipleOwners, string(contract), pNFT.OpenseaID, pNFT.OpenseaTokenID, pNFT.ImageURL, pNFT.ImageThumbnailURL, pNFT.ImagePreviewURL, pNFT.ImageOriginalURL, pNFT.AnimationURL, pNFT.AnimationOriginalURL, pNFT.TokenCollectionName).Scan(&id)
	if err != nil {
		return "", err
	}

	return persist.DBID(id), nil
}

// GetByUserID gets all NFTs for a user
func (n *NFTRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.NFT, error) {
	sqlStr := `SELECT addresses FROM users WHERE ID = $1`
	pAddresses := []persist.Address{}
	err := n.db.QueryRowContext(pCtx, sqlStr, pUserID).Scan(pq.Array(&pAddresses))
	if err != nil {
		return nil, err
	}
	return n.GetByAddresses(pCtx, pAddresses)
}

// GetByAddresses gets all NFTs owned by a set of addresses
func (n *NFTRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE OWNER_ADDRESS = ANY($1)`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pAddresses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, len(pAddresses)*25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.CreationTime, &nft.LastUpdatedTime, &nft.TokenCollectionName)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nfts, nil
}

// GetByID gets a NFT by its ID
func (n *NFTRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE ID = $1`
	var nft persist.NFT
	err := n.db.QueryRowContext(pCtx, sqlStr, pID).Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.CreationTime, &nft.LastUpdatedTime, &nft.TokenCollectionName)
	if err != nil {
		return persist.NFT{}, err
	}
	return nft, nil
}

// GetByContractData gets a NFT by its contract data (contract address and token ID)
func (n *NFTRepository) GetByContractData(pCtx context.Context, pTokenID persist.TokenID, pContract persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE CONTRACT -> 'contract_address' = $1 AND OPENSEA_TOKEN_ID = $2`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pContract.String(), pTokenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.CreationTime, &nft.LastUpdatedTime, &nft.TokenCollectionName)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nfts, nil
}

// GetByOpenseaID gets a NFT by its Opensea ID and owner address
func (n *NFTRepository) GetByOpenseaID(pCtx context.Context, pOpenseaID int, pWalletAddress persist.Address) ([]persist.NFT, error) {
	sqlStr := `SELECT * FROM nfts WHERE OPENSEA_ID = $1 AND OWNER_ADDRESS = $2`
	rows, err := n.db.QueryContext(pCtx, sqlStr, pOpenseaID, pWalletAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.CreationTime, &nft.LastUpdatedTime, &nft.TokenCollectionName)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nfts, nil
}

// UpdateByID updates a NFT by its ID
func (n *NFTRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	getUserSQLStr := `SELECT addresses FROM users WHERE ID = $1`
	var userAddresses []persist.Address
	err := n.db.QueryRowContext(pCtx, getUserSQLStr, pUserID).Scan(pq.Array(&userAddresses))
	if err != nil {
		return err
	}

	sqlStr := `UPDATE nfts `
	switch pUpdate.(type) {
	case persist.NFTUpdateInfoInput:
		update := pUpdate.(persist.NFTUpdateInfoInput)
		sqlStr += `SET LAST_UPDATED = $1, COLLECTORS_NOTE = $3 WHERE ID = $2 AND OWNER_ADDRESS = ANY($4)`
		_, err = n.db.ExecContext(pCtx, sqlStr, time.Now(), pID, update.CollectorsNote, pq.Array(userAddresses))
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported update type")
	}

	return nil
}

// BulkUpsert inserts or updates multiple NFTs
func (n *NFTRepository) BulkUpsert(pCtx context.Context, pUserID persist.DBID, pNFTs []persist.NFTDB) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL
	vals := make([]interface{}, 0, len(pNFTs)*20)

	for i, nft := range pNFTs {
		sqlStr += generateValuesPlaceholders(20, i*20) + ","
		vals = append(vals, persist.GenerateID(), nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, nft.Contract, nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL, nft.TokenCollectionName)
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

	if err := res.Err(); err != nil {
		return nil, err
	}

	return resultIDs, nil
}

// OpenseaCacheSet adds a set of nfts to the opensea cache under a given set of wallet addresses as well as ensures
// that the nfts for user cache is most up to date
func (n *NFTRepository) OpenseaCacheSet(pCtx context.Context, pWalletAddresses []persist.Address, pNfts []persist.NFT) error {
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
func (n *NFTRepository) OpenseaCacheDelete(pCtx context.Context, pWalletAddresses []persist.Address) error {

	for i, v := range pWalletAddresses {
		pWalletAddresses[i] = v
	}

	return n.openseaCache.Delete(pCtx, fmt.Sprint(pWalletAddresses))
}

// OpenseaCacheGet gets a set of nfts from the opensea cache under a given set of wallet addresses
func (n *NFTRepository) OpenseaCacheGet(pCtx context.Context, pWalletAddresses []persist.Address) ([]persist.NFT, error) {

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
