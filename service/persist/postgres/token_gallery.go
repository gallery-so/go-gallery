package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgtype"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenGalleryRepository represents a postgres repository for tokens
type TokenGalleryRepository struct {
	db                                                  *sql.DB
	queries                                             *db.Queries
	getByID                                             *sql.Stmt
	getByUserIDStmt                                     *sql.Stmt
	getByUserIDPaginateStmt                             *sql.Stmt
	getByTokenIDStmt                                    *sql.Stmt
	getByTokenIDPaginateStmt                            *sql.Stmt
	getByTokenIdentifiersStmt                           *sql.Stmt
	getByTokenIdentifiersPaginateStmt                   *sql.Stmt
	getByFullIdentifiersStmt                            *sql.Stmt
	updateInfoStmt                                      *sql.Stmt
	updateMediaStmt                                     *sql.Stmt
	updateInfoByTokenIdentifiersUnsafeStmt              *sql.Stmt
	updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt *sql.Stmt
	updateMediaByTokenIdentifiersUnsafeStmt             *sql.Stmt
	updateMetadataFieldsByTokenIdentifiersUnsafeStmt    *sql.Stmt
	deleteByIdentifiersStmt                             *sql.Stmt
	deleteByIDStmt                                      *sql.Stmt
	getContractByAddressStmt                            *sql.Stmt
	setTokensAsUserMarkedSpamStmt                       *sql.Stmt
	checkOwnTokensStmt                                  *sql.Stmt
	deleteTokensOfContractBeforeTimeStampStmt           *sql.Stmt
	deleteTokensOfOwnerBeforeTimeStampStmt              *sql.Stmt
}

var errTokensNotOwnedByUser = errors.New("not all tokens are owned by user")

// NewTokenGalleryRepository creates a new TokenRepository
// TODO joins on addresses
func NewTokenGalleryRepository(db *sql.DB, queries *db.Queries) *TokenGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE ID = $1 AND DELETED = false;`)
	checkNoErr(err)

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

	getByFullIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND OWNER_USER_ID = $3 AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateMediaStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = '', TOKEN_METADATA = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`)
	checkNoErr(err)

	updateInfoByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateURIDerivedFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = '', TOKEN_METADATA = $2, NAME = $3, DESCRIPTION = $4, LAST_UPDATED = $5 WHERE TOKEN_ID = $6 AND CONTRACT = $7 AND DELETED = false;`)
	checkNoErr(err)

	updateMediaByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateMetadataFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET NAME = $1, DESCRIPTION = $2, LAST_UPDATED = $3 WHERE TOKEN_ID = $4 AND CONTRACT = $5 AND DELETED = false;`)
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

	deleteTokensOfContractBeforeTimeStampStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE CONTRACT = $1 AND LAST_SYNCED < $2 AND DELETED = false;`)
	checkNoErr(err)

	deleteTokensOfOwnerBeforeTimeStampStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET DELETED = true WHERE OWNER_USER_ID = $1 AND CHAIN = ANY($2) AND LAST_SYNCED < $3 AND DELETED = false;`)
	checkNoErr(err)

	return &TokenGalleryRepository{
		db:                                     db,
		queries:                                queries,
		getByID:                                getByIDStmt,
		getByUserIDStmt:                        getByUserIDStmt,
		getByUserIDPaginateStmt:                getByUserIDPaginateStmt,
		getByTokenIdentifiersStmt:              getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt:      getByTokenIdentifiersPaginateStmt,
		updateInfoStmt:                         updateInfoStmt,
		updateMediaStmt:                        updateMediaStmt,
		updateInfoByTokenIdentifiersUnsafeStmt: updateInfoByTokenIdentifiersUnsafeStmt,
		updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt: updateURIDerivedFieldsByTokenIdentifiersUnsafeStmt,
		updateMediaByTokenIdentifiersUnsafeStmt:             updateMediaByTokenIdentifiersUnsafeStmt,
		updateMetadataFieldsByTokenIdentifiersUnsafeStmt:    updateMetadataFieldsByTokenIdentifiersUnsafeStmt,
		deleteByIdentifiersStmt:                             deleteByIdentifiersStmt,
		getByTokenIDStmt:                                    getByTokenIDStmt,
		getByTokenIDPaginateStmt:                            getByTokenIDPaginateStmt,
		deleteByIDStmt:                                      deleteByIDStmt,
		getContractByAddressStmt:                            getContractByAddressStmt,
		setTokensAsUserMarkedSpamStmt:                       setTokensAsUserMarkedSpamStmt,
		checkOwnTokensStmt:                                  checkOwnTokensStmt,
		getByFullIdentifiersStmt:                            getByFullIdentifiersStmt,
		deleteTokensOfContractBeforeTimeStampStmt:           deleteTokensOfContractBeforeTimeStampStmt,
		deleteTokensOfOwnerBeforeTimeStampStmt:              deleteTokensOfOwnerBeforeTimeStampStmt,
	}

}

