package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenRepository represents a postgres repository for tokens
type TokenRepository struct {
	db                                                   *sql.DB
	getByWalletStmt                                      *sql.Stmt
	getByWalletPaginateStmt                              *sql.Stmt
	getOwnedByContractStmt                               *sql.Stmt
	getOwnedByContractPaginateStmt                       *sql.Stmt
	getByContractStmt                                    *sql.Stmt
	getByContractPaginateStmt                            *sql.Stmt
	getByTokenIdentifiersStmt                            *sql.Stmt
	getByTokenIdentifiersPaginateStmt                    *sql.Stmt
	getByIdentifiersStmt                                 *sql.Stmt
	getExistsByTokenIdentifiersStmt                      *sql.Stmt
	getMetadataByTokenIdentifiersStmt                    *sql.Stmt
	updateOwnerUnsafeStmt                                *sql.Stmt
	updateBalanceUnsafeStmt                              *sql.Stmt
	updateURIByTokenIdentifiersStmt                      *sql.Stmt
	updateAllMetadataDerivedFieldsByTokenIdentifiersStmt *sql.Stmt
	updateMetadataFieldByTokenIdentifiersStmt            *sql.Stmt
	mostRecentBlockStmt                                  *sql.Stmt
	upsert721Stmt                                        *sql.Stmt
	upsert1155Stmt                                       *sql.Stmt
	deleteStmt                                           *sql.Stmt
	deleteByIDStmt                                       *sql.Stmt
	getURIByTokenIdentifiersStmt                         *sql.Stmt
}

