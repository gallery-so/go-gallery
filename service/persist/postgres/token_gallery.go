package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// TokenGalleryRepository represents a postgres repository for tokens
type TokenGalleryRepository struct {
	db                                      *sql.DB
	galleryRepo                             *GalleryRepository
	createStmt                              *sql.Stmt
	getByUserIDStmt                         *sql.Stmt
	getByUserIDPaginateStmt                 *sql.Stmt
	getUserWalletsStmt                      *sql.Stmt
	getByContractStmt                       *sql.Stmt
	getByContractPaginateStmt               *sql.Stmt
	getByTokenIDStmt                        *sql.Stmt
	getByTokenIDPaginateStmt                *sql.Stmt
	getByTokenIdentifiersStmt               *sql.Stmt
	getByTokenIdentifiersPaginateStmt       *sql.Stmt
	getByIDStmt                             *sql.Stmt
	updateInfoStmt                          *sql.Stmt
	updateInfoUnsafeStmt                    *sql.Stmt
	updateMediaStmt                         *sql.Stmt
	updateMediaUnsafeStmt                   *sql.Stmt
	updateInfoByTokenIdentifiersUnsafeStmt  *sql.Stmt
	updateMediaByTokenIdentifiersUnsafeStmt *sql.Stmt
	mostRecentBlockStmt                     *sql.Stmt
	countTokensStmt                         *sql.Stmt
	upsertStmt                              *sql.Stmt
	deleteByIdentifiersStmt                 *sql.Stmt
	deleteByIDStmt                          *sql.Stmt
	getContractByAddressStmt                *sql.Stmt
}