// GetByID gets a token by its DBID
func (t *TokenGalleryRepository) GetByID(pCtx context.Context, tokenID persist.DBID) (persist.TokenGallery, error) {
	token := persist.TokenGallery{}
	err := t.getByID.QueryRowContext(pCtx, tokenID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam)
	if err != nil {
		return persist.TokenGallery{}, err
	}
	return token, nil
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

// GetByTokenIdentifiers gets a token by its token ID and contract address and chain
func (t *TokenGalleryRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, limit int64, page int64) ([]persist.TokenGallery, error) {

	var contractID persist.DBID
	err := t.getContractByAddressStmt.QueryRowContext(pCtx, pContractAddress, pChain).Scan(&contractID)
	if err != nil {
		return nil, err
	}

	var rows *sql.Rows
	if limit > 0 {
		rows, err = t.getByTokenIdentifiersPaginateStmt.QueryContext(pCtx, pTokenID, contractID, limit, page*limit)
	} else {
		rows, err = t.getByTokenIdentifiersStmt.QueryContext(pCtx, pTokenID, contractID)
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

// GetByFullIdentifiers gets a token by its token ID and contract address and chain and owner user ID
func (t *TokenGalleryRepository) GetByFullIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, pUserID persist.DBID) (persist.TokenGallery, error) {

	var contractID persist.DBID
	err := t.getContractByAddressStmt.QueryRowContext(pCtx, pContractAddress, pChain).Scan(&contractID)
	if err != nil {
		return persist.TokenGallery{}, err
	}

	token := persist.TokenGallery{}
	err = t.getByFullIdentifiersStmt.QueryRowContext(pCtx, pTokenID, contractID, pUserID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam)
	if err != nil {
		return persist.TokenGallery{}, err
	}

	return token, nil
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

// BulkUpsertByOwnerUserID upserts multiple tokens for a user and removes any tokens that are not in the list
func (t *TokenGalleryRepository) BulkUpsertByOwnerUserID(pCtx context.Context, ownerUserID persist.DBID, chains []persist.Chain, pTokens []persist.TokenGallery, skipDelete bool) ([]persist.TokenGallery, error) {
	now, persistedTokens, err := t.bulkUpsert(pCtx, pTokens)
	if err != nil {
		return nil, err
	}

	// delete tokens of owner before timestamp

	if !skipDelete {
		res, err := t.deleteTokensOfOwnerBeforeTimeStampStmt.ExecContext(pCtx, ownerUserID, chains, now)
		if err != nil {
			return nil, fmt.Errorf("failed to delete tokens: %w", err)
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get rows affected: %w", err)
		}

		logger.For(pCtx).Infof("deleted %d tokens", rowsAffected)
	}

	return persistedTokens, nil
}

// BulkUpsertTokensOfContract upserts all tokens of a contract and deletes the old tokens
func (t *TokenGalleryRepository) BulkUpsertTokensOfContract(pCtx context.Context, contractID persist.DBID, pTokens []persist.TokenGallery, skipDelete bool) ([]persist.TokenGallery, error) {
	now, persistedTokens, err := t.bulkUpsert(pCtx, pTokens)
	if err != nil {
		return nil, err
	}

	// delete tokens of contract before timestamp
	if !skipDelete {
		_, err = t.deleteTokensOfContractBeforeTimeStampStmt.ExecContext(pCtx, contractID, now)
		if err != nil {
			return nil, fmt.Errorf("failed to delete tokens: %w", err)
		}
	}

	return persistedTokens, nil
}

func (t *TokenGalleryRepository) bulkUpsert(pCtx context.Context, pTokens []persist.TokenGallery) (time.Time, []persist.TokenGallery, error) {
	tokens, err := t.excludeZeroQuantityTokens(pCtx, pTokens)
	if err != nil {
		return time.Time{}, nil, err
	}

	if len(tokens) == 0 {
		return time.Time{}, []persist.TokenGallery{}, nil
	}

	appendWalletList := func(dest *[]string, src []persist.Wallet, startIndices, endIndices *[]int32) {
		items := make([]persist.DBID, len(src))
		for i, wallet := range src {
			items[i] = wallet.ID
		}
		appendDBIDList(dest, items, startIndices, endIndices)
	}

	appendAddressAtBlock := func(dest *[]pgtype.JSONB, src []persist.AddressAtBlock, startIndices, endIndices *[]int32, errs *[]error) {
		items := make([]any, len(src))
		for i, item := range src {
			items[i] = item
		}
		appendJSONBList(dest, items, startIndices, endIndices, errs)
	}

	// addIDIfMissing is used because sqlc was unable to bind arrays of our own custom types
	// i.e. an array of persist.DBIDs instead of an array of strings. A zero-valued persist.DBID
	// generates a new ID on insert, but instead we need to generate an ID beforehand.
	addIDIfMissing := func(t *persist.TokenGallery) {
		if t.ID == persist.DBID("") {
			(*t).ID = persist.GenerateID()
		}
	}

	// addTimesIfMissing is required because sqlc was unable to bind arrays of our own custom types
	// i.e. an array of persist.CreationTime instead of an array of time.Time. A zero-valued persist.CreationTime
	// uses the current time as the column value, but instead we need to manually add a time to the struct.
	addTimesIfMissing := func(t *persist.TokenGallery, ts time.Time) {
		if t.CreationTime.Time().IsZero() {
			(*t).CreationTime = persist.CreationTime(ts)
		}
		if t.LastSynced.Time().IsZero() {
			(*t).LastSynced = persist.LastUpdatedTime(ts)
		}
		if t.LastUpdated.Time().IsZero() {
			(*t).LastUpdated = persist.LastUpdatedTime(ts)
		}
	}

	tokens = t.dedupeTokens(tokens)
	params := db.UpsertTokensParams{}
	now := time.Now()

	var errors []error

	for i := range tokens {
		t := &tokens[i]
		addIDIfMissing(t)
		addTimesIfMissing(t, now)
		params.ID = append(params.ID, t.ID.String())
		params.Deleted = append(params.Deleted, t.Deleted.Bool())
		params.Version = append(params.Version, t.Version.Int32())
		params.CreatedAt = append(params.CreatedAt, t.CreationTime.Time())
		params.LastUpdated = append(params.LastUpdated, t.LastUpdated.Time())
		params.Name = append(params.Name, t.Name.String())
		params.Description = append(params.Description, t.Description.String())
		params.CollectorsNote = append(params.CollectorsNote, t.CollectorsNote.String())
		appendJSONB(&params.Media, t.Media, &errors)
		params.TokenType = append(params.TokenType, t.TokenType.String())
		params.TokenID = append(params.TokenID, t.TokenID.String())
		params.Quantity = append(params.Quantity, t.Quantity.String())
		appendAddressAtBlock(&params.OwnershipHistory, t.OwnershipHistory, &params.OwnershipHistoryStartIdx, &params.OwnershipHistoryEndIdx, &errors)
		appendJSONB(&params.TokenMetadata, t.TokenMetadata, &errors)
		params.ExternalUrl = append(params.ExternalUrl, t.ExternalURL.String())
		params.BlockNumber = append(params.BlockNumber, t.BlockNumber.BigInt().Int64())
		params.OwnerUserID = append(params.OwnerUserID, t.OwnerUserID.String())
		appendWalletList(&params.OwnedByWallets, t.OwnedByWallets, &params.OwnedByWalletsStartIdx, &params.OwnedByWalletsEndIdx)
		params.Chain = append(params.Chain, int32(t.Chain))
		params.Contract = append(params.Contract, t.Contract.String())
		appendBool(&params.IsUserMarkedSpam, t.IsUserMarkedSpam, &errors)
		appendBool(&params.IsProviderMarkedSpam, t.IsProviderMarkedSpam, &errors)
		params.LastSynced = append(params.LastSynced, t.LastSynced.Time())
		params.TokenUri = append(params.TokenUri, "")

		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	upserted, err := t.queries.UpsertTokens(pCtx, params)
	if err != nil {
		return time.Time{}, nil, err
	}

	// Update tokens with the existing data if the token already exists.
	for i := range tokens {
		t := &tokens[i]
		(*t).ID = upserted[i].ID
		(*t).CreationTime = persist.CreationTime(upserted[i].CreatedAt)
		(*t).LastUpdated = persist.LastUpdatedTime(upserted[i].LastUpdated)
		(*t).LastSynced = persist.LastUpdatedTime(upserted[i].LastSynced)
		(*t).Media = upserted[i].Media
	}

	return now, tokens, nil
}

func appendIndices(startIndices *[]int32, endIndices *[]int32, entryLength int) {
	// Postgres uses 1-based indexing
	startIndex := int32(1)
	if len(*endIndices) > 0 {
		startIndex = (*endIndices)[len(*endIndices)-1] + 1
	}
	*startIndices = append(*startIndices, startIndex)
	*endIndices = append(*endIndices, startIndex+int32(entryLength)-1)
}

func appendBool(dest *[]bool, src *bool, errs *[]error) {
	if src == nil {
		*dest = append(*dest, false)
		return
	}
	*dest = append(*dest, *src)
}

func appendJSONB(dest *[]pgtype.JSONB, src any, errs *[]error) error {
	jsonb, err := persist.ToJSONB(src)
	if err != nil {
		*errs = append(*errs, err)
		return err
	}
	*dest = append(*dest, jsonb)
	return nil
}

func appendDBIDList(dest *[]string, src []persist.DBID, startIndices, endIndices *[]int32) {
	for _, id := range src {
		*dest = append(*dest, id.String())
	}
	appendIndices(startIndices, endIndices, len(src))
}

func appendJSONBList(dest *[]pgtype.JSONB, src []any, startIndices, endIndices *[]int32, errs *[]error) {
	for _, item := range src {
		if err := appendJSONB(dest, item, errs); err != nil {
			return
		}
	}
	appendIndices(startIndices, endIndices, len(src))
}

func (t *TokenGalleryRepository) excludeZeroQuantityTokens(pCtx context.Context, pTokens []persist.TokenGallery) ([]persist.TokenGallery, error) {
	newTokens := make([]persist.TokenGallery, 0, len(pTokens))
	for _, token := range pTokens {
		if token.Quantity == "" || token.Quantity == "0" {
			logger.For(pCtx).Warnf("Token %s has 0 quantity", token.Name)
			continue
		}
		newTokens = append(newTokens, token)
	}
	return newTokens, nil
}

// UpdateByID updates a token by its ID
func (t *TokenGalleryRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error

	switch pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		update := pUpdate.(persist.TokenUpdateInfoInput)
		res, err = t.updateInfoStmt.ExecContext(pCtx, update.CollectorsNote, update.LastUpdated, pID, pUserID)
	case persist.TokenUpdateAllURIDerivedFieldsInput:
		update := pUpdate.(persist.TokenUpdateAllURIDerivedFieldsInput)
		res, err = t.updateMediaStmt.ExecContext(pCtx, update.Media, update.Metadata, update.LastUpdated, pID, pUserID)

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
	return nil
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
	case persist.TokenUpdateAllURIDerivedFieldsInput:
		update := pUpdate.(persist.TokenUpdateAllURIDerivedFieldsInput)
		res, err = t.updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Media, update.Metadata, update.Name, update.Description, update.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateMediaInput:
		update := pUpdate.(persist.TokenUpdateMediaInput)
		res, err = t.updateMediaByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Media, update.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateMetadataFieldsInput:
		update := pUpdate.(persist.TokenUpdateMetadataFieldsInput)
		res, err = t.updateMetadataFieldsByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, update.Name, update.Description, update.LastUpdated, pTokenID, contractID)
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