// NewTokenRepository creates a new TokenRepository
func NewTokenRepository(db *sql.DB) *TokenRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByWalletStmt, err := db.PrepareContext(ctx, `SELECT t.ID,t.TOKEN_TYPE,t.CHAIN,t.NAME,t.DESCRIPTION,t.TOKEN_ID,t.TOKEN_URI,t.QUANTITY,t.OWNER_ADDRESS,t.OWNERSHIP_HISTORY,t.TOKEN_METADATA,t.CONTRACT_ADDRESS,t.EXTERNAL_URL,t.BLOCK_NUMBER,t.VERSION,t.CREATED_AT,t.LAST_UPDATED,t.IS_SPAM,c.ID,c.VERSION,c.CREATED_AT,c.LAST_UPDATED,c.ADDRESS,c.SYMBOL,c.NAME,c.LATEST_BLOCK,c.CREATOR_ADDRESS FROM tokens t INNER JOIN contracts c ON c.ADDRESS = t.CONTRACT_ADDRESS WHERE t.OWNER_ADDRESS = $1 ORDER BY t.BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByWalletPaginateStmt, err := db.PrepareContext(ctx, `SELECT t.ID,t.TOKEN_TYPE,t.CHAIN,t.NAME,t.DESCRIPTION,t.TOKEN_ID,t.TOKEN_URI,t.QUANTITY,t.OWNER_ADDRESS,t.OWNERSHIP_HISTORY,t.TOKEN_METADATA,t.CONTRACT_ADDRESS,t.EXTERNAL_URL,t.BLOCK_NUMBER,t.VERSION,t.CREATED_AT,t.LAST_UPDATED,t.IS_SPAM,c.ID,c.VERSION,c.CREATED_AT,c.LAST_UPDATED,c.ADDRESS,c.SYMBOL,c.NAME,c.LATEST_BLOCK,c.CREATOR_ADDRESS FROM tokens t INNER JOIN contracts c ON c.ADDRESS = t.CONTRACT_ADDRESS WHERE t.OWNER_ADDRESS = $1 ORDER BY t.BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getOwnedByContractStmt, err := db.PrepareContext(ctx, `SELECT t.ID,t.TOKEN_TYPE,t.CHAIN,t.NAME,t.DESCRIPTION,t.TOKEN_ID,t.TOKEN_URI,t.QUANTITY,t.OWNER_ADDRESS,t.OWNERSHIP_HISTORY,t.TOKEN_METADATA,t.CONTRACT_ADDRESS,t.EXTERNAL_URL,t.BLOCK_NUMBER,t.VERSION,t.CREATED_AT,t.LAST_UPDATED,t.IS_SPAM,c.ID,c.VERSION,c.CREATED_AT,c.LAST_UPDATED,c.ADDRESS,c.SYMBOL,c.NAME,c.LATEST_BLOCK,c.CREATOR_ADDRESS FROM tokens t INNER JOIN contracts c ON c.ADDRESS = t.CONTRACT_ADDRESS WHERE t.OWNER_ADDRESS = $1 AND t.CONTRACT_ADDRESS = $2 ORDER BY t.BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getOwnedByContractPaginateStmt, err := db.PrepareContext(ctx, `SELECT t.ID,t.TOKEN_TYPE,t.CHAIN,t.NAME,t.DESCRIPTION,t.TOKEN_ID,t.TOKEN_URI,t.QUANTITY,t.OWNER_ADDRESS,t.OWNERSHIP_HISTORY,t.TOKEN_METADATA,t.CONTRACT_ADDRESS,t.EXTERNAL_URL,t.BLOCK_NUMBER,t.VERSION,t.CREATED_AT,t.LAST_UPDATED,t.IS_SPAM,c.ID,c.VERSION,c.CREATED_AT,c.LAST_UPDATED,c.ADDRESS,c.SYMBOL,c.NAME,c.LATEST_BLOCK,c.CREATOR_ADDRESS FROM tokens t INNER JOIN contracts c ON c.ADDRESS = t.CONTRACT_ADDRESS WHERE t.OWNER_ADDRESS = $1 AND t.CONTRACT_ADDRESS = $2 ORDER BY t.BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	getByContractStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_SPAM FROM tokens WHERE CONTRACT_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByContractPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_SPAM FROM tokens WHERE CONTRACT_ADDRESS = $1 ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	getByIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 AND OWNER_ADDRESS = $3;`)
	checkNoErr(err)

	getExistsByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT EXISTS(SELECT 1 FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2);`)
	checkNoErr(err)

	getMetadataByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT TOKEN_URI,TOKEN_METADATA FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 ORDER BY BLOCK_NUMBER DESC LIMIT 1;`)
	checkNoErr(err)

	updateOwnerUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET OWNER_ADDRESS = $1, OWNERSHIP_HISTORY = $2 || OWNERSHIP_HISTORY, BLOCK_NUMBER = $3, LAST_UPDATED = $4 WHERE ID = $5;`)
	checkNoErr(err)

	updateBalanceUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET QUANTITY = $1, BLOCK_NUMBER = $2, LAST_UPDATED = $3 WHERE ID = $4;`)
	checkNoErr(err)

	updateURIByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET TOKEN_URI = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT_ADDRESS = $4;`)
	checkNoErr(err)

	updateAllMetadataDerivedFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET NAME = $1, DESCRIPTION = $2, LAST_UPDATED = $3 WHERE TOKEN_ID = $4 AND CONTRACT_ADDRESS = $5 AND DELETED = false;`)
	checkNoErr(err)

	updateMetadataFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET TOKEN_URI = $1, TOKEN_METADATA = $2, LAST_UPDATED = $3 WHERE TOKEN_ID = $4 AND CONTRACT_ADDRESS = $5 AND DELETED = false;`)
	checkNoErr(err)

	mostRecentBlockStmt, err := db.PrepareContext(ctx, `SELECT MAX(BLOCK_NUMBER) FROM tokens;`)
	checkNoErr(err)

	upsert721Stmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS) WHERE TOKEN_TYPE = 'ERC-721' DO UPDATE SET TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED;`)
	checkNoErr(err)

	upsert1155Stmt, err := db.PrepareContext(ctx, `INSERT INTO tokens (ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_ADDRESS) WHERE TOKEN_TYPE = 'ERC-1155' DO UPDATE SET TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED;`)
	checkNoErr(err)

	deleteStmt, err := db.PrepareContext(ctx, `DELETE FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 AND OWNER_ADDRESS = $3 and TOKEN_TYPE = $4;`)
	checkNoErr(err)

	deleteByIDStmt, err := db.PrepareContext(ctx, `DELETE FROM tokens WHERE ID = $1;`)
	checkNoErr(err)

	getURIByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT TOKEN_URI FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT_ADDRESS = $2 AND LENGTH(TOKEN_URI) > 0 ORDER BY BLOCK_NUMBER DESC LIMIT 1;`)
	checkNoErr(err)

	return &TokenRepository{
		db:                                db,
		getByWalletStmt:                   getByWalletStmt,
		getByWalletPaginateStmt:           getByWalletPaginateStmt,
		getOwnedByContractStmt:            getOwnedByContractStmt,
		getOwnedByContractPaginateStmt:    getOwnedByContractPaginateStmt,
		getByContractStmt:                 getByContractStmt,
		getByContractPaginateStmt:         getByContractPaginateStmt,
		getByTokenIdentifiersStmt:         getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt: getByTokenIdentifiersPaginateStmt,
		getMetadataByTokenIdentifiersStmt: getMetadataByTokenIdentifiersStmt,
		updateOwnerUnsafeStmt:             updateOwnerUnsafeStmt,
		updateBalanceUnsafeStmt:           updateBalanceUnsafeStmt,
		updateURIByTokenIdentifiersStmt:   updateURIByTokenIdentifiersUnsafeStmt,
		updateAllMetadataDerivedFieldsByTokenIdentifiersStmt: updateAllMetadataDerivedFieldsByTokenIdentifiersUnsafeStmt,
		mostRecentBlockStmt:                       mostRecentBlockStmt,
		upsert721Stmt:                             upsert721Stmt,
		upsert1155Stmt:                            upsert1155Stmt,
		deleteStmt:                                deleteStmt,
		deleteByIDStmt:                            deleteByIDStmt,
		getByIdentifiersStmt:                      getByIdentifiersStmt,
		getExistsByTokenIdentifiersStmt:           getExistsByTokenIdentifiersStmt,
		getURIByTokenIdentifiersStmt:              getURIByTokenIdentifiersStmt,
		updateMetadataFieldByTokenIdentifiersStmt: updateMetadataFieldsByTokenIdentifiersUnsafeStmt,
	}

}

