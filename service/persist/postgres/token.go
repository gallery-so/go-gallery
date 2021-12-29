package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenRepository represents a postgres repository for tokens
type TokenRepository struct {
	db *sql.DB
}

// NewTokenRepository creates a new TokenRepository
func NewTokenRepository(db *sql.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

// CreateBulk creates many tokens in the database
func (t *TokenRepository) CreateBulk(pCtx context.Context, pTokens []persist.Token) ([]persist.DBID, error) {
	insertSQL := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*17)
	for i, token := range pTokens {
		insertSQL += generateValuesPlaceholders(17, i*17) + ","

		vals = append(vals, token.ID, token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version)
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
func (t *TokenRepository) Create(pCtx context.Context, pToken persist.Token) (persist.DBID, error) {

	insertSQL := `INSERT INTO tokens (ID,VERSION,COLLECTORS_NOTE,MEDIA,TOKEN_METADATA,TOKEN_TYPE,TOKEN_ID,CHAIN,NAME,DESCRIPTION,EXTERNAL_URL,BLOCK_NUMBER,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,CONTRACT_ADDRESS) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING ID`

	var id persist.DBID
	err := t.db.QueryRowContext(pCtx, insertSQL, pToken.ID, pToken.Version, pToken.CollectorsNote, pToken.Media, pToken.TokenMetadata, pToken.TokenType, pToken.TokenID, pToken.Chain, pToken.Name, pToken.Description, pToken.ExternalURL, pToken.BlockNumber, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pq.Array(pToken.OwnershipHistory), pToken.ContractAddress).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByWallet retrieves all tokens associated with a wallet
func (t *TokenRepository) GetByWallet(pCtx context.Context, pAddress persist.Address, limit int64, page int64) ([]persist.Token, error) {
	sqlStr := `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE OWNER_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC`
	if limit > 0 {
		sqlStr += " LIMIT " + fmt.Sprint(limit)
		sqlStr += " OFFSET " + fmt.Sprint(page)
	}
	rows, err := t.db.QueryContext(pCtx, sqlStr, pAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil

}

// GetByUserID retrieves all tokens associated with a user
func (t *TokenRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, limit int64, page int64) ([]persist.Token, error) {
	getUserSQLStr := `SELECT ADDRESSES FROM users WHERE ID = $1 AND DELETED = false`
	addresses := make([]persist.Address, 0, 10)
	err := t.db.QueryRowContext(pCtx, getUserSQLStr, pUserID).Scan(pq.Array(&addresses))
	if err != nil {
		return nil, err
	}
	tokens := make([]persist.Token, 0, 10)
	for i, address := range addresses {
		t, err := t.GetByWallet(pCtx, address, limit, int64(i))
		if err != nil {
			return nil, err
		}
		if len(t) == int(limit) {
			t = t[:limit]
		}

		tokens = append(tokens, t...)
	}
	return tokens, nil
}

// GetByContract retrieves all tokens associated with a contract
func (t *TokenRepository) GetByContract(pCtx context.Context, pContractAddress persist.Address, limit int64, page int64) ([]persist.Token, error) {
	sqlStr := `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE CONTRACT_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC`
	if limit > 0 {
		sqlStr += " LIMIT " + fmt.Sprint(limit)
		sqlStr += " OFFSET " + fmt.Sprint(page)
	}
	rows, err := t.db.QueryContext(pCtx, sqlStr, pContractAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetByTokenIdentifiers gets a token by its token ID and contract address
func (t *TokenRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, limit int64, page int64) ([]persist.Token, error) {
	sqlStr := `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC`
	if limit > 0 {
		sqlStr += " LIMIT " + fmt.Sprint(limit)
		sqlStr += " OFFSET " + fmt.Sprint(page)
	}
	rows, err := t.db.QueryContext(pCtx, sqlStr, pTokenID, pContractAddress)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokenNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress}
	}

	return tokens, nil
}

// GetByID gets a token by its ID
func (t *TokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Token, error) {
	sqlStr := `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE ID = $1`
	token := persist.Token{}
	err := t.db.QueryRowContext(pCtx, sqlStr, pID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated)
	if err != nil {
		return persist.Token{}, err
	}
	return token, nil
}

// BulkUpsert upserts multiple tokens
func (t *TokenRepository) BulkUpsert(pCtx context.Context, pTokens []persist.Token) error {
	erc721s := make([]persist.Token, 0, len(pTokens))
	erc1155s := make([]persist.Token, 0, len(pTokens))
	for _, token := range pTokens {
		if token.TokenType == persist.TokenTypeERC721 {
			erc721s = append(erc721s, token)
		}
		if token.TokenType == persist.TokenTypeERC1155 {
			erc1155s = append(erc1155s, token)
		}
	}
	erc721SqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	erc721Vals := make([]interface{}, 0, len(erc721s)*17)
	for i, token := range erc721s {
		erc721SqlStr += generateValuesPlaceholders(17, i*17)
		erc721Vals = append(erc721Vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, token.OwnershipHistory, token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	erc721SqlStr = erc721SqlStr[:len(erc721SqlStr)-1]

	erc1155SqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	erc1155Vals := make([]interface{}, 0, len(erc1155s)*17)
	for i, token := range erc1155s {
		erc1155SqlStr += generateValuesPlaceholders(17, i*17)
		erc1155Vals = append(erc1155Vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, token.OwnershipHistory, token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	erc1155SqlStr = erc1155SqlStr[:len(erc1155SqlStr)-1]

	erc721SqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS) DO UPDATE SET COLLECTORS_NOTE = EXCLUDED.COLLECTORS_NOTE,MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED WHERE EXCLUDED.BLOCK_NUMBER > BLOCK_NUMBER`

	erc1155SqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_ADDRESS) DO UPDATE SET COLLECTORS_NOTE = EXCLUDED.COLLECTORS_NOTE,MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED WHERE EXCLUDED.BLOCK_NUMBER > BLOCK_NUMBER`

	_, err := t.db.ExecContext(pCtx, erc721SqlStr, erc721Vals...)
	if err != nil {
		return err
	}

	_, err = t.db.ExecContext(pCtx, erc1155SqlStr, erc1155Vals...)
	if err != nil {
		return err
	}

	return nil

}

// Upsert upserts a token by its token ID and contract address and if its token type is ERC-1155 it also upserts using the owner address
func (t *TokenRepository) Upsert(pCtx context.Context, pToken persist.Token) error {
	conflict := "(TOKEN_ID,CONTRACT_ADDRESS"
	if pToken.TokenType == persist.TokenTypeERC1155 {
		conflict += ",OWNER_ADDRESS"
	}
	conflict += ")"
	sqlStr := fmt.Sprintf(`INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) ON CONFLICT %s DO UPDATE SET COLLECTORS_NOTE = $2,MEDIA = $3,TOKEN_TYPE = $4,CHAIN = $5,NAME = $6,DESCRIPTION = $7,TOKEN_URI = $9,QUANTITY = $10,OWNER_ADDRESS = $11,OWNERSHIP_HISTORY = $12,TOKEN_METADATA = $13,EXTERNAL_URL = $15,BLOCK_NUMBER = $16,VERSION = $17,CREATED_AT = $18,LAST_UPDATED = $19`, conflict)
	_, err := t.db.ExecContext(pCtx, sqlStr, persist.GenerateID(), pToken.CollectorsNote, pToken.Media, pToken.TokenType, pToken.Chain, pToken.Name, pToken.Description, pToken.TokenID, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pToken.OwnershipHistory, pToken.TokenMetadata, pToken.ContractAddress, pToken.ExternalURL, pToken.BlockNumber, pToken.Version, pToken.CreationTime, pToken.LastUpdated)
	return err
}

// UpdateByIDUnsafe updates a token by its ID without checking if it is owned by any given user
func (t *TokenRepository) UpdateByIDUnsafe(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	sqlStr := `UPDATE tokens SET `
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		sqlStr += "COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3"
		res, err = t.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.LastUpdated, pID)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		sqlStr += "MEDIA = $1, LAST_UPDATED = $2 WHERE ID = $3"
		res, err = t.db.ExecContext(pCtx, sqlStr, update.Media, update.LastUpdated, pID)
	default:
		return errors.New("unsupported update type")
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
func (t *TokenRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	getUserSQL := `SELECT ADDRESSES FROM users WHERE ID = $1`
	var addresses []persist.Address
	err := t.db.QueryRowContext(pCtx, getUserSQL, pUserID).Scan(pq.Array(&addresses))
	if err != nil {
		return err
	}

	sqlStr := `UPDATE tokens `
	var res sql.Result
	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		sqlStr += `SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_ADDRESS = ANY($4)`
		res, err = t.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.LastUpdated, pID, pq.Array(addresses))
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		sqlStr += `SET MEDIA = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_ADDRESS = ANY($4)`
		res, err = t.db.ExecContext(pCtx, sqlStr, update.Media, update.LastUpdated, pID, pq.Array(addresses))
	default:
		return errors.New("unsupported update type")
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
	return nil
}

// UpdateByTokenIdentifiersUnsafe updates a token by its token identifiers without checking if it is owned by any given user
func (t *TokenRepository) UpdateByTokenIdentifiersUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pUpdate interface{}) error {
	sqlStr := `UPDATE tokens SET `
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		sqlStr += "COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT_ADDRESS = $4"
		res, err = t.db.ExecContext(pCtx, sqlStr, update.CollectorsNote, update.LastUpdated, pTokenID, pContractAddress)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		sqlStr += "MEDIA = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT_ADDRESS = $4"
		res, err = t.db.ExecContext(pCtx, sqlStr, update.Media, update.LastUpdated, pTokenID, pContractAddress)
	default:
		return errors.New("unsupported update type")
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrTokenNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress}
	}
	return nil
}

// MostRecentBlock returns the most recent block number of any token
func (t *TokenRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {
	sqlStr := `SELECT MAX(BLOCK_NUMBER) FROM tokens`
	var blockNumber persist.BlockNumber
	err := t.db.QueryRowContext(pCtx, sqlStr).Scan(&blockNumber)
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

// Count returns the number of tokens in the database
func (t *TokenRepository) Count(pCtx context.Context, pTokenType persist.TokenCountType) (int64, error) {
	sqlStr := `SELECT COUNT(*) FROM tokens`
	var count int64
	err := t.db.QueryRowContext(pCtx, sqlStr).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
