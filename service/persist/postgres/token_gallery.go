package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/util"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"

	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
)

// TokenGalleryRepository represents a postgres repository for tokens
type TokenGalleryRepository struct {
	db                                                    *sql.DB
	queries                                               *db.Queries
	getByID                                               *sql.Stmt
	getByUserIDStmt                                       *sql.Stmt
	getByUserIDPaginateStmt                               *sql.Stmt
	getByTokenIdentifiersStmt                             *sql.Stmt
	getByTokenIdentifiersPaginateStmt                     *sql.Stmt
	getByFullIdentifiersStmt                              *sql.Stmt
	getByContractIDStmt                                   *sql.Stmt
	updateInfoStmt                                        *sql.Stmt
	updateMediaStmt                                       *sql.Stmt
	updateInfoByTokenIdentifiersUnsafeStmt                *sql.Stmt
	updateAllURIDerivedFieldsByTokenIdentifiersUnsafeStmt *sql.Stmt
	updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt   *sql.Stmt
	updateMediaByTokenIdentifiersUnsafeStmt               *sql.Stmt
	updateMetadataFieldsByTokenIdentifiersUnsafeStmt      *sql.Stmt
	getContractByAddressStmt                              *sql.Stmt
	setTokensAsUserMarkedSpamStmt                         *sql.Stmt
	checkOwnTokensStmt                                    *sql.Stmt
	deleteTokensOfContractBeforeTimeStampStmt             *sql.Stmt
}

var errTokensNotOwnedByUser = errors.New("not all tokens are owned by user")

// NewTokenGalleryRepository creates a new TokenRepository
// TODO joins on addresses
func NewTokenGalleryRepository(db *sql.DB, queries *db.Queries) *TokenGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByIDStmt, err := db.PrepareContext(ctx, `SELECT tokens.ID,COLLECTORS_NOTE,tokens.MEDIA,token_medias.MEDIA as token_media,token_medias.ID AS token_media_id,TOKEN_TYPE,tokens.CHAIN,tokens.NAME,tokens.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,tokens.EXTERNAL_URL,BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID WHERE tokens.ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false;`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `SELECT tokens.ID,COLLECTORS_NOTE,token_medias.MEDIA,TOKEN_MEDIA_ID,TOKEN_TYPE,tokens.CHAIN,tokens.NAME,tokens.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID WHERE OWNER_USER_ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByUserIDPaginateStmt, err := db.PrepareContext(ctx, `SELECT tokens.ID,COLLECTORS_NOTE,token_medias.MEDIA,TOKEN_MEDIA_ID,TOKEN_TYPE,tokens.CHAIN,tokens.NAME,tokens.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID WHERE OWNER_USER_ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $2 OFFSET $3;`)
	checkNoErr(err)

	getByContractIDStmt, err := db.PrepareContext(ctx, `SELECT tokens.ID,COLLECTORS_NOTE,token_medias.MEDIA,TOKEN_MEDIA_ID,TOKEN_TYPE,tokens.CHAIN,tokens.NAME,tokens.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID WHERE tokens.CONTRACT = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DISPLAYABLE AND DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `SELECT ID,COLLECTORS_NOTE,MEDIA,TOKEN_TYPE,CHAIN,NAME,DESCRIPTION,TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,EXTERNAL_URL,BLOCK_NUMBER,VERSION,CREATED_AT,LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens WHERE TOKEN_ID = $1 AND CONTRACT = $2 AND DISPLAYABLE AND DELETED = false ORDER BY BLOCK_NUMBER DESC LIMIT $3 OFFSET $4;`)
	checkNoErr(err)

	getByFullIdentifiersStmt, err := db.PrepareContext(ctx, `SELECT tokens.ID,COLLECTORS_NOTE,tokens.MEDIA,token_medias.MEDIA as token_media,token_medias.ID AS token_media_id,TOKEN_TYPE,tokens.CHAIN,tokens.NAME,tokens.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,TOKEN_METADATA,CONTRACT,tokens.EXTERNAL_URL,BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,IS_USER_MARKED_SPAM,IS_PROVIDER_MARKED_SPAM FROM tokens LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID WHERE tokens.TOKEN_ID = $1 AND tokens.CONTRACT = $2 AND tokens.OWNER_USER_ID = $3 AND tokens.DISPLAYABLE AND tokens.DELETED = false ORDER BY BLOCK_NUMBER DESC;`)
	checkNoErr(err)

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateMediaStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = '', TOKEN_METADATA = $2, LAST_UPDATED = $3 WHERE ID = $4 AND OWNER_USER_ID = $5;`)
	checkNoErr(err)

	updateInfoByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateURIDerivedFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, TOKEN_URI = '', TOKEN_METADATA = $2, NAME = $3, DESCRIPTION = $4, LAST_UPDATED = $5 WHERE TOKEN_ID = $6 AND CONTRACT = $7 AND DELETED = false;`)
	checkNoErr(err)

	updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET TOKEN_URI = '', TOKEN_METADATA = $1, NAME = $2, DESCRIPTION = $3, LAST_UPDATED = $4 WHERE TOKEN_ID = $5 AND CONTRACT = $6 AND DELETED = false;`)
	checkNoErr(err)

	updateMediaByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET MEDIA = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	updateMetadataFieldsByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET NAME = $1, DESCRIPTION = $2, LAST_UPDATED = $3 WHERE TOKEN_ID = $4 AND CONTRACT = $5 AND DELETED = false;`)
	checkNoErr(err)

	getContractByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	setTokensAsUserMarkedSpamStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET is_user_marked_spam = $1, LAST_UPDATED = now() WHERE OWNER_USER_ID = $2 AND ID = ANY($3) AND DELETED = false;`)
	checkNoErr(err)

	checkOwnTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) = $1 FROM tokens WHERE OWNER_USER_ID = $2 AND ID = ANY($3);`)
	checkNoErr(err)

	deleteTokensOfContractBeforeTimeStampStmt, err := db.PrepareContext(ctx, `update tokens set owned_by_wallets = '{}' where contract = $1 and last_synced < $2 and deleted = false;`)
	checkNoErr(err)

	return &TokenGalleryRepository{
		db:                                     db,
		queries:                                queries,
		getByID:                                getByIDStmt,
		getByUserIDStmt:                        getByUserIDStmt,
		getByUserIDPaginateStmt:                getByUserIDPaginateStmt,
		getByTokenIdentifiersStmt:              getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt:      getByTokenIdentifiersPaginateStmt,
		getByContractIDStmt:                    getByContractIDStmt,
		updateInfoStmt:                         updateInfoStmt,
		updateMediaStmt:                        updateMediaStmt,
		updateInfoByTokenIdentifiersUnsafeStmt: updateInfoByTokenIdentifiersUnsafeStmt,
		updateAllURIDerivedFieldsByTokenIdentifiersUnsafeStmt: updateURIDerivedFieldsByTokenIdentifiersUnsafeStmt,
		updateMediaByTokenIdentifiersUnsafeStmt:               updateMediaByTokenIdentifiersUnsafeStmt,
		updateMetadataFieldsByTokenIdentifiersUnsafeStmt:      updateMetadataFieldsByTokenIdentifiersUnsafeStmt,
		getContractByAddressStmt:                              getContractByAddressStmt,
		setTokensAsUserMarkedSpamStmt:                         setTokensAsUserMarkedSpamStmt,
		checkOwnTokensStmt:                                    checkOwnTokensStmt,
		getByFullIdentifiersStmt:                              getByFullIdentifiersStmt,
		deleteTokensOfContractBeforeTimeStampStmt:             deleteTokensOfContractBeforeTimeStampStmt,
		updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt:   updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt,
	}

}