// NewTokenGalleryRepository creates a new TokenRepository
// TODO joins on addresses
func NewTokenGalleryRepository(db *sql.DB, galleryRepo *GalleryRepository) *TokenGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,VERSION,COLLECTORS_NOTE,MEDIA,TOKEN_METADATA,TOKEN_TYPE,TOKEN_ID,CHAIN,NAME,DESCRIPTION,EXTERNAL_URL,BLOCK_NUMBER,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,CONTRACT) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING ID;`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE OWNER_USER_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByUserIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE OWNER_USER_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getUserWalletsStmt, err := db.PrepareContext(ctx, `SELECT WALLETS FROM users WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

	getByContractStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE CONTRACT = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByContractPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE CONTRACT = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE ID = $1;`)
	checkNoErr(err)

	updateInfoUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3;`)
	checkNoErr(err)

	updateMediaUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE ID = $5;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateMediaStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE ID = $5 AND OWNER_USER_ID = $6;`)
	checkNoErr(err)

	updateInfoByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateMediaByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE TOKEN_ID = $5 AND CONTRACT = $6 AND DELETED = false;`)
	checkNoErr(err)

	mostRecentBlockStmt, err := db.PrepareContext(ctx, `SELECT MAX(BLOCK_NUMBER) FROM tokens;`)
	checkNoErr(err)

	countTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM tokens;`)
	checkNoErr(err)

	upsertStmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) ON CONFLICT (TOKEN_ID,CONTRACT,CHAIN,OWNER_USER_ID) DO UPDATE SET MEDIA = $3,TOKEN_TYPE = $4,CHAIN = $5,NAME = $6,DESCRIPTION = $7,TOKEN_URI = $9,QUANTITY = $10, OWNED_BY_WALLETS = $12,OWNERSHIP_HISTORY = $13,TOKEN_METADATA = $14,EXTERNAL_URL = $16,BLOCK_NUMBER = $17,VERSION = $18,CREATED_AT = $19,LAST_UPDATED = $20;`)
	checkNoErr(err)

	deleteByIdentifiersStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND OWNER_USER_ID = $3 AND CHAIN = $4;`)
	checkNoErr(err)

	deleteByIDStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	getContractByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	return &TokenGalleryRepository{
		db:                                      db,
		galleryRepo:                             galleryRepo,
		createStmt:                              createStmt,
		getByUserIDStmt:                         getByUserIDStmt,
		getByUserIDPaginateStmt:                 getByUserIDPaginateStmt,
		getUserWalletsStmt:                      getUserWalletsStmt,
		getByContractStmt:                       getByContractStmt,
		getByContractPaginateStmt:               getByContractPaginateStmt,
		getByTokenIdentifiersStmt:               getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt:       getByTokenIdentifiersPaginateStmt,
		getByIDStmt:                             getByIDStmt,
		updateInfoUnsafeStmt:                    updateInfoUnsafeStmt,
		updateMediaUnsafeStmt:                   updateMediaUnsafeStmt,
		updateInfoStmt:                          updateInfoStmt,
		updateMediaStmt:                         updateMediaStmt,
		updateInfoByTokenIdentifiersUnsafeStmt:  updateInfoByTokenIdentifiersUnsafeStmt,
		updateMediaByTokenIdentifiersUnsafeStmt: updateMediaByTokenIdentifiersUnsafeStmt,
		mostRecentBlockStmt:                     mostRecentBlockStmt,
		countTokensStmt:                         countTokensStmt,
		upsertStmt:                              upsertStmt,
		deleteByIdentifiersStmt:                 deleteByIdentifiersStmt,
		getByTokenIDStmt:                        getByTokenIDStmt,
		getByTokenIDPaginateStmt:                getByTokenIDPaginateStmt,
		deleteByIDStmt:                          deleteByIDStmt,
		getContractByAddressStmt:                getContractByAddressStmt,
	}

}

// CreateBulk creates many tokens in the database
func (t *TokenGalleryRepository) CreateBulk(pCtx context.Context, pTokens []persist.TokenGallery) ([]persist.DBID, error) {
	insertSQL := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*18)
	for i, token := range pTokens {
		insertSQL += generateValuesPlaceholders(18, i*18) + ","
		vals = append(vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnedByWallets, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.Contract, token.ExternalURL, token.BlockNumber, token.Version)
	}
	insertSQL = insertSQL[:len(insertSQL)-1]
	insertSQL += " RETURNING ID"

	rows, err := t.db.QueryContext(pCtx, insertSQL, vals...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]persist.DBID, 0, len(pTokens))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, persist.DBID(id))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil

}

// Create creates a token in the database
func (t *TokenGalleryRepository) Create(pCtx context.Context, pToken persist.TokenGallery) (persist.DBID, error) {

	var id persist.DBID
	err := t.createStmt.QueryRowContext(pCtx, persist.GenerateID(), pToken.Version, pToken.CollectorsNote, pToken.Media, pToken.TokenMetadata, pToken.TokenType, pToken.TokenID, pToken.Chain, pToken.Name, pToken.Description, pToken.ExternalURL, pToken.BlockNumber, pToken.TokenURI, pToken.Quantity, pToken.OwnerUserID, pToken.OwnedByWallets, pq.Array(pToken.OwnershipHistory), pToken.Contract).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByUserID gets all tokens for a user
func (t *TokenGalleryRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, limit int64, page int64) ([]persist.TokenGallery, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByUserIDPaginateStmt.QueryContext(pCtx, pUserID, limit, page*limit)
	} else {
		rows, err = t.getByUserIDStmt.QueryContext(pCtx, pUserID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil

}

// GetByContract retrieves all tokens associated with a contract
func (t *TokenGalleryRepository) GetByContract(pCtx context.Context, pContractAddress persist.Address, pChain persist.Chain, limit int64, page int64) ([]persist.TokenGallery, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByContractPaginateStmt.QueryContext(pCtx, pContractAddress, pChain, limit, page*limit)
	} else {
		rows, err = t.getByContractStmt.QueryContext(pCtx, pContractAddress, pChain)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokensGalleryNotFoundByContract{ContractAddress: pContractAddress, Chain: pChain}
	}

	return tokens, nil
}

// GetByTokenIdentifiers gets a token by its token ID and contract address
func (t *TokenGalleryRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, limit int64, page int64) ([]persist.TokenGallery, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByTokenIdentifiersPaginateStmt.QueryContext(pCtx, pTokenID, pContractAddress, pChain, limit, page*limit)
	} else {
		rows, err = t.getByTokenIdentifiersStmt.QueryContext(pCtx, pTokenID, pContractAddress, pChain)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokenGalleryNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress, Chain: pChain}
	}

	return tokens, nil
}

// GetByTokenID retrieves all tokens associated with a contract
func (t *TokenGalleryRepository) GetByTokenID(pCtx context.Context, pTokenID persist.TokenID, limit int64, page int64) ([]persist.TokenGallery, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByTokenIDPaginateStmt.QueryContext(pCtx, pTokenID, limit, page*limit)
	} else {
		rows, err = t.getByTokenIDStmt.QueryContext(pCtx, pTokenID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokensNotFoundByTokenID{TokenID: pTokenID}
	}

	return tokens, nil
}

// GetByID gets a token by its ID
func (t *TokenGalleryRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.TokenGallery, error) {
	token := persist.TokenGallery{}
	err := t.getByIDStmt.QueryRowContext(pCtx, pID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated)
	if err != nil {
		return persist.TokenGallery{}, err
	}
	return token, nil
}

// BulkUpsert upserts multiple tokens
func (t *TokenGalleryRepository) BulkUpsert(pCtx context.Context, pTokens []persist.TokenGallery) error {
	if len(pTokens) == 0 {
		return nil
	}

	logrus.Infof("Checking 0 quantities for tokens...")
	newTokens := make([]persist.TokenGallery, len(pTokens))
	for i, token := range pTokens {
		if token.Quantity == "" || token.Quantity == "0" {
			if err := t.deleteTokenUnsafe(pCtx, token.TokenID, token.Contract, token.OwnerUserID, token.Chain); err != nil {
				return err
			}
			continue
		}
		newTokens[i] = token
	}

	if len(newTokens) == 0 {
		return nil
	}

	// Postgres only allows 65535 parameters at a time.
	// TODO: Consider trying this implementation at some point instead of chunking:
	//       https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit
	paramsPerRow := 20
	rowsPerQuery := 65535 / paramsPerRow

	if len(newTokens) > rowsPerQuery {
		logrus.Debugf("Chunking %d tokens recursively into %d queries", len(newTokens), len(newTokens)/rowsPerQuery)
		next := newTokens[rowsPerQuery:]
		current := newTokens[:rowsPerQuery]
		if err := t.BulkUpsert(pCtx, next); err != nil {
			return err
		}
		newTokens = current
	}

	newTokens = t.dedupeTokens(newTokens)

	sqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	vals := make([]interface{}, 0, len(newTokens)*paramsPerRow)
	for i, token := range newTokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnedByWallets, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.Contract, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT,CHAIN,OWNER_USER_ID) WHERE DELETED = false DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_USER_ID = EXCLUDED.OWNER_USER_ID,OWNED_BY_WALLETS = EXCLUDED.OWNED_BY_WALLETS,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,LAST_UPDATED = EXCLUDED.LAST_UPDATED;`

	_, err := t.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		logrus.Debugf("SQL: %s", sqlStr)
		return fmt.Errorf("failed to upsert tokens: %w", err)
	}

	return nil

}