// GetByWallet retrieves all tokens associated with a wallet
func (t *TokenRepository) GetByWallet(pCtx context.Context, pAddress persist.EthereumAddress, limit int64, offset int64) ([]persist.Token, []persist.Contract, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByWalletPaginateStmt.QueryContext(pCtx, pAddress, limit, offset)
	} else {
		rows, err = t.getByWalletStmt.QueryContext(pCtx, pAddress)
	}
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	contracts := make(map[persist.DBID]persist.Contract)
	for rows.Next() {
		token := persist.Token{}
		contract := persist.Contract{}
		if err := rows.Scan(&token.ID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsSpam, &contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.CreatorAddress); err != nil {
			return nil, nil, err
		}
		tokens = append(tokens, token)
		contracts[contract.ID] = contract
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	resultContracts := make([]persist.Contract, 0, len(contracts))
	for _, contract := range contracts {
		resultContracts = append(resultContracts, contract)
	}

	return tokens, resultContracts, nil

}

// GetByContract retrieves all tokens associated with a contract
func (t *TokenRepository) GetByContract(pCtx context.Context, pContractAddress persist.EthereumAddress, limit int64, offset int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByContractPaginateStmt.QueryContext(pCtx, pContractAddress, limit, offset)
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
		if err := rows.Scan(&token.ID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsSpam); err != nil {
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

// GetOwnedByContract retrieves all tokens associated with a wallet with a specific contract
func (t *TokenRepository) GetOwnedByContract(pCtx context.Context, pContractAddress, pAddress persist.EthereumAddress, limit int64, offset int64) ([]persist.Token, persist.Contract, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getOwnedByContractPaginateStmt.QueryContext(pCtx, pAddress, pContractAddress, limit, offset)
	} else {
		rows, err = t.getOwnedByContractStmt.QueryContext(pCtx, pAddress, pContractAddress)
	}
	if err != nil {
		return nil, persist.Contract{}, err
	}
	defer rows.Close()

	tokens := make([]persist.Token, 0, 10)
	contract := persist.Contract{}
	for rows.Next() {
		token := persist.Token{}

		if err := rows.Scan(&token.ID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsSpam, &contract.ID, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Address, &contract.Symbol, &contract.Name, &contract.LatestBlock, &contract.CreatorAddress); err != nil {
			return nil, persist.Contract{}, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, persist.Contract{}, err
	}

	return tokens, contract, nil

}

// GetByTokenIdentifiers gets a token by its token ID and contract address
func (t *TokenRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.EthereumAddress, limit int64, offset int64) ([]persist.Token, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByTokenIdentifiersPaginateStmt.QueryContext(pCtx, pTokenID, pContractAddress, limit, offset)
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
		if err := rows.Scan(&token.ID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsSpam); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokenNotFoundByTokenIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress}
	}

	return tokens, nil
}