// GetByID gets a token by its DBID
func (t *TokenGalleryRepository) GetByID(pCtx context.Context, tokenID persist.DBID) (persist.TokenGallery, error) {
	token := persist.TokenGallery{}
	err := t.getByID.QueryRowContext(pCtx, tokenID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenMedia, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam)
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
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam); err != nil {
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
	err = t.getByFullIdentifiersStmt.QueryRowContext(pCtx, pTokenID, contractID, pUserID).Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenMedia, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam)
	if err != nil {
		return persist.TokenGallery{}, err
	}

	return token, nil
}

// GetByContractID gets all tokens for a contract
func (t *TokenGalleryRepository) GetByContractID(pCtx context.Context, pContractID persist.DBID) ([]persist.TokenGallery, error) {
	rows, err := t.getByContractIDStmt.QueryContext(pCtx, pContractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.Media, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.Contract, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil

}

type TokenUpsertParams struct {
	SetCreatorFields bool
	SetHolderFields  bool
	OptionalDelete   *TokenUpsertDeletionParams
}

type TokenUpsertDeletionParams struct {
	DeleteCreatorStatus bool
	DeleteHolderStatus  bool
	OnlyFromUserID      *persist.DBID
	OnlyFromContracts   []persist.DBID
	OnlyFromChains      []persist.Chain
}

func (d *TokenUpsertDeletionParams) ToParams(upsertTime time.Time) db.DeleteTokensBeforeTimestampParams {
	userID := ""
	if d.OnlyFromUserID != nil {
		userID = d.OnlyFromUserID.String()
	}

	chains := util.MapWithoutError(d.OnlyFromChains, func(c persist.Chain) int32 { return int32(c) })
	return db.DeleteTokensBeforeTimestampParams{
		RemoveCreatorStatus: d.DeleteCreatorStatus,
		RemoveHolderStatus:  d.DeleteHolderStatus,
		OnlyFromUserID:      sql.NullString{String: userID, Valid: d.OnlyFromUserID != nil},
		OnlyFromChains:      chains,
		OnlyFromContractIds: util.StringersToStrings(d.OnlyFromContracts),
		Timestamp:           upsertTime,
	}
}

func (t *TokenGalleryRepository) UpsertTokens(ctx context.Context, pTokens []persist.TokenGallery, setCreatorFields bool, setHolderFields bool) (time.Time, []persist.TokenGallery, error) {
	tokens, err := t.excludeZeroQuantityTokens(ctx, pTokens)
	if err != nil {
		return time.Time{}, nil, err
	}

	// If we're not upserting anything, we still need to return the current database time
	// since it may be used by the caller and is assumed valid if err == nil
	if len(tokens) == 0 {
		currentTime, err := t.queries.GetCurrentTime(ctx)
		if err != nil {
			return time.Time{}, nil, err
		}
		return currentTime, []persist.TokenGallery{}, nil
	}

	tokens = t.dedupeTokens(tokens)
	params := db.UpsertTokensParams{
		SetCreatorFields: setCreatorFields,
		SetHolderFields:  setHolderFields,
		OwnedByWallets:   []string{},
	}

	var errors []error

	for i := range tokens {
		t := &tokens[i]
		params.ID = append(params.ID, persist.GenerateID().String())
		params.Version = append(params.Version, t.Version.Int32())
		params.Name = append(params.Name, t.Name.String())
		params.Description = append(params.Description, t.Description.String())
		params.CollectorsNote = append(params.CollectorsNote, t.CollectorsNote.String())
		params.TokenType = append(params.TokenType, t.TokenType.String())
		params.TokenID = append(params.TokenID, t.TokenID.String())
		params.Quantity = append(params.Quantity, t.Quantity.String())
		InsertHelpers.AppendAddressAtBlock(&params.OwnershipHistory, t.OwnershipHistory, &params.OwnershipHistoryStartIdx, &params.OwnershipHistoryEndIdx, &errors)
		appendJSONB(&params.Media, t.Media, &errors)
		appendJSONB(&params.FallbackMedia, t.FallbackMedia, &errors)
		appendJSONB(&params.TokenMetadata, t.TokenMetadata, &errors)
		params.ExternalUrl = append(params.ExternalUrl, t.ExternalURL.String())
		params.BlockNumber = append(params.BlockNumber, t.BlockNumber.BigInt().Int64())
		params.OwnerUserID = append(params.OwnerUserID, t.OwnerUserID.String())
		InsertHelpers.AppendWalletList(&params.OwnedByWallets, t.OwnedByWallets, &params.OwnedByWalletsStartIdx, &params.OwnedByWalletsEndIdx)
		params.Chain = append(params.Chain, int32(t.Chain))
		params.Contract = append(params.Contract, t.Contract.String())
		appendBool(&params.IsProviderMarkedSpam, t.IsProviderMarkedSpam, &errors)
		params.TokenUri = append(params.TokenUri, "")
		params.IsCreatorToken = append(params.IsCreatorToken, t.IsCreatorToken)

		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	upserted, err := t.queries.UpsertTokens(ctx, params)
	if err != nil {
		return time.Time{}, nil, err
	}

	// Update tokens with the existing data if the token already exists.
	for i := range tokens {
		t := &tokens[i]
		(*t).ID = upserted[i].ID
		(*t).CreationTime = upserted[i].CreatedAt
		(*t).LastUpdated = upserted[i].LastUpdated
		(*t).LastSynced = upserted[i].LastSynced
	}

	return upserted[0].LastSynced, tokens, nil
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

	switch errTyp := pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		res, err = t.updateInfoStmt.ExecContext(pCtx, errTyp.CollectorsNote, errTyp.LastUpdated, pID, pUserID)
	case persist.TokenUpdateAllURIDerivedFieldsInput:
		res, err = t.updateMediaStmt.ExecContext(pCtx, errTyp.Media, errTyp.Metadata, errTyp.LastUpdated, pID, pUserID)

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
	switch errTyp := pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		res, err = t.updateInfoByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.CollectorsNote, errTyp.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateAllURIDerivedFieldsInput:
		res, err = t.updateAllURIDerivedFieldsByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.Media, errTyp.Metadata, errTyp.Name, errTyp.Description, errTyp.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateAllMetadataFieldsInput:
		res, err = t.updateAllMetadataFieldsByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.Metadata, errTyp.Name, errTyp.Description, errTyp.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateMediaInput:
		res, err = t.updateMediaByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.Media, errTyp.LastUpdated, pTokenID, contractID)
	case persist.TokenUpdateMetadataDerivedFieldsInput:
		res, err = t.updateMetadataFieldsByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.Name, errTyp.Description, errTyp.LastUpdated, pTokenID, contractID)
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