// Upsert upserts a token by its token ID and contract address and if its token type is ERC-1155 it also upserts using the owner address
func (t *TokenGalleryRepository) Upsert(pCtx context.Context, pToken persist.TokenGallery) error {
	var err error
	if pToken.Quantity == "0" {
		_, err = t.deleteByIdentifiersStmt.ExecContext(pCtx, pToken.TokenID, pToken.Contract, pToken.OwnerUserID, pToken.Chain)
	} else {
		_, err = t.upsertStmt.ExecContext(pCtx, persist.GenerateID(), pToken.CollectorsNote, pToken.Media, pToken.TokenType, pToken.Chain, pToken.Name, pToken.Description, pToken.TokenID, pToken.TokenURI, pToken.Quantity, pToken.OwnerUserID, pToken.OwnedByWallets, pToken.OwnershipHistory, pToken.TokenMetadata, pToken.Contract, pToken.ExternalURL, pToken.BlockNumber, pToken.Version, pToken.CreationTime, pToken.LastUpdated)
	}
	return err
}

// UpdateByIDUnsafe updates a token by its ID without checking if it is owned by any given user
func (t *TokenGalleryRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		res, err = t.updateInfoUnsafeStmt.ExecContext(pCtx, update.CollectorsNote, update.LastUpdated, pID)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		res, err = t.updateMediaUnsafeStmt.ExecContext(pCtx, update.Media, update.TokenURI, update.Metadata, update.LastUpdated, pID)
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrTokenNotFoundByID{ID: pID}
	}
	return nil
}

