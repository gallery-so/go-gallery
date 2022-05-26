package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

var openseaAssetsTTL time.Duration = time.Minute * 5

var insertNFTsSQL = `INSERT INTO nfts (ID, DELETED, VERSION, NAME, DESCRIPTION, EXTERNAL_URL, CREATOR_ADDRESS, CREATOR_NAME, OWNER_ADDRESS, MULTIPLE_OWNERS, CONTRACT, OPENSEA_ID, OPENSEA_TOKEN_ID, IMAGE_URL, IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL, ANIMATION_URL, ANIMATION_ORIGINAL_URL, TOKEN_COLLECTION_NAME, COLLECTORS_NOTE) VALUES `

// NFTRepository is a repository that stores collections in a postgres database
type NFTRepository struct {
	db                           *sql.DB
	galleryRepo                  *GalleryRepository
	createStmt                   *sql.Stmt
	getByAddressesStmt           *sql.Stmt
	getByIDStmt                  *sql.Stmt
	getByCollectionIDStmt        *sql.Stmt
	getByContractDataStmt        *sql.Stmt
	getByOpenseaIDStmt           *sql.Stmt
	getUserWalletsStmt           *sql.Stmt
	updateInfoStmt               *sql.Stmt
	updateOwnerAddressStmt       *sql.Stmt
	updateOwnerAddressUnsafeStmt *sql.Stmt
}