func (t *TokenRepository) GetURIByTokenIdentifiers(pCtx context.Context, tokenID persist.TokenID, contract persist.EthereumAddress) (persist.TokenURI, error) {
	var uri persist.TokenURI
	err := t.getURIByTokenIdentifiersStmt.QueryRowContext(pCtx, tokenID, contract).Scan(&uri)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no token URI found for token ID %s and contract %s", tokenID, contract)
		}
		return "", err
	}

	return uri, nil
}

// GetByIdentifiers gets a token by its token ID and contract address and owner address
func (t *TokenRepository) GetByIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress, pOwnerAddress persist.EthereumAddress) (persist.Token, error) {
	var token persist.Token
	err := t.getByIdentifiersStmt.QueryRowContext(pCtx, pTokenID, pContractAddress, pOwnerAddress).Scan(&token.ID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerAddress, pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ContractAddress, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return token, persist.ErrTokenNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress, OwnerAddress: pOwnerAddress}
		}
		return persist.Token{}, err
	}
	return token, nil
}

// TokenExistsByTokenIdentifiers gets a token by its token ID and contract address and owner address
func (t *TokenRepository) TokenExistsByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.EthereumAddress) (bool, error) {
	var exists bool
	err := t.getExistsByTokenIdentifiersStmt.QueryRowContext(pCtx, pTokenID, pContractAddress).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// BulkUpsert upserts multiple tokens
func (t *TokenRepository) BulkUpsert(pCtx context.Context, pTokens []persist.Token) error {
	if len(pTokens) == 0 {
		return nil
	}

	logger.For(pCtx).Infof("Deduping %d tokens", len(pTokens))

	pTokens = t.dedupeTokens(pTokens)

	logger.For(pCtx).Infof("Deduped down to %d tokens", len(pTokens))

	erc1155Tokens := make([]persist.Token, 0, len(pTokens)/2)
	erc721Tokens := make([]persist.Token, 0, len(pTokens)/2)

	logger.For(pCtx).Infof("Separating %d tokens into ERC1155 and ERC721", len(pTokens))
	for _, token := range pTokens {
		switch token.TokenType {
		case persist.TokenTypeERC721:
			erc721Tokens = append(erc721Tokens, token)
		case persist.TokenTypeERC1155:
			erc1155Tokens = append(erc1155Tokens, token)
		default:
			return fmt.Errorf("unknown token type: %s", token.TokenType)
		}
	}

	logger.For(pCtx).Infof("Starting upsert...")

	if err := t.upsertERC1155Tokens(pCtx, erc1155Tokens); err != nil {
		return err
	}

	if err := t.upsertERC721Tokens(pCtx, erc721Tokens); err != nil {
		return err
	}

	for _, token := range erc1155Tokens {
		if token.Quantity == "" || token.Quantity == "0" || token.Quantity == "<nil>" {
			logger.For(pCtx).Debugf("Deleting token %s for 0 quantity", persist.NewTokenIdentifiers(persist.Address(token.ContractAddress.String()), token.TokenID, token.Chain))
			if err := t.deleteTokenUnsafe(pCtx, token.TokenID, token.ContractAddress, token.OwnerAddress, token.TokenType); err != nil {
				return err
			}
		}
	}

	return nil

}

func (t *TokenRepository) upsertERC721Tokens(pCtx context.Context, pTokens []persist.Token) error {
	if len(pTokens) == 0 {
		return nil
	}
	// Postgres only allows 65535 parameters at a time.
	// TODO: Consider trying this implementation at some point instead of chunking:
	//       https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit
	paramsPerRow := 20
	rowsPerQuery := 65535 / paramsPerRow

	if len(pTokens) > rowsPerQuery {
		logger.For(pCtx).Debugf("Chunking %d tokens recursively into %d queries", len(pTokens), len(pTokens)/rowsPerQuery)
		next := pTokens[rowsPerQuery:]
		current := pTokens[:rowsPerQuery]
		if err := t.upsertERC721Tokens(pCtx, next); err != nil {
			return fmt.Errorf("error with erc721 upsert: %w", err)
		}
		pTokens = current
	}

	sqlStr := `INSERT INTO tokens (ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,DELETED,IS_SPAM) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*paramsPerRow)
	for i, token := range pTokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow, nil) + ","
		vals = append(vals, persist.GenerateID(), token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated, token.Deleted, token.IsSpam)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS) WHERE TOKEN_TYPE = 'ERC-721' DO UPDATE SET TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_ADDRESS = EXCLUDED.OWNER_ADDRESS,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED,DELETED = EXCLUDED.DELETED,IS_SPAM = EXCLUDED.IS_SPAM WHERE EXCLUDED.BLOCK_NUMBER > tokens.BLOCK_NUMBER;`

	_, err := t.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		logger.For(pCtx).Errorf("SQL: %s", sqlStr)
		return fmt.Errorf("failed to upsert erc721 tokens: %w", err)
	}
	return nil
}