// UpdateByID updates a token by its ID
func (t *TokenGalleryRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error

	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		res, err = t.updateInfoStmt.ExecContext(pCtx, update.CollectorsNote, update.LastUpdated, pID, pUserID)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		res, err = t.updateMediaStmt.ExecContext(pCtx, update.Media, update.TokenURI, update.Metadata, update.LastUpdated, pID, pUserID)
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return persist.ErrTokenNotFoundByID{ID: pID}
	}
	return t.galleryRepo.RefreshCache(pCtx, pUserID)
}

// UpdateByTokenIdentifiersUnsafe updates a token by its token identifiers without checking if it is owned by any given user
func (t *TokenGalleryRepository) UpdateByTokenIdentifiersUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, pUpdate interface{}) error {

	var contractID persist.DBID
	if err := t.getContractByAddressStmt.QueryRowContext(pCtx, pContractAddress, pChain).Scan(&contractID); err != nil {
		return err
	}

	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		res, err = t.updateInfoByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.CollectorsNote, update.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		res, err = t.updateMediaByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Media, update.TokenURI, update.Metadata, update.LastUpdated, pTokenID, contractID)
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrTokenGalleryNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress, Chain: pChain}
	}
	return nil
}

// MostRecentBlock returns the most recent block number of any token
func (t *TokenGalleryRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {
	var blockNumber persist.BlockNumber
	err := t.mostRecentBlockStmt.QueryRowContext(pCtx).Scan(&blockNumber)
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

// Count returns the number of tokens in the database
func (t *TokenGalleryRepository) Count(pCtx context.Context, pTokenType persist.TokenCountType) (int64, error) {
	var count int64
	err := t.countTokensStmt.QueryRowContext(pCtx).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// DeleteByID deletes a token by its ID
func (t *TokenGalleryRepository) DeleteByID(ctx context.Context, id persist.DBID) error {
	_, err := t.deleteByIDStmt.ExecContext(ctx, id)
	return err
}

func (t *TokenGalleryRepository) deleteTokenUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.DBID, pOwnerUserID persist.DBID, pChain persist.Chain) error {
	_, err := t.deleteByIdentifiersStmt.ExecContext(pCtx, pTokenID, pContractAddress, pOwnerUserID, pChain)
	return err
}

type uniqueConstraintKey struct {
	tokenID     persist.TokenID
	contract    persist.DBID
	chain       persist.Chain
	ownerUserID persist.DBID
}

func (t *TokenGalleryRepository) dedupeTokens(pTokens []persist.TokenGallery) []persist.TokenGallery {
	seen := map[uniqueConstraintKey]persist.TokenGallery{}
	for _, token := range pTokens {
		key := uniqueConstraintKey{chain: token.Chain, contract: token.Contract, tokenID: token.TokenID, ownerUserID: token.OwnerUserID}
		if seenToken, ok := seen[key]; ok {
			if seenToken.BlockNumber.Uint64() > token.BlockNumber.Uint64() {
				continue
			}
			seen[key] = token
		}
		seen[key] = token
	}
	result := make([]persist.TokenGallery, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}