// NewNFTRepository creates a new persist.NFTPostgresRepository
func NewNFTRepository(db *sql.DB, galleryRepo *GalleryRepository) *NFTRepository {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, insertNFTsSQL+`($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) RETURNING ID;`)
	checkNoErr(err)

	getByAddressesStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,CREATED_AT,LAST_UPDATED,NAME,DESCRIPTION,EXTERNAL_URL,CREATOR_ADDRESS,CREATOR_NAME,OWNER_ADDRESS,MULTIPLE_OWNERS,CONTRACT,OPENSEA_ID,OPENSEA_TOKEN_ID,IMAGE_URL,IMAGE_THUMBNAIL_URL,IMAGE_PREVIEW_URL,IMAGE_ORIGINAL_URL,ANIMATION_URL,ANIMATION_ORIGINAL_URL,TOKEN_COLLECTION_NAME,COLLECTORS_NOTE FROM nfts WHERE OWNER_ADDRESS = ANY($1) AND DELETED = false;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,CREATED_AT,LAST_UPDATED,NAME,DESCRIPTION,EXTERNAL_URL,CREATOR_ADDRESS,CREATOR_NAME,OWNER_ADDRESS,MULTIPLE_OWNERS,CONTRACT,OPENSEA_ID,OPENSEA_TOKEN_ID,IMAGE_URL,IMAGE_THUMBNAIL_URL,IMAGE_PREVIEW_URL,IMAGE_ORIGINAL_URL,ANIMATION_URL,ANIMATION_ORIGINAL_URL,TOKEN_COLLECTION_NAME,COLLECTORS_NOTE FROM nfts WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getByCollectionIDStmt, err := db.PrepareContext(ctx, `SELECT n.ID,n.DELETED,n.VERSION,n.CREATED_AT,n.LAST_UPDATED,n.NAME,n.DESCRIPTION,n.EXTERNAL_URL,n.CREATOR_ADDRESS,n.CREATOR_NAME,n.OWNER_ADDRESS,n.MULTIPLE_OWNERS,n.CONTRACT,n.OPENSEA_ID,n.OPENSEA_TOKEN_ID,n.IMAGE_URL,n.IMAGE_THUMBNAIL_URL,n.IMAGE_PREVIEW_URL,n.IMAGE_ORIGINAL_URL,n.ANIMATION_URL,n.ANIMATION_ORIGINAL_URL,n.TOKEN_COLLECTION_NAME,n.COLLECTORS_NOTE FROM collections c, unnest(c.NFTS) WITH ORDINALITY AS u(nft_id, nft_ord) LEFT JOIN nfts n ON n.ID = nft_id WHERE c.ID = $1 AND c.DELETED = false AND n.DELETED = false ORDER BY nft_ord;`)
	checkNoErr(err)

	getByContractDataStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,CREATED_AT,LAST_UPDATED,NAME,DESCRIPTION,EXTERNAL_URL,CREATOR_ADDRESS,CREATOR_NAME,OWNER_ADDRESS,MULTIPLE_OWNERS,CONTRACT,OPENSEA_ID,OPENSEA_TOKEN_ID,IMAGE_URL,IMAGE_THUMBNAIL_URL,IMAGE_PREVIEW_URL,IMAGE_ORIGINAL_URL,ANIMATION_URL,ANIMATION_ORIGINAL_URL,TOKEN_COLLECTION_NAME,COLLECTORS_NOTE FROM nfts WHERE CONTRACT ->> 'address' = $1 AND OPENSEA_TOKEN_ID = $2 AND DELETED = false;`)
	checkNoErr(err)

	getByOpenseaStmt, err := db.PrepareContext(ctx, `SELECT ID,DELETED,VERSION,CREATED_AT,LAST_UPDATED,NAME,DESCRIPTION,EXTERNAL_URL,CREATOR_ADDRESS,CREATOR_NAME,OWNER_ADDRESS,MULTIPLE_OWNERS,CONTRACT,OPENSEA_ID,OPENSEA_TOKEN_ID,IMAGE_URL,IMAGE_THUMBNAIL_URL,IMAGE_PREVIEW_URL,IMAGE_ORIGINAL_URL,ANIMATION_URL,ANIMATION_ORIGINAL_URL,TOKEN_COLLECTION_NAME,COLLECTORS_NOTE FROM nfts WHERE OPENSEA_ID = $1 AND OWNER_ADDRESS = $2 AND DELETED = false;`)
	checkNoErr(err)

	getUserWalletsStmt, err := db.PrepareContext(ctx, `SELECT WALLETS FROM users WHERE ID = $1;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE nfts SET LAST_UPDATED = $1, COLLECTORS_NOTE = $2 WHERE ID = $3 AND OWNER_ADDRESS = ANY($4);`)
	checkNoErr(err)

	updateOwnerAddressStmt, err := db.PrepareContext(ctx, `UPDATE nfts SET OWNER_ADDRESS = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_ADDRESS = ANY($4);`)
	checkNoErr(err)

	updateOwnerAddressUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE nfts SET OWNER_ADDRESS = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	return &NFTRepository{
		db:                           db,
		galleryRepo:                  galleryRepo,
		createStmt:                   createStmt,
		getByAddressesStmt:           getByAddressesStmt,
		getByIDStmt:                  getByIDStmt,
		getByCollectionIDStmt:        getByCollectionIDStmt,
		getByContractDataStmt:        getByContractDataStmt,
		getByOpenseaIDStmt:           getByOpenseaStmt,
		getUserWalletsStmt:           getUserWalletsStmt,
		updateInfoStmt:               updateInfoStmt,
		updateOwnerAddressStmt:       updateOwnerAddressStmt,
		updateOwnerAddressUnsafeStmt: updateOwnerAddressUnsafeStmt,
	}
}

// CreateBulk creates many new NFTs in the database
func (n *NFTRepository) CreateBulk(pCtx context.Context, pNFTs []persist.NFT) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL
	vals := make([]interface{}, 0, len(pNFTs)*21)

	for i, nft := range pNFTs {
		sqlStr += generateValuesPlaceholders(21, i*21) + ","
		vals = append(vals, persist.GenerateID(), nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, nft.Contract, nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL, nft.TokenCollectionName, nft.CollectorsNote)
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
func (n *NFTRepository) Create(pCtx context.Context, pNFT persist.NFT) (persist.DBID, error) {

	var id persist.DBID
	err := n.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pNFT.Deleted, pNFT.Version, pNFT.Name, pNFT.Description, pNFT.ExternalURL, pNFT.CreatorAddress, pNFT.CreatorName, pNFT.OwnerAddress, pNFT.MultipleOwners, pNFT.Contract, pNFT.OpenseaID, pNFT.OpenseaTokenID, pNFT.ImageURL, pNFT.ImageThumbnailURL, pNFT.ImagePreviewURL, pNFT.ImageOriginalURL, pNFT.AnimationURL, pNFT.AnimationOriginalURL, pNFT.TokenCollectionName, pNFT.CollectorsNote).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByUserID gets all NFTs for a user
func (n *NFTRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]persist.NFT, error) {
	pAddresses := []persist.EthereumAddress{}
	err := n.getUserWalletsStmt.QueryRowContext(pCtx, pUserID).Scan(pq.Array(&pAddresses))
	if err != nil {
		return nil, err
	}
	return n.GetByAddresses(pCtx, pAddresses)
}

// GetByCollectionID gets all NFTs in a collection
func (n *NFTRepository) GetByCollectionID(pCtx context.Context, pCollectionID persist.DBID) ([]persist.NFT, error) {
	rows, err := n.getByCollectionIDStmt.QueryContext(pCtx, pCollectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
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

// GetByAddresses gets all NFTs owned by a set of addresses
func (n *NFTRepository) GetByAddresses(pCtx context.Context, pAddresses []persist.EthereumAddress) ([]persist.NFT, error) {
	rows, err := n.getByAddressesStmt.QueryContext(pCtx, pq.Array(pAddresses))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, len(pAddresses)*25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
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
	var nft persist.NFT
	err := n.getByIDStmt.QueryRowContext(pCtx, pID).Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
	if err != nil {
		if err == sql.ErrNoRows {
			return persist.NFT{}, persist.ErrNFTNotFoundByID{ID: pID}
		}
		return persist.NFT{}, err
	}
	return nft, nil
}

// GetByContractData gets a NFT by its contract data (contract address and token ID)
func (n *NFTRepository) GetByContractData(pCtx context.Context, pTokenID persist.TokenID, pContract persist.EthereumAddress) ([]persist.NFT, error) {
	rows, err := n.getByContractDataStmt.QueryContext(pCtx, pContract, pTokenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nfts := make([]persist.NFT, 0, 25)
	for rows.Next() {
		var nft persist.NFT
		err = rows.Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
		if err != nil {
			return nil, err
		}
		nfts = append(nfts, nft)
	}

	if err := rows.Err(); err != nil {
		if err == sql.ErrNoRows {
			return nil, persist.ErrNFTNotFoundByContractData{TokenID: pTokenID.String(), ContractAddress: pContract.String()}
		}
		return nil, err
	}

	return nfts, nil
}

// GetByOpenseaID gets a NFT by its Opensea ID and owner address
func (n *NFTRepository) GetByOpenseaID(pCtx context.Context, pOpenseaID persist.NullInt64, pWalletAddress persist.EthereumAddress) (persist.NFT, error) {
	var nft persist.NFT
	err := n.getByOpenseaIDStmt.QueryRowContext(pCtx, pOpenseaID, pWalletAddress).Scan(&nft.ID, &nft.Deleted, &nft.Version, &nft.CreationTime, &nft.LastUpdatedTime, &nft.Name, &nft.Description, &nft.ExternalURL, &nft.CreatorAddress, &nft.CreatorName, &nft.OwnerAddress, &nft.MultipleOwners, &nft.Contract, &nft.OpenseaID, &nft.OpenseaTokenID, &nft.ImageURL, &nft.ImageThumbnailURL, &nft.ImagePreviewURL, &nft.ImageOriginalURL, &nft.AnimationURL, &nft.AnimationOriginalURL, &nft.TokenCollectionName, &nft.CollectorsNote)
	if err != nil {
		if err == sql.ErrNoRows {
			// TODO custom error here
			return persist.NFT{}, fmt.Errorf("NFT not found by Opensea ID %d", pOpenseaID)
		}
		return persist.NFT{}, err
	}

	return nft, nil
}

// UpdateByID updates a NFT by its ID
func (n *NFTRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	var userAddresses []persist.EthereumAddress
	err := n.getUserWalletsStmt.QueryRowContext(pCtx, pUserID).Scan(pq.Array(&userAddresses))
	if err != nil {
		return err
	}

	switch pUpdate.(type) {
	case persist.NFTUpdateInfoInput:
		update := pUpdate.(persist.NFTUpdateInfoInput)
		it, err := n.updateInfoStmt.ExecContext(pCtx, time.Now(), update.CollectorsNote, pID, pq.Array(userAddresses))
		if err != nil {
			return err
		}
		rows, err := it.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return persist.ErrNFTNotFoundByID{ID: pID}
		}
	case persist.NFTUpdateOwnerAddressInput:
		update := pUpdate.(persist.NFTUpdateOwnerAddressInput)
		it, err := n.updateOwnerAddressStmt.ExecContext(pCtx, update.OwnerAddress, time.Now(), pID, pq.Array(userAddresses))
		if err != nil {
			return err
		}
		rows, err := it.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return persist.ErrNFTNotFoundByID{ID: pID}
		}

	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}

	return n.galleryRepo.RefreshCache(pCtx, pUserID)
}

// UpdateByIDUnsafe updates a NFT by its ID without ensure the NFT is owned by the user
func (n *NFTRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	switch pUpdate.(type) {
	case persist.NFTUpdateOwnerAddressInput:
		update := pUpdate.(persist.NFTUpdateOwnerAddressInput)
		it, err := n.updateOwnerAddressUnsafeStmt.ExecContext(pCtx, update.OwnerAddress, time.Now(), pID)
		if err != nil {
			return err
		}
		rows, err := it.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return persist.ErrNFTNotFoundByID{ID: pID}
		}

	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}

	return nil
}

// BulkUpsert inserts or updates multiple NFTs
func (n *NFTRepository) BulkUpsert(pCtx context.Context, pNFTs []persist.NFT) ([]persist.DBID, error) {
	sqlStr := insertNFTsSQL

	resultIDs := make([]persist.DBID, len(pNFTs))

	// Postgres only allows 65535 parameters at a time.
	// TODO: Consider trying this implementation at some point instead of chunking:
	//       https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit
	paramsPerRow := 21
	rowsPerQuery := 65535 / paramsPerRow

	if len(pNFTs) > rowsPerQuery {
		next := pNFTs[rowsPerQuery:]
		current := pNFTs[:rowsPerQuery]
		ids, err := n.BulkUpsert(pCtx, next)
		if err != nil {
			return nil, err
		}
		resultIDs = append(resultIDs, ids...)
		pNFTs = current
	}

	vals := make([]interface{}, 0, len(pNFTs)*paramsPerRow)

	for i, nft := range pNFTs {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, nft.ID, nft.Deleted, nft.Version, nft.Name, nft.Description, nft.ExternalURL, nft.CreatorAddress, nft.CreatorName, nft.OwnerAddress, nft.MultipleOwners, nft.Contract, nft.OpenseaID, nft.OpenseaTokenID, nft.ImageURL, nft.ImageThumbnailURL, nft.ImagePreviewURL, nft.ImageOriginalURL, nft.AnimationURL, nft.AnimationOriginalURL, nft.TokenCollectionName, nft.CollectorsNote)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (OPENSEA_ID, OWNER_ADDRESS) WHERE NOT DELETED DO UPDATE SET DELETED = EXCLUDED.DELETED, VERSION = EXCLUDED.VERSION, NAME = EXCLUDED.NAME, DESCRIPTION = EXCLUDED.DESCRIPTION, EXTERNAL_URL = EXCLUDED.EXTERNAL_URL, CREATOR_ADDRESS = EXCLUDED.CREATOR_ADDRESS, CREATOR_NAME = EXCLUDED.CREATOR_NAME, MULTIPLE_OWNERS = EXCLUDED.MULTIPLE_OWNERS, CONTRACT = EXCLUDED.CONTRACT, OPENSEA_TOKEN_ID = EXCLUDED.OPENSEA_TOKEN_ID, IMAGE_URL = EXCLUDED.IMAGE_URL, IMAGE_THUMBNAIL_URL = EXCLUDED.IMAGE_THUMBNAIL_URL, IMAGE_PREVIEW_URL = EXCLUDED.IMAGE_PREVIEW_URL, IMAGE_ORIGINAL_URL = EXCLUDED.IMAGE_ORIGINAL_URL, ANIMATION_URL = EXCLUDED.ANIMATION_URL, ANIMATION_ORIGINAL_URL = EXCLUDED.ANIMATION_ORIGINAL_URL, TOKEN_COLLECTION_NAME = EXCLUDED.TOKEN_COLLECTION_NAME RETURNING ID;`

	res, err := n.db.QueryContext(pCtx, sqlStr, vals...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

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