func (t *TokenRepository) upsertERC1155Tokens(pCtx context.Context, pTokens []persist.Token) error {
	if len(pTokens) == 0 {
		return nil
	}
	// Postgres only allows 65535 parameters at a time.
	// TODO: Consider trying this implementation at some point instead of chunking:
	//       https://klotzandrew.com/blog/postgres-passing-65535-parameter-limit
	paramsPerRow := 20
	rowsPerQuery := 65535 / paramsPerRow

	if len(pTokens) > rowsPerQuery {
		logger.For(pCtx).Debugf("Chunking %d tokens recursively into %d queries", len(pTokens), len(pTokens)/rowsPerQuery)
		next := pTokens[rowsPerQuery:]
		current := pTokens[:rowsPerQuery]
		if err := t.upsertERC1155Tokens(pCtx, next); err != nil {
			return fmt.Errorf("error with erc1155 upsert: %w", err)
		}
		pTokens = current
	}

	sqlStr := `INSERT INTO tokens (ID,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_ADDRESS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT_ADDRESS,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,DELETED,IS_SPAM) VALUES `
	vals := make([]interface{}, 0, len(pTokens)*paramsPerRow)
	for i, token := range pTokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow, nil) + ","
		vals = append(vals, persist.GenerateID(), token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerAddress, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.ContractAddress, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated, token.Deleted, token.IsSpam)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT_ADDRESS,OWNER_ADDRESS) WHERE TOKEN_TYPE = 'ERC-1155' DO UPDATE SET TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,CREATED_AT = EXCLUDED.CREATED_AT,LAST_UPDATED = EXCLUDED.LAST_UPDATED,DELETED = EXCLUDED.DELETED,IS_SPAM = EXCLUDED.IS_SPAM;`

	_, err := t.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		logger.For(pCtx).Errorf("SQL: %s", sqlStr)
		return fmt.Errorf("failed to upsert erc1155 tokens: %w", err)
	}
	return nil
}

// Upsert upserts a token by its token ID and contract address and if its token type is ERC-1155 it also upserts using the owner address
func (t *TokenRepository) Upsert(pCtx context.Context, pToken persist.Token) error {
	var err error
	if pToken.Quantity == "0" {
		_, err = t.deleteStmt.ExecContext(pCtx, pToken.TokenID, pToken.ContractAddress, pToken.OwnerAddress, pToken.TokenType)
	} else {
		if pToken.TokenType == persist.TokenTypeERC1155 {
			_, err = t.upsert1155Stmt.ExecContext(pCtx, persist.GenerateID(), pToken.TokenType, pToken.Chain, pToken.Name, pToken.Description, pToken.TokenID, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pToken.OwnershipHistory, pToken.TokenMetadata, pToken.ContractAddress, pToken.ExternalURL, pToken.BlockNumber, pToken.Version, pToken.CreationTime, pToken.LastUpdated)
		} else if pToken.TokenType == persist.TokenTypeERC721 {
			_, err = t.upsert721Stmt.ExecContext(pCtx, persist.GenerateID(), pToken.TokenType, pToken.Chain, pToken.Name, pToken.Description, pToken.TokenID, pToken.TokenURI, pToken.Quantity, pToken.OwnerAddress, pToken.OwnershipHistory, pToken.TokenMetadata, pToken.ContractAddress, pToken.ExternalURL, pToken.BlockNumber, pToken.Version, pToken.CreationTime, pToken.LastUpdated)
		}
	}
	return err
}

// UpdateByID updates a token by its ID
func (t *TokenRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUpdate interface{}) error {

	var res sql.Result
	var err error
	switch pUpdate.(type) {
	case persist.TokenUpdateOwnerInput:
		update := pUpdate.(persist.TokenUpdateOwnerInput)
		res, err = t.updateOwnerUnsafeStmt.ExecContext(pCtx, update.OwnerAddress, []persist.AddressAtBlock{{Address: persist.Address(update.OwnerAddress), Block: update.BlockNumber}}, update.BlockNumber, persist.LastUpdatedTime{}, pID)
	case persist.TokenUpdateBalanceInput:
		update := pUpdate.(persist.TokenUpdateBalanceInput)
		res, err = t.updateBalanceUnsafeStmt.ExecContext(pCtx, update.Quantity, update.BlockNumber, persist.LastUpdatedTime{}, pID)
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
	// this all makes me feel sticky because it is not doubled up on the UpdateByID method, but I am assuming that this will all become irrelevant soon with the sqlc refactor by ezra
	case persist.TokenUpdateURIInput:
		update := pUpdate.(persist.TokenUpdateURIInput)
		res, err = t.updateURIByTokenIdentifiersStmt.ExecContext(pCtx, update.TokenURI, update.LastUpdated, pTokenID, pContractAddress)
	case persist.TokenUpdateMetadataDerivedFieldsInput:
		update := pUpdate.(persist.TokenUpdateMetadataDerivedFieldsInput)
		res, err = t.updateAllMetadataDerivedFieldsByTokenIdentifiersStmt.ExecContext(pCtx, update.Name, update.Description, update.LastUpdated, pTokenID, pContractAddress)
	case persist.TokenUpdateMetadataFieldsInput:
		update := pUpdate.(persist.TokenUpdateMetadataFieldsInput)
		res, err = t.updateMetadataFieldByTokenIdentifiersStmt.ExecContext(pCtx, update.TokenURI, update.Metadata, update.LastUpdated, pTokenID, pContractAddress)
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
		return persist.ErrTokenNotFoundByTokenIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress}
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

func (t *TokenRepository) DeleteByID(pCtx context.Context, pID persist.DBID) error {
	_, err := t.deleteByIDStmt.ExecContext(pCtx, pID)
	return err
}

func (t *TokenRepository) deleteTokenUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress, pOwnerAddress persist.EthereumAddress, pTokenType persist.TokenType) error {
	_, err := t.deleteStmt.ExecContext(pCtx, pTokenID, pContractAddress, pOwnerAddress, pTokenType)
	return err
}

type blockWithIndex struct {
	block persist.BlockNumber
	index int
}

func (t *TokenRepository) dedupeTokens(pTokens []persist.Token) []persist.Token {
	seen := map[string]persist.Token{}
	for _, token := range pTokens {
		var key string
		if token.TokenType == persist.TokenTypeERC1155 {
			key = token.ContractAddress.String() + "-" + token.TokenID.String() + "-" + token.OwnerAddress.String()
		} else {
			key = token.ContractAddress.String() + "-" + token.TokenID.String()
		}

		if seenToken, ok := seen[key]; ok {
			if seenToken.BlockNumber.Uint64() > token.BlockNumber.Uint64() {
				continue
			}
		}
		seen[key] = token
	}
	result := make([]persist.Token, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	return result
}
