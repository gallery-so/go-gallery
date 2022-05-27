package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/mikeydub/go-gallery/service/logger"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenRepository represents a postgres repository for tokens
type TokenRepository struct {
	db                                      *sql.DB
	createStmt                              *sql.Stmt
	getByWalletStmt                         *sql.Stmt
	getByWalletPaginateStmt                 *sql.Stmt
	getByContractStmt                       *sql.Stmt
	getByContractPaginateStmt               *sql.Stmt
	getByTokenIDStmt                        *sql.Stmt
	getByTokenIDPaginateStmt                *sql.Stmt
	getByTokenIdentifiersStmt               *sql.Stmt
	getByTokenIdentifiersPaginateStmt       *sql.Stmt
	getByIDStmt                             *sql.Stmt
	updateMediaStmt                         *sql.Stmt
	updateMediaUnsafeStmt                   *sql.Stmt
	updateMediaByTokenIdentifiersUnsafeStmt *sql.Stmt
	mostRecentBlockStmt                     *sql.Stmt
	countTokensStmt                         *sql.Stmt
	upsertStmt                              *sql.Stmt
	deleteBalanceZeroStmt                   *sql.Stmt
	deleteStmt                              *sql.Stmt
}

// NewTokenRepository creates a new TokenRepository
func NewTokenRepository(db *sql.DB) *TokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createStmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,VERSION,MEDIA,TOKEN_METADATA,TOKEN_TYPE,TOKEN_ID,CHAIN,NAME,DESCRIPTION,EXTERNAL_URL,BLOCK_NUMBER,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,CONTRACT_ADDRESS) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING ID;`)
	checkNoErr(err)

	getByWalletStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE OWNER_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByWalletPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE OWNER_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByContractStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE CONTRACT_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByContractPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE CONTRACT_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIDStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE ID = $1;`)
	checkNoErr(err)

	updateMediaUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE ID = $5;`)
	checkNoErr(err)

	updateMediaStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE ID = $5 AND OWNER_ADDRESS = ANY($6);`)
	checkNoErr(err)

	updateMediaByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE TOKEN_ID = $5 AND CONTRACT_ADDRESS = $6;`)
	checkNoErr(err)

	mostRecentBlockStmt, err := db.PrepareContext(ctx, `SELECT MAX(BLOCK_NUMBER) FROM tokens;`)
	checkNoErr(err)

	countTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) FROM tokens;`)
	checkNoErr(err)

	upsertStmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_ADDRESS) DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED;`)
	checkNoErr(err)

	deleteBalanceZeroStmt, err := db.PrepareContext(ctx, `DELETE FROM tokens WHERE QUANTITY = '0';`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `DELETE FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 AND OWNER_ADDRESS = $3;`)
	checkNoErr(err)

	return &TokenRepository{
		db:                                      db,
		createStmt:                              createStmt,
		getByWalletStmt:                         getByWalletStmt,
		getByWalletPaginateStmt:                 getByWalletPaginateStmt,
		getByContractStmt:                       getByContractStmt,
		getByContractPaginateStmt:               getByContractPaginateStmt,
		getByTokenIdentifiersStmt:               getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt:       getByTokenIdentifiersPaginateStmt,
		getByIDStmt:                             getByIDStmt,
		updateMediaUnsafeStmt:                   updateMediaUnsafeStmt,
		updateMediaStmt:                         updateMediaStmt,
		updateMediaByTokenIdentifiersUnsafeStmt: updateMediaByTokenIdentifiersUnsafeStmt,
		mostRecentBlockStmt:                     mostRecentBlockStmt,
		countTokensStmt:                         countTokensStmt,
		upsertStmt:                              upsertStmt,
		deleteBalanceZeroStmt:                   deleteBalanceZeroStmt,
		deleteStmt:                              deleteStmt,
		getByTokenIDStmt:                        getByTokenIDStmt,
		getByTokenIDPaginateStmt:                getByTokenIDPaginateStmt,
	}

}

// CreateBulk creates many tokens in the database
func (t *TokenRepository) CreateBulk(pCtx context.Context, pTokens []persist.Token) ([]persist.DBID, error) {
	insertSQL := `INSERT INTO tokens (ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*15)
	for i, token := range pTokens {
		insertSQL += generateValuesPlaceholders(15, i*15) + ","
		vals = append(vals, token.ID, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version)
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

	var id persist.DBID
	err := t.createStmt.QueryRowContext(pCtx, pToken.ID, pToken.Version, pToken.Media, pToken.TokenMetadata, pToken.TokenType, pToken.TokenID, pToken.Chain, pToken.Name, pToken.Description, pToken.ExternalURL, pToken.BlockNumber, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pq.Array(pToken.OwnershipHistory), pToken.ContractAddress).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// GetByWallet retrieves all tokens associated with a wallet
func (t *TokenRepository) GetByWallet(pCtx context.Context, pAddress persist.EthereumAddress, limit int64, page int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByWalletPaginateStmt.QueryContext(pCtx, pAddress, limit, page)
	} else {
		rows, err = t.getByWalletStmt.QueryContext(pCtx, pAddress)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
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
func (t *TokenRepository) GetByContract(pCtx context.Context, pContractAddress persist.EthereumAddress, limit int64, page int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByContractPaginateStmt.QueryContext(pCtx, pContractAddress, limit, page)
	} else {
		rows, err = t.getByContractStmt.QueryContext(pCtx, pContractAddress)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokensNotFoundByContract{ContractAddress: pContractAddress}
	}

	return tokens, nil
}

// GetByTokenIdentifiers gets a token by its token ID and contract address
func (t *TokenRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.EthereumAddress, limit int64, page int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByTokenIdentifiersPaginateStmt.QueryContext(pCtx, pTokenID, pContractAddress, limit, page)
	} else {
		rows, err = t.getByTokenIdentifiersStmt.QueryContext(pCtx, pTokenID, pContractAddress)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
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

// GetByTokenID retrieves all tokens associated with a contract
func (t *TokenRepository) GetByTokenID(pCtx context.Context, pTokenID persist.TokenID, limit int64, page int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByTokenIDPaginateStmt.QueryContext(pCtx, pTokenID, limit, page)
	} else {
		rows, err = t.getByTokenIDStmt.QueryContext(pCtx, pTokenID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	for rows.Next() {
		token := persist.Token{}
		if err := rows.Scan(&token.ID, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated); err != nil {
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
func (t *TokenRepository) GetByID(pCtx context.Context, pID persist.DBID) (persist.Token, error) {
	token := persist.Token{}
	err := t.getByIDStmt.QueryRowContext(pCtx, pID).Scan(&token.ID, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated)
	if err != nil {
		return persist.Token{}, err
	}
	return token, nil
}

// BulkUpsert upserts multiple tokens
func (t *TokenRepository) BulkUpsert(pCtx context.Context, pTokens []persist.Token) error {
	if len(pTokens) == 0 {
		return nil
	}

	owners := map[string]bool{}
	for _, token := range pTokens {
		owners[token.OwnerAddress.String()] = true
		if token.TokenType != persist.TokenTypeERC721 {
			continue
		}
		for _, ownership := range token.OwnershipHistory {
			if !owners[ownership.Address.String()] {
				logger.For(pCtx).Debugf("Deleting ownership history for %s for token %s", ownership.Address.String(), persist.NewTokenIdentifiers(persist.Address(token.ContractAddress.String()), token.TokenID, token.Chain))
				if err := t.deleteTokenUnsafe(pCtx, token.TokenID, token.ContractAddress, ownership.Address); err != nil {
					return err
				}
				owners[ownership.Address.String()] = true
			}
		}
	}

	logger.For(pCtx).Infof("Checking 0 quantities for tokens...")
	for i, token := range pTokens {
		if token.Quantity == "" || token.Quantity == "0" {
			logger.For(pCtx).Debugf("Deleting token %s for 0 quantity", persist.NewTokenIdentifiers(persist.Address(token.ContractAddress.String()), token.TokenID, token.Chain))
			if err := t.deleteTokenUnsafe(pCtx, token.TokenID, token.ContractAddress, token.OwnerAddress); err != nil {
				return err
			}
			if len(pTokens) < i+1 {
				pTokens = pTokens[:i]
			} else {
				pTokens = append(pTokens[:i], pTokens[i+1:]...)
			}
		}
	}

	if len(pTokens) == 0 {
		return nil
	}

	// Postgres only allows 65535 parameters at a time.
	// TODO: Consider trying this implementation at some point instead of chunking:
	//       https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit
	paramsPerRow := 18
	rowsPerQuery := 65535 / paramsPerRow

	if len(pTokens) > rowsPerQuery {
		logger.For(pCtx).Debugf("Chunking %d tokens recursively into %d queries", len(pTokens), len(pTokens)/rowsPerQuery)
		next := pTokens[rowsPerQuery:]
		current := pTokens[:rowsPerQuery]
		if err := t.BulkUpsert(pCtx, next); err != nil {
			return err
		}
		pTokens = current
	}

	pTokens = t.dedupTokens(pTokens)

	sqlStr := `INSERT INTO tokens (ID,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*paramsPerRow)
	for i, token := range pTokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, persist.GenerateID(), token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_ADDRESS) DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED WHERE EXCLUDED.BLOCK_NUMBER > tokens.BLOCK_NUMBER`

	_, err := t.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		logger.For(pCtx).Debugf("SQL: %s", sqlStr)
		return fmt.Errorf("failed to upsert tokens: %w", err)
	}

	return nil

}

// Upsert upserts a token by its token ID and contract address and if its token type is ERC-1155 it also upserts using the owner address
func (t *TokenRepository) Upsert(pCtx context.Context, pToken persist.Token) error {
	var err error
	if pToken.Quantity == "0" {
		_, err = t.deleteStmt.ExecContext(pCtx, pToken.TokenID, pToken.ContractAddress, pToken.OwnerAddress)
	} else {
		_, err = t.upsertStmt.ExecContext(pCtx, persist.GenerateID(), pToken.Media, pToken.TokenType, pToken.Chain, pToken.Name, pToken.Description, pToken.TokenID, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pToken.OwnershipHistory, pToken.TokenMetadata, pToken.ContractAddress, pToken.ExternalURL, pToken.BlockNumber, pToken.Version, pToken.CreationTime, pToken.LastUpdated)
	}
	return err
}

// UpdateByID updates a token by its ID
func (t *TokenRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	var res sql.Result
	var err error
	switch pUpdate.(type) {
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

// UpdateByTokenIdentifiers updates a token by its token identifiers without checking if it is owned by any given user
func (t *TokenRepository) UpdateByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.EthereumAddress, pUpdate interface{}) error {
	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		res, err = t.updateMediaByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Media, update.TokenURI, update.Metadata, update.LastUpdated, pTokenID, pContractAddress)
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
		return persist.ErrTokenNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress}
	}
	return nil
}

// MostRecentBlock returns the most recent block number of any token
func (t *TokenRepository) MostRecentBlock(pCtx context.Context) (persist.BlockNumber, error) {
	var blockNumber persist.BlockNumber
	err := t.mostRecentBlockStmt.QueryRowContext(pCtx).Scan(&blockNumber)
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

// Count returns the number of tokens in the database
func (t *TokenRepository) Count(pCtx context.Context, pTokenType persist.TokenCountType) (int64, error) {
	var count int64
	err := t.countTokensStmt.QueryRowContext(pCtx).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (t *TokenRepository) deleteTokenUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress, pOwnerAddress persist.EthereumAddress) error {
	_, err := t.deleteStmt.ExecContext(pCtx, pTokenID, pContractAddress, pOwnerAddress)
	return err
}

type blockWithIndex struct {
	block persist.BlockNumber
	index int
}

func (t *TokenRepository) dedupTokens(pTokens []persist.Token) []persist.Token {
	seen := map[string]persist.Token{}
	for _, token := range pTokens {
		key := token.ContractAddress.String() + "-" + token.TokenID.String() + "-" + token.OwnerAddress.String()
		if seenToken, ok := seen[key]; ok {
			if seenToken.BlockNumber.Uint64() > token.BlockNumber.Uint64() {
				continue
			}
			seen[key] = token
		}
		seen[key] = token
	}
	result := make([]persist.Token, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}
