package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/sirupsen/logrus"
)

// TokenGalleryRepository represents a postgres repository for tokens
type TokenGalleryRepository struct {
	db                                      *sql.DB
	galleryRepo                             *GalleryRepository
	getByUserIDStmt                         *sql.Stmt
	getByUserIDPaginateStmt                 *sql.Stmt
	getByTokenIDStmt                        *sql.Stmt
	getByTokenIDPaginateStmt                *sql.Stmt
	getByTokenIdentifiersStmt               *sql.Stmt
	getByTokenIdentifiersPaginateStmt       *sql.Stmt
	updateInfoStmt                          *sql.Stmt
	updateMediaStmt                         *sql.Stmt
	updateInfoByTokenIdentifiersUnsafeStmt  *sql.Stmt
	updateMediaByTokenIdentifiersUnsafeStmt *sql.Stmt
	deleteByIdentifiersStmt                 *sql.Stmt
	deleteByIDStmt                          *sql.Stmt
	getContractByAddressStmt                *sql.Stmt
	setTokensAsUserMarkedSpamStmt           *sql.Stmt
	checkOwnTokensStmt                      *sql.Stmt
}

var errTokensNotOwnedByUser = errors.New("not all tokens are owned by user")

// NewTokenGalleryRepository creates a new TokenRepository
// TODO joins on addresses
func NewTokenGalleryRepository(db *sql.DB, galleryRepo *GalleryRepository) *TokenGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE OWNER_USER_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByUserIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE OWNER_USER_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateMediaStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, LAST_UPDATED = $4 WHERE ID = $5 AND OWNER_USER_ID = $6;`)
	checkNoErr(err)

	updateInfoByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateMediaByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = $2, TOKEN_METADATA = $3, NAME = $4, DESCRIPTION = $5, LAST_UPDATED = $6 WHERE TOKEN_ID = $7 AND CONTRACT = $8 AND DELETED = false;`)
	checkNoErr(err)

	deleteByIdentifiersStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND OWNER_USER_ID = $3 AND CHAIN = $4;`)
	checkNoErr(err)

	deleteByIDStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE ID = $1;`)
	checkNoErr(err)

	getContractByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	setTokensAsUserMarkedSpamStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET is_user_marked_spam = $1, LAST_UPDATED = now() WHERE OWNER_USER_ID = $2 AND ID = ANY($3) AND DELETED = false;`)
	checkNoErr(err)

	checkOwnTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) = $1 FROM tokens WHERE OWNER_USER_ID = $2 AND ID = ANY($3);`)
	checkNoErr(err)

	setTokensAsUserMarkedSpamStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET is_user_marked_spam = $1, LAST_UPDATED = now() WHERE OWNER_USER_ID = $2 AND ID = ANY($3) AND DELETED = false;`)
	checkNoErr(err)

	checkOwnTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) = $1 FROM tokens WHERE OWNER_USER_ID = $2 AND ID = ANY($3);`)
	checkNoErr(err)

	return &TokenGalleryRepository{
		db:                                      db,
		galleryRepo:                             galleryRepo,
		getByUserIDStmt:                         getByUserIDStmt,
		getByUserIDPaginateStmt:                 getByUserIDPaginateStmt,
		getByTokenIdentifiersStmt:               getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt:       getByTokenIdentifiersPaginateStmt,
		updateInfoStmt:                          updateInfoStmt,
		updateMediaStmt:                         updateMediaStmt,
		updateInfoByTokenIdentifiersUnsafeStmt:  updateInfoByTokenIdentifiersUnsafeStmt,
		updateMediaByTokenIdentifiersUnsafeStmt: updateMediaByTokenIdentifiersUnsafeStmt,
		deleteByIdentifiersStmt:                 deleteByIdentifiersStmt,
		getByTokenIDStmt:                        getByTokenIDStmt,
		getByTokenIDPaginateStmt:                getByTokenIDPaginateStmt,
		deleteByIDStmt:                          deleteByIDStmt,
		getContractByAddressStmt:                getContractByAddressStmt,
		setTokensAsUserMarkedSpamStmt:           setTokensAsUserMarkedSpamStmt,
		checkOwnTokensStmt:                      checkOwnTokensStmt,
	}

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
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam); err != nil {
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
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam); err != nil {
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
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam); err != nil {
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

// BulkUpsert upserts multiple tokens
func (t *TokenGalleryRepository) BulkUpsert(pCtx context.Context, pTokens []persist.TokenGallery) error {
	if len(pTokens) == 0 {
		return nil
	}

	logrus.Infof("Checking 0 quantities for tokens...")
	newTokens := make([]persist.TokenGallery, len(pTokens))
	for i, token := range pTokens {
		if token.Quantity == "" || token.Quantity == "0" {
			logger.For(pCtx).Warnf("Token %s has 0 quantity", token.Name)
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
	paramsPerRow := 22
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

	sqlStr := `INSERT INTO tokens (ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,DELETED,IS_PROVIDER_MARKED_SPAM) VALUES `
	vals := make([]interface{}, 0, len(newTokens)*paramsPerRow)
	for i, token := range newTokens {
		sqlStr += generateValuesPlaceholders(paramsPerRow, i*paramsPerRow) + ","
		vals = append(vals, persist.GenerateID(), token.CollectorsNote, token.Media, token.TokenType, token.Chain, token.Name, token.Description, token.TokenID, token.TokenURI, token.Quantity, token.OwnerUserID, token.OwnedByWallets, pq.Array(token.OwnershipHistory), token.TokenMetadata, token.Contract, token.ExternalURL, token.BlockNumber, token.Version, token.CreationTime, token.LastUpdated, token.Deleted, token.IsProviderMarkedSpam)
	}

	sqlStr = sqlStr[:len(sqlStr)-1]

	sqlStr += ` ON CONFLICT (TOKEN_ID,CONTRACT,CHAIN,OWNER_USER_ID) WHERE DELETED = false DO UPDATE SET MEDIA = EXCLUDED.MEDIA,TOKEN_TYPE = EXCLUDED.TOKEN_TYPE,CHAIN = EXCLUDED.CHAIN,NAME = EXCLUDED.NAME,DESCRIPTION = EXCLUDED.DESCRIPTION,TOKEN_URI = EXCLUDED.TOKEN_URI,QUANTITY = EXCLUDED.QUANTITY,OWNER_USER_ID = EXCLUDED.OWNER_USER_ID,OWNED_BY_WALLETS = EXCLUDED.OWNED_BY_WALLETS,OWNERSHIP_HISTORY = tokens.OWNERSHIP_HISTORY || EXCLUDED.OWNERSHIP_HISTORY,TOKEN_METADATA = EXCLUDED.TOKEN_METADATA,EXTERNAL_URL = EXCLUDED.EXTERNAL_URL,BLOCK_NUMBER = EXCLUDED.BLOCK_NUMBER,VERSION = EXCLUDED.VERSION,LAST_UPDATED = EXCLUDED.LAST_UPDATED,IS_USER_MARKED_SPAM = tokens.IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM = EXCLUDED.IS_PROVIDER_MARKED_SPAM;`

	_, err := t.db.ExecContext(pCtx, sqlStr, vals...)
	if err != nil {
		logrus.Debugf("SQL: %s", sqlStr)
		return fmt.Errorf("failed to upsert tokens: %w", err)
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
		res, err = t.updateMediaByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Media, update.TokenURI, update.Metadata, update.Name, update.Description, update.LastUpdated, pTokenID, contractID)
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

// DeleteByID deletes a token by its ID
func (t *TokenGalleryRepository) DeleteByID(ctx context.Context, id persist.DBID) error {
	_, err := t.deleteByIDStmt.ExecContext(ctx, id)
	return err
}

// FlagTokensAsUserMarkedSpam marks tokens as spam by the user.
func (t *TokenGalleryRepository) FlagTokensAsUserMarkedSpam(ctx context.Context, ownerUserID persist.DBID, tokens []persist.DBID, isSpam bool) error {
	_, err := t.setTokensAsUserMarkedSpamStmt.ExecContext(ctx, isSpam, ownerUserID, tokens)
	return err
}

// TokensAreOwnedByUser checks if all tokens are owned by the provided user.
func (t *TokenGalleryRepository) TokensAreOwnedByUser(ctx context.Context, userID persist.DBID, tokens []persist.DBID) error {
	var owned bool

	err := t.checkOwnTokensStmt.QueryRowContext(ctx, len(tokens), userID, tokens).Scan(&owned)
	if err != nil {
		return err
	}

	if !owned {
		return errTokensNotOwnedByUser
	}

	return nil
}

// FlagTokensAsUserMarkedSpam marks tokens as spam by the user.
func (t *TokenGalleryRepository) FlagTokensAsUserMarkedSpam(ctx context.Context, ownerUserID persist.DBID, tokens []persist.DBID, isSpam bool) error {
	_, err := t.setTokensAsUserMarkedSpamStmt.ExecContext(ctx, isSpam, ownerUserID, tokens)
	return err
}

// TokensAreOwnedByUser checks if all tokens are owned by the provided user.
func (t *TokenGalleryRepository) TokensAreOwnedByUser(ctx context.Context, userID persist.DBID, tokens []persist.DBID) error {
	var owned bool

	err := t.checkOwnTokensStmt.QueryRowContext(ctx, len(tokens), userID, tokens).Scan(&owned)
	if err != nil {
		return err
	}

	if !owned {
		return errTokensNotOwnedByUser
	}

	return nil
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
